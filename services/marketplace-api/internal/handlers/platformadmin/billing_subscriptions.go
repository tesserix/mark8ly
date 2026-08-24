package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// SubscriptionLister is the subset of subscription.Repository this handler
// needs, declared as an interface so the handler can be tested with a stub —
// the same pattern TrialLister uses in billing_trials.go for
// trial.ListExpiring.
type SubscriptionLister interface {
	ListAllSubscriptions(ctx context.Context, db *gorm.DB, f subscription.CrossTenantFilter) ([]subscription.StoreSubscription, int64, error)
}

// SubscriptionListerFunc adapts a bare function — such as a
// subscription.Repository's ListAllSubscriptions method value — to the
// SubscriptionLister interface.
type SubscriptionListerFunc func(ctx context.Context, db *gorm.DB, f subscription.CrossTenantFilter) ([]subscription.StoreSubscription, int64, error)

// ListAllSubscriptions implements SubscriptionLister by delegating to the
// wrapped func.
func (f SubscriptionListerFunc) ListAllSubscriptions(ctx context.Context, db *gorm.DB, filter subscription.CrossTenantFilter) ([]subscription.StoreSubscription, int64, error) {
	return f(ctx, db, filter)
}

// defaultBillingSubscriptionsLimit and maxBillingSubscriptionsLimit mirror
// subscription.DefaultCrossTenantPageSize/MaxCrossTenantPageSize so the
// handler reports the SAME effective limit it sends downstream — the
// repository clamps independently, but a stub in tests does not, so the
// handler must clamp itself before the pagination envelope is built.
const (
	defaultBillingSubscriptionsLimit = subscription.DefaultCrossTenantPageSize
	maxBillingSubscriptionsLimit     = subscription.MaxCrossTenantPageSize
)

// consoleHiddenStatuses are the lifecycle states the platform console does
// not filter on. Both are internal-only teardown states with no operator
// meaning.
//
// This is an explicit DENY list rather than a hand-written allow list: a new
// SubscriptionStatus added to AllStatuses() becomes filterable automatically,
// and hiding one requires editing this set deliberately. The reverse — a
// hand-written allow list — silently leaves new states unfilterable, which
// is how a console loses sight of a state nobody remembered to add.
var consoleHiddenStatuses = map[subscription.SubscriptionStatus]bool{
	subscription.StatusPendingHardDelete: true,
	subscription.StatusHardDeleted:       true,
}

// validSubscriptionStatuses is derived from subscription.AllStatuses()
// minus consoleHiddenStatuses — every status this surface accepts as a
// `status` filter. See billing_subscriptions_test.go's table-driven status
// test, which loops over subscription.AllStatuses() so a status added to
// AllStatuses() without a corresponding entry in consoleHiddenStatuses is
// accepted automatically, and hiding one is a deliberate, tested edit.
// AllStatuses() itself is verified against the store_subscriptions.status
// database CHECK constraint by
// subscription.TestAllStatuses_MatchesDatabaseCheckConstraint, which is
// what makes it trustworthy as "every status" in the first place — see that
// test's doc comment for why the schema, not this package, is the
// authority.
var validSubscriptionStatuses = func() map[string]bool {
	m := make(map[string]bool)
	for _, status := range subscription.AllStatuses() {
		if !consoleHiddenStatuses[status] {
			m[string(status)] = true
		}
	}
	return m
}()

// validSubscriptionPlans is the five subscription.Plan* values this surface
// accepts as a `plan` filter — including trial and marketplace, which have
// no catalog price but are still valid rows/filters.
var validSubscriptionPlans = map[string]bool{
	string(subscription.PlanTrial):       true,
	string(subscription.PlanStarter):     true,
	string(subscription.PlanStudio):      true,
	string(subscription.PlanPro):         true,
	string(subscription.PlanMarketplace): true,
}

// BillingSubscriptionsHandler serves the platform console's GET
// /admin/billing/subscriptions (#284): a cross-tenant, filterable,
// paginated view of every store's subscription.
type BillingSubscriptionsHandler struct {
	subs   SubscriptionLister
	dir    TenantDirectory
	db     *gorm.DB
	logger *slog.Logger
}

// NewBillingSubscriptionsHandler constructs the handler. logger may be nil.
func NewBillingSubscriptionsHandler(subs SubscriptionLister, dir TenantDirectory, db *gorm.DB, logger *slog.Logger) *BillingSubscriptionsHandler {
	return &BillingSubscriptionsHandler{subs: subs, dir: dir, db: db, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *BillingSubscriptionsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/billing/subscriptions", h.list)
}

// subscriptionRow is the wire shape for one cross-tenant subscription.
//
// TenantName is omitempty: a tenant missing from the directory response gets
// its name omitted rather than a blank string, matching trialRow above.
// Amount is a *money with omitempty: resolveMoney returning ok=false (no
// billing_currency, or a plan/period/currency the catalog has no price for —
// trial and marketplace by design) means the row carries NO amount key at
// all, never null and never a guessed 0. CurrentPeriodEnd is likewise
// omitempty for a NULL column.
type subscriptionRow struct {
	TenantID          string `json:"tenant_id"`
	TenantName        string `json:"tenant_name,omitempty"`
	StoreID           string `json:"store_id"`
	Plan              string `json:"plan"`
	Period            string `json:"period"`
	Status            string `json:"status"`
	Amount            *money `json:"amount,omitempty"`
	CurrentPeriodEnd  string `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
}

type billingSubscriptionsResponse struct {
	Data       []subscriptionRow `json:"data"`
	Pagination pagination        `json:"pagination"`
}

func (h *BillingSubscriptionsHandler) list(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !validSubscriptionStatuses[status] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "validation_error", "message": "status is invalid",
		})
		return
	}

	plan := strings.TrimSpace(c.Query("plan"))
	if plan != "" && !validSubscriptionPlans[plan] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "validation_error", "message": "plan is invalid",
		})
		return
	}

	page := parsePositiveIntDefault(c.Query("page"), 1)
	limit := parsePositiveIntDefault(c.Query("limit"), defaultBillingSubscriptionsLimit)
	if limit > maxBillingSubscriptionsLimit {
		limit = maxBillingSubscriptionsLimit
	}

	ctx := c.Request.Context()
	filter := subscription.CrossTenantFilter{Status: status, Plan: plan, Page: page, Limit: limit}

	rows, total, err := h.subs.ListAllSubscriptions(ctx, h.db, filter)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	names, err := h.lookupTenantNames(ctx, rows)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []`.
	out := make([]subscriptionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSubscriptionRow(r, names))
	}

	c.JSON(http.StatusOK, billingSubscriptionsResponse{
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
// otherwise match "no id filter" on the wire and pull back the whole
// directory instead of nothing. Mirrors BillingTrialsHandler.lookupTenantNames.
func (h *BillingSubscriptionsHandler) lookupTenantNames(ctx context.Context, rows []subscription.StoreSubscription) (map[string]string, error) {
	names := map[string]string{}
	if len(rows) == 0 {
		return names, nil
	}

	seen := make(map[string]bool, len(rows))
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		id := r.TenantID.String()
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	res, err := h.dir.List(ctx, tenantdirectory.ListParams{IDs: ids, Limit: len(ids)})
	if err != nil {
		return nil, err
	}

	for _, t := range res.Tenants {
		names[t.ID] = t.Name
	}

	// A tenant id present on a subscription row but absent from the
	// directory response means the two services disagree about which
	// tenants exist. The row still appears (its name is simply omitted) but
	// this is worth a log line, not silence.
	for _, id := range ids {
		if _, ok := names[id]; !ok && h.logger != nil {
			h.logger.Warn("billing subscriptions: tenant missing from directory", "tenant_id", id)
		}
	}

	return names, nil
}

func toSubscriptionRow(r subscription.StoreSubscription, names map[string]string) subscriptionRow {
	tenantID := r.TenantID.String()
	row := subscriptionRow{
		TenantID:          tenantID,
		TenantName:        names[tenantID],
		StoreID:           r.StoreID.String(),
		Plan:              string(r.Plan),
		Period:            string(r.SubscriptionPeriod),
		Status:            string(r.Status),
		CancelAtPeriodEnd: r.CancelAtPeriodEnd,
	}

	if m, ok := resolveMoney(string(r.Plan), string(r.SubscriptionPeriod), r.BillingCurrency, r.PriceTier); ok {
		row.Amount = &m
	}

	if r.CurrentPeriodEnd != nil {
		row.CurrentPeriodEnd = r.CurrentPeriodEnd.UTC().Format(time.RFC3339)
	}

	return row
}

// respondErr maps a gather-stage error to the surface's stable codes,
// mirroring respondErr in billing_trials.go and entities_tenants.go so this
// surface stays internally consistent.
//
// tenantdirectory.ErrUnavailable (platform-api unreachable or 5xx) becomes
// 503 upstream_unavailable. Any other error — including a bare
// ListAllSubscriptions (DB) error, which wraps neither sentinel — becomes
// 500 internal_error. Neither path ever returns a partial page.
func (h *BillingSubscriptionsHandler) respondErr(c *gin.Context, err error) {
	if errors.Is(err, tenantdirectory.ErrUnavailable) {
		if h.logger != nil {
			h.logger.Error("billing subscriptions upstream unavailable", "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "tenant directory is unavailable",
		})
		return
	}
	if h.logger != nil {
		h.logger.Error("billing subscriptions", "err", err)
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal_error", "message": "could not list subscriptions",
	})
}
