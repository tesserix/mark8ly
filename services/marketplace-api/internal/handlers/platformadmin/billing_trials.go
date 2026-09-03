package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// TrialLister is the subset of trial.ListExpiring this handler needs,
// declared as an interface so the handler can be tested with a stub.
// TrialListerFunc below adapts the bare package function directly, the same
// pattern Subscriptions/SubscriptionsFunc uses in kpis.go for
// trial.CountExpiring.
type TrialLister interface {
	ListExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int, opts trial.ListOptions) ([]trial.ExpiringRow, int64, error)
}

// TrialListerFunc adapts a bare function — such as trial.ListExpiring — to
// the TrialLister interface.
type TrialListerFunc func(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int, opts trial.ListOptions) ([]trial.ExpiringRow, int64, error)

// ListExpiring implements TrialLister by delegating to the wrapped func.
func (f TrialListerFunc) ListExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int, opts trial.ListOptions) ([]trial.ExpiringRow, int64, error) {
	return f(ctx, db, asOf, window, page, limit, opts)
}

// defaultBillingTrialsLimit and maxBillingTrialsLimit bound `limit`, mirroring
// the days bounds trial.DefaultExpiryWindow/trial.MaxExpiryWindow already
// enforce on the query window.
const (
	defaultBillingTrialsLimit = 50
	maxBillingTrialsLimit     = 500
)

// BillingTrialsHandler serves the platform console's GET
// /admin/billing/trials (#285): trials about to expire, page by page.
//
// amount is resolved via the same resolveMoney helper
// /admin/billing/subscriptions uses, so the two surfaces cannot disagree
// about what a plan costs (#328). It is omitted, never null or zero, when
// resolveMoney reports ok=false — which is the normal outcome for every row
// on plan="trial": the catalog holds no Price objects for `trial` or
// `marketplace` by design. A trial cannot be in dunning either: the dunning
// ladder selects status IN (past_due, expired, store_closed), status is
// single-valued, and a trialing row's status is 'trialing'. So no
// `dunning_state` key. All three are structural absences, not nulled-out
// fields — see billing_trials_test.go for the raw-body assertions that pin
// this.
type BillingTrialsHandler struct {
	trials TrialLister
	dir    TenantDirectory
	db     *gorm.DB
	logger *slog.Logger
	// catalog resolves the plan catalog each response is priced from
	// (tesserix-home#328 phase C). Nil prices from the compiled catalog —
	// see compiledPriceCatalog for why that nil is the cutover's kill
	// switch and not a degraded state.
	catalog CatalogResolver
	// fallbackLog throttles the warning that a page was priced from a
	// non-fresh catalog. Per handler, not per package, so the interval is
	// testable without global state.
	fallbackLog *catalogFallbackLog
	// now is overridable in tests; production uses time.Now. The single
	// instant it returns is used for BOTH the query window and every row's
	// days_remaining — two clocks in one response is a defect this project
	// has already shipped once (see kpis.go's `now`, the same guard for
	// trials_expiring).
	now func() time.Time
}

// NewBillingTrialsHandler constructs the handler. logger may be nil. clock
// may be nil, in which case it defaults to time.Now; tests pass a fixed
// clock so the fixture's TrialEndsAt values and the resulting
// days_remaining stay deterministic, mirroring the clock parameter pattern
// used by trial.NewExpiryCron and friends.
func NewBillingTrialsHandler(trials TrialLister, dir TenantDirectory, db *gorm.DB, clock func() time.Time, catalog CatalogResolver, logger *slog.Logger) *BillingTrialsHandler {
	if clock == nil {
		clock = time.Now
	}
	return &BillingTrialsHandler{
		trials: trials, dir: dir, db: db, logger: logger, now: clock,
		catalog: catalog, fallbackLog: newCatalogFallbackLog(),
	}
}

// Register mounts the route on the supplied group.
func (h *BillingTrialsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/billing/trials", h.list)
}

// trialRow is the wire shape for one expiring trial.
//
// TenantName is omitempty: a tenant missing from the directory response
// gets its name omitted rather than a blank string — see list() for why
// that disagreement is also logged. BillingCurrency is omitempty for the
// same reason trial.ExpiringRow carries it as a pointer: a store that has
// never taken a currency has none to report, not an empty one. Amount is a
// *money with omitempty, mirroring subscriptionRow: resolveMoney returning
// ok=false (no billing_currency, or a plan/period/currency the catalog has
// no price for — trial and marketplace by design) means the row carries NO
// amount key at all, never null and never a guessed 0.
type trialRow struct {
	TenantID            string  `json:"tenant_id"`
	TenantName          string  `json:"tenant_name,omitempty"`
	StoreID             string  `json:"store_id"`
	TrialEndsAt         string  `json:"trial_ends_at"`
	DaysRemaining       int     `json:"days_remaining"`
	Plan                string  `json:"plan"`
	Period              string  `json:"period"`
	BillingCurrency     *string `json:"billing_currency,omitempty"`
	Amount              *money  `json:"amount,omitempty"`
	PaymentMethodOnFile bool    `json:"payment_method_on_file"`
	Status              string  `json:"status"`
	// StripeManaged has no omitempty: it is a fact about every row, and an
	// absent `false` would be indistinguishable from a server that predates
	// this field (#358).
	StripeManaged bool `json:"stripe_managed"`
}

type billingTrialsResponse struct {
	Data       []trialRow `json:"data"`
	Pagination pagination `json:"pagination"`
}

func (h *BillingTrialsHandler) list(c *gin.Context) {
	window := parseTrialWindow(c.Query("days"))
	page := parsePositiveIntDefault(c.Query("page"), 1)
	limit := parsePositiveIntDefault(c.Query("limit"), defaultBillingTrialsLimit)
	if limit > maxBillingTrialsLimit {
		limit = maxBillingTrialsLimit
	}

	// Default false: #285's live contract lists trials that will EXPIRE. The
	// flag widens it to trials the console can now EXTEND (#358), which is a
	// different question and an explicit one.
	includeStripeManaged := c.Query("include_stripe_managed") == "true"

	ctx := c.Request.Context()
	asOf := h.now()

	rows, total, err := h.trials.ListExpiring(ctx, h.db, asOf, window, page, limit, trial.ListOptions{IncludeStripeManaged: includeStripeManaged})
	if err != nil {
		h.respondErr(c, err)
		return
	}

	names, err := h.lookupTenantNames(ctx, rows)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Resolved ONCE for the whole page, off the request's context — never
	// context.Background(): a client that has gone away must be able to
	// cancel the console read this may perform, like every other upstream
	// call on this path.
	pc := resolvePriceCatalog(ctx, h.catalog, h.fallbackLog, h.logger)

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []` and crashes their page precisely when there is no
	// data — same reasoning as toTenantRow's caller in entities_tenants.go.
	out := make([]trialRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTrialRow(r, names, asOf, pc))
	}

	c.JSON(http.StatusOK, billingTrialsResponse{
		Data: out,
		Pagination: pagination{
			Page:  page,
			Limit: limit,
			Total: total,
		},
	})
}

// lookupTenantNames collects the DISTINCT tenant ids on the page and makes
// exactly one tenantdirectory.List call for all of them, never one call per
// row. A page with zero rows makes no call at all: an empty IDs slice would
// send `ids=` (or nothing) on the wire, which tenantdirectory.List treats as
// "no id filter" and would return the WHOLE directory instead of nothing.
func (h *BillingTrialsHandler) lookupTenantNames(ctx context.Context, rows []trial.ExpiringRow) (map[string]string, error) {
	names := map[string]string{}
	if len(rows) == 0 {
		return names, nil
	}

	seen := make(map[string]bool, len(rows))
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if !seen[r.TenantID] {
			seen[r.TenantID] = true
			ids = append(ids, r.TenantID)
		}
	}

	res, err := h.dir.List(ctx, tenantdirectory.ListParams{IDs: ids, Limit: len(ids)})
	if err != nil {
		return nil, err
	}

	for _, t := range res.Tenants {
		names[t.ID] = t.Name
	}

	// A tenant id present on a trial row but absent from the directory
	// response means the two services disagree about which tenants exist.
	// The row still appears (its name is simply omitted) but this is
	// worth a log line, not silence.
	for _, id := range ids {
		if _, ok := names[id]; !ok && h.logger != nil {
			h.logger.Warn("billing trials: tenant missing from directory", "tenant_id", id)
		}
	}

	return names, nil
}

func toTrialRow(r trial.ExpiringRow, names map[string]string, asOf time.Time, pc priceCatalog) trialRow {
	row := trialRow{
		TenantID:      r.TenantID,
		TenantName:    names[r.TenantID],
		StoreID:       r.StoreID,
		TrialEndsAt:   r.TrialEndsAt.UTC().Format(time.RFC3339),
		DaysRemaining: daysRemaining(r.TrialEndsAt, asOf),
		Plan:          r.Plan,
		Period:        r.Period,
		// BillingCurrency copies the pointer, not the value: nil stays nil
		// and marshals as an omitted key via the omitempty tag above, never
		// a null.
		BillingCurrency:     r.BillingCurrency,
		PaymentMethodOnFile: r.HasPaymentMethod,
		Status:              r.Status,
		StripeManaged:       r.StripeManaged,
	}

	if m, ok := pc.resolveMoney(r.Plan, r.Period, r.BillingCurrency, r.PriceTier); ok {
		row.Amount = &m
	}

	return row
}

// daysRemaining is computed from asOf — the SAME instant the query itself
// used — never a fresh time.Now().
//
// This deliberately mirrors enrichTrialBanner's formula
// (internal/handlers/admin/subscription.go) rather than a plain
// math.Ceil(hours/24): for any non-integral remainder, Ceil rounds up one
// day further than the merchant-facing endpoint does, so the console would
// quote a different "days remaining" than the merchant's own dashboard for
// the same trial. The two surfaces must agree on this number, so they share
// the exact same arithmetic: floor(hours/24), bumped to 1 only when
// 0 < hours < 24, floored at zero.
func daysRemaining(trialEndsAt, asOf time.Time) int {
	hoursLeft := trialEndsAt.Sub(asOf).Hours()
	days := int(hoursLeft / 24)
	if hoursLeft > 0 && hoursLeft < 24 {
		days = 1
	}
	if days < 0 {
		days = 0
	}
	return days
}

// parseTrialWindow parses `days` into a query window. A missing or invalid
// value takes trial.DefaultExpiryWindow; this never errors. The result
// clamps to trial.MaxExpiryWindow — a bound to keep the window finite, not
// a claim about trial length: an operator-extended trial can end further
// out than TrialDays.
func parseTrialWindow(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return trial.DefaultExpiryWindow
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return trial.DefaultExpiryWindow
	}
	window := time.Duration(n) * 24 * time.Hour
	if window > trial.MaxExpiryWindow {
		return trial.MaxExpiryWindow
	}
	return window
}

// parsePositiveIntDefault parses raw as a positive int, falling back to def
// for an absent or invalid value. Never errors.
func parsePositiveIntDefault(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// respondErr maps a gather-stage error to the surface's stable codes,
// mirroring respondErr in entities_tenants.go and kpis.go so this surface
// stays internally consistent.
//
// tenantdirectory.ErrUnavailable (platform-api unreachable or 5xx) becomes
// 503 upstream_unavailable: retrying may help once the dependency recovers.
// Any other error — including a bare trial.ListExpiring (DB) error, which
// wraps neither sentinel — becomes 500 internal_error. Neither path ever
// returns a partial page: a partial page is indistinguishable from a
// complete one, so it must fail outright instead.
func (h *BillingTrialsHandler) respondErr(c *gin.Context, err error) {
	if errors.Is(err, tenantdirectory.ErrUnavailable) {
		if h.logger != nil {
			h.logger.Error("billing trials upstream unavailable", "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "tenant directory is unavailable",
		})
		return
	}
	if h.logger != nil {
		h.logger.Error("billing trials", "err", err)
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal_error", "message": "could not list expiring trials",
	})
}
