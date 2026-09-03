package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/estatecounts"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// stubTrialLister records the params ListExpiring was called with and
// returns canned rows/total.
type stubTrialLister struct {
	rows  []trial.ExpiringRow
	total int64
	err   error

	gotAsOf   time.Time
	gotWindow time.Duration
	gotPage   int
	gotLimit  int
	gotOpts   trial.ListOptions
	calls     int
}

func (s *stubTrialLister) ListExpiring(_ context.Context, _ *gorm.DB, asOf time.Time, window time.Duration, page, limit int, opts trial.ListOptions) ([]trial.ExpiringRow, int64, error) {
	s.calls++
	s.gotAsOf = asOf
	s.gotWindow = window
	s.gotPage = page
	s.gotLimit = limit
	s.gotOpts = opts
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.rows, s.total, nil
}

// stubBillingDirectory is a dedicated TenantDirectory stub for this file's
// tests, distinct from stubDirectory in entities_tenants_test.go, because it
// needs to count calls and answer only a fixed id->name map — the exact
// thing test 6 (one lookup per page) exists to check.
type stubBillingDirectory struct {
	names map[string]string // tenant id -> name; missing id = absent from directory
	err   error

	calls    int
	gotIDs   []string
	gotLimit int
}

func (s *stubBillingDirectory) List(_ context.Context, p tenantdirectory.ListParams) (*tenantdirectory.ListResult, error) {
	s.calls++
	s.gotIDs = p.IDs
	s.gotLimit = p.Limit
	if s.err != nil {
		return nil, s.err
	}
	tenants := make([]tenantdirectory.Tenant, 0, len(p.IDs))
	for _, id := range p.IDs {
		if name, ok := s.names[id]; ok {
			tenants = append(tenants, tenantdirectory.Tenant{ID: id, Name: name})
		}
	}
	return &tenantdirectory.ListResult{Tenants: tenants, Total: int64(len(tenants))}, nil
}

func (s *stubBillingDirectory) Get(_ context.Context, _ string) (*tenantdirectory.TenantDetail, error) {
	return nil, nil
}

func (s *stubBillingDirectory) FindByOwnerEmail(_ context.Context, _ string) (*tenantdirectory.Tenant, error) {
	return nil, nil
}

// billingTrialsRouter builds a router with a fixed clock so days_remaining
// stays deterministic across test runs.
func billingTrialsRouter(t *testing.T, trials platformadmin.TrialLister, dir platformadmin.TenantDirectory) *gin.Engine {
	return billingTrialsRouterAt(t, trials, dir, billingTrialsFixtureAsOf)
}

func billingTrialsRouterAt(t *testing.T, trials platformadmin.TrialLister, dir platformadmin.TenantDirectory, asOf time.Time) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// nil CatalogResolver: these tests pin the compiled-catalog
	// (rollback) behaviour, which is what an unconfigured console gives.
	platformadmin.NewBillingTrialsHandler(trials, dir, nil, func() time.Time { return asOf }, nil, nil).Register(r.Group(""))
	return r
}

// billingTrialsFixtureAsOf is the instant the fixture rows below are
// expressed relative to.
var billingTrialsFixtureAsOf = time.Date(2026, 8, 24, 9, 14, 2, 0, time.UTC)

// billingTrialsFixtureRows is the shared row set behind the golden fixture,
// the ordering test, and the cross-endpoint invariant test.
func billingTrialsFixtureRows(asOf time.Time) []trial.ExpiringRow {
	gbp := "GBP"
	return []trial.ExpiringRow{
		{
			TenantID:         "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
			StoreID:          "aaaaaaaa-4f89-11d3-9a0c-0305e82c3301",
			TrialEndsAt:      asOf.Add(3 * 24 * time.Hour),
			Plan:             "trial",
			Period:           "monthly",
			BillingCurrency:  &gbp,
			HasPaymentMethod: false,
			Status:           "trialing",
		},
		{
			TenantID:         "3f2504e0-4f89-11d3-9a0c-0305e82c3302",
			StoreID:          "bbbbbbbb-4f89-11d3-9a0c-0305e82c3302",
			TrialEndsAt:      asOf.Add(6 * 24 * time.Hour),
			Plan:             "trial",
			Period:           "annual",
			BillingCurrency:  nil,
			HasPaymentMethod: true,
			Status:           "trialing",
		},
	}
}

// THE test. Real handler output compared to the committed contract.
func TestBillingTrialsMatchesContract(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	rows := billingTrialsFixtureRows(asOf)

	trials := &stubTrialLister{rows: rows, total: int64(len(rows))}
	dir := &stubBillingDirectory{names: map[string]string{
		"3f2504e0-4f89-11d3-9a0c-0305e82c3301": "Acme Trading",
		"3f2504e0-4f89-11d3-9a0c-0305e82c3302": "Beta Goods",
	}}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/billing_trials_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

func TestBillingTrialsEmptyIsArray(t *testing.T) {
	trials := &stubTrialLister{rows: nil, total: 0}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":[],"pagination":{"page":1,"limit":50,"total":0}}`, rec.Body.String())

	// A page with no rows must not call the directory at all: an empty
	// IDs slice would otherwise match "no filter" on the wire and pull
	// back the whole directory.
	require.Equal(t, 0, dir.calls, "empty page must not call tenantdirectory.List")
}

// Rows come back from ListExpiring already ordered soonest-first. The
// handler must preserve that order, not resort it.
func TestBillingTrialsPreservesOrder(t *testing.T) {
	asOf := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	rows := []trial.ExpiringRow{
		{TenantID: "t-zzz", StoreID: "s-1", TrialEndsAt: asOf.Add(1 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
		{TenantID: "t-aaa", StoreID: "s-2", TrialEndsAt: asOf.Add(5 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
		{TenantID: "t-mmm", StoreID: "s-3", TrialEndsAt: asOf.Add(2 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
	}
	trials := &stubTrialLister{rows: rows, total: 3}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			StoreID string `json:"store_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 3)
	require.Equal(t, "s-1", body.Data[0].StoreID)
	require.Equal(t, "s-2", body.Data[1].StoreID)
	require.Equal(t, "s-3", body.Data[2].StoreID)
}

func TestBillingTrialsDaysClamps(t *testing.T) {
	trials := &stubTrialLister{}
	dir := &stubBillingDirectory{}
	router := billingTrialsRouter(t, trials, dir)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/billing/trials?days=9999", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, trial.MaxExpiryWindow, trials.gotWindow, "days=9999 must clamp to MaxExpiryWindow")

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/admin/billing/trials", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, trial.DefaultExpiryWindow, trials.gotWindow, "absent days must take DefaultExpiryWindow")
}

func TestBillingTrialsLimitClampsAndReportsEffectiveValue(t *testing.T) {
	trials := &stubTrialLister{}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials?limit=9999", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 500, trials.gotLimit, "limit must clamp to 500 before reaching the stub")

	var body struct {
		Pagination struct {
			Limit int `json:"limit"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 500, body.Pagination.Limit, "pagination.limit must report the clamped, effective value")
}

// One tenant lookup for a page with several rows sharing a tenant — the
// stub must be called exactly once and receive DEDUPLICATED ids, not one
// call per row.
func TestBillingTrialsOneTenantLookupPerPageDeduplicated(t *testing.T) {
	asOf := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	rows := []trial.ExpiringRow{
		{TenantID: "t-shared", StoreID: "s-1", TrialEndsAt: asOf.Add(1 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
		{TenantID: "t-shared", StoreID: "s-2", TrialEndsAt: asOf.Add(2 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
		{TenantID: "t-other", StoreID: "s-3", TrialEndsAt: asOf.Add(3 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
		{TenantID: "t-shared", StoreID: "s-4", TrialEndsAt: asOf.Add(4 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
	}
	trials := &stubTrialLister{rows: rows, total: 4}
	dir := &stubBillingDirectory{names: map[string]string{
		"t-shared": "Shared Co", "t-other": "Other Co",
	}}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, dir.calls, "must make exactly one tenantdirectory.List call per page")
	require.ElementsMatch(t, []string{"t-shared", "t-other"}, dir.gotIDs, "ids sent upstream must be deduplicated")
}

// A tenant absent from the directory response omits tenant_name; the row
// still appears.
func TestBillingTrialsMissingTenantOmitsName(t *testing.T) {
	asOf := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	rows := []trial.ExpiringRow{
		{TenantID: "t-ghost", StoreID: "s-1", TrialEndsAt: asOf.Add(1 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
	}
	trials := &stubTrialLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{names: map[string]string{}} // directory knows nothing

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "tenant_name", "a tenant missing from the directory must omit tenant_name entirely")

	var body struct {
		Data []struct {
			TenantID string `json:"tenant_id"`
			StoreID  string `json:"store_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1, "the row must still appear even without a name")
	require.Equal(t, "t-ghost", body.Data[0].TenantID)
}

func TestBillingTrialsUpstreamUnavailableIs503NoDataKey(t *testing.T) {
	asOf := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	rows := []trial.ExpiringRow{
		{TenantID: "t-1", StoreID: "s-1", TrialEndsAt: asOf.Add(1 * 24 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
	}
	trials := &stubTrialLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{err: tenantdirectory.ErrUnavailable}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasData := raw["data"]
	require.False(t, hasData, "a 503 must never carry a data key — no partial result")
}

func TestBillingTrialsListExpiringErrorIs500(t *testing.T) {
	trials := &stubTrialLister{err: errBoom}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
}

// A trial cannot be in dunning: the dunning ladder never selects status
// 'trialing', so no row ever carries a `dunning_state` key. amount is
// asserted absent too, but only because billingTrialsFixtureRows uses
// plan="trial" throughout, which the catalog excludes by design (#328) — see
// TestBillingTrialsPricedPlanCarriesAmount and
// TestBillingTrialsTrialPlanOmitsAmount below for the cases that pin amount
// as conditional rather than categorically absent.
func TestBillingTrialsNeverCarriesDunningState(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	rows := billingTrialsFixtureRows(asOf)

	cases := []struct {
		name    string
		trials  *stubTrialLister
		dir     *stubBillingDirectory
		request string
	}{
		{
			name:    "happy path",
			trials:  &stubTrialLister{rows: rows, total: int64(len(rows))},
			dir:     &stubBillingDirectory{names: map[string]string{"3f2504e0-4f89-11d3-9a0c-0305e82c3301": "Acme Trading", "3f2504e0-4f89-11d3-9a0c-0305e82c3302": "Beta Goods"}},
			request: "/admin/billing/trials",
		},
		{
			name:    "empty",
			trials:  &stubTrialLister{},
			dir:     &stubBillingDirectory{},
			request: "/admin/billing/trials",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			billingTrialsRouter(t, tc.trials, tc.dir).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.request, nil))
			body := rec.Body.String()
			require.False(t, strings.Contains(body, "\"amount\""), "fixture rows are all plan=trial, which the catalog excludes: no amount key")
			require.False(t, strings.Contains(body, "\"dunning_state\""), "no dunning_state key: a trial cannot be in dunning")
		})
	}
}

// TestBillingTrialsPricedPlanCarriesAmount proves a trial row on a merchant-
// chosen, PRICED plan (starter/studio/pro — reachable via
// internal/subscription/service.go's other creation path) carries `amount`
// with an uppercase currency, resolved through the same resolveMoney helper
// /admin/billing/subscriptions uses (#328).
func TestBillingTrialsPricedPlanCarriesAmount(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	gbp := "gbp"
	rows := []trial.ExpiringRow{
		{
			TenantID:         "t-priced",
			StoreID:          "s-priced",
			TrialEndsAt:      asOf.Add(3 * 24 * time.Hour),
			Plan:             "starter",
			Period:           "monthly",
			BillingCurrency:  &gbp,
			PriceTier:        subscription.PriceTierDeveloped,
			HasPaymentMethod: false,
			Status:           "trialing",
		},
	}
	trials := &stubTrialLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			Amount *struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.NotNil(t, body.Data[0].Amount, "a priced plan must carry an amount")
	require.Equal(t, int64(1500), body.Data[0].Amount.Amount)
	require.Equal(t, "GBP", body.Data[0].Amount.Currency, "currency on the wire must be uppercase")
}

// TestBillingTrialsTrialPlanOmitsAmount proves a row with plan="trial"
// carries NO `amount` key at all — asserted on the raw JSON body, not a
// decoded struct, so a null or a zeroed amount would also fail this. The
// catalog has no Price objects for plan="trial" by design (#328's
// correction): subscriptions reach plan="trial" by
// internal/subscription/service.go:124, a path distinct from a merchant
// choosing starter/studio/pro.
func TestBillingTrialsTrialPlanOmitsAmount(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	gbp := "gbp"
	rows := []trial.ExpiringRow{
		{
			TenantID:         "t-trial",
			StoreID:          "s-trial",
			TrialEndsAt:      asOf.Add(3 * 24 * time.Hour),
			Plan:             "trial",
			Period:           "monthly",
			BillingCurrency:  &gbp,
			PriceTier:        subscription.PriceTierDeveloped,
			HasPaymentMethod: false,
			Status:           "trialing",
		},
	}
	trials := &stubTrialLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "\"amount\"",
		"plan=trial has no catalog price; the row must omit the amount key entirely, not null it out")
}

// TestBillingTrialsPPPTierResolvesFromPPPTable proves a PPP-tier trial row
// resolves from the PPP price table, not the developed one — the exact
// failure mode #328 called out: guessing the tier wrong produces a
// wrong-but-plausible number, the worst outcome available here.
func TestBillingTrialsPPPTierResolvesFromPPPTable(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	inr := "inr"
	rows := []trial.ExpiringRow{
		{
			TenantID:         "t-ppp",
			StoreID:          "s-ppp",
			TrialEndsAt:      asOf.Add(3 * 24 * time.Hour),
			Plan:             "starter",
			Period:           "monthly",
			BillingCurrency:  &inr,
			PriceTier:        subscription.PriceTierPPP,
			HasPaymentMethod: false,
			Status:           "trialing",
		},
	}
	trials := &stubTrialLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			Amount *struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.NotNil(t, body.Data[0].Amount)
	require.Equal(t, int64(99900), body.Data[0].Amount.Amount,
		"PPP tier must resolve from the PPP table (99900), not the developed table")
	require.Equal(t, "INR", body.Data[0].Amount.Currency)
}

// TestCrossEndpointAmountAgreement is the cross-endpoint invariant #328
// exists to guarantee: the same plan/period/currency/tier resolves to the
// SAME amount whether it is read through /admin/billing/trials or
// /admin/billing/subscriptions. Both handlers are driven against ONE shared
// row in this single test — two separate assertions against the same
// catalog, each configured independently, would still pass even if the two
// handlers resolved money differently, which is the failure mode that let a
// structurally-zero counter ship in this codebase (#282) and the reasoning
// #328 explicitly calls out for why one resolver must serve both endpoints.
func TestCrossEndpointAmountAgreement(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	gbp := "gbp"
	tenantID := uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	storeID := uuid.MustParse("aaaaaaaa-4f89-11d3-9a0c-0305e82c3301")

	// --- drive /admin/billing/trials ---
	trialsRows := []trial.ExpiringRow{
		{
			TenantID:         tenantID.String(),
			StoreID:          storeID.String(),
			TrialEndsAt:      asOf.Add(3 * 24 * time.Hour),
			Plan:             "starter",
			Period:           "monthly",
			BillingCurrency:  &gbp,
			PriceTier:        subscription.PriceTierDeveloped,
			HasPaymentMethod: false,
			Status:           "trialing",
		},
	}
	trials := &stubTrialLister{rows: trialsRows, total: 1}
	trialsDir := &stubBillingDirectory{}
	trialsRec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, trialsDir).ServeHTTP(trialsRec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))
	require.Equal(t, http.StatusOK, trialsRec.Code)

	var trialsBody struct {
		Data []struct {
			Amount *struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(trialsRec.Body.Bytes(), &trialsBody))
	require.Len(t, trialsBody.Data, 1)
	require.NotNil(t, trialsBody.Data[0].Amount)

	// --- drive /admin/billing/subscriptions over the SAME plan/period/
	// currency/tier ---
	subsRows := []subscription.StoreSubscription{
		{
			TenantID:           tenantID,
			StoreID:            storeID,
			Plan:               subscription.PlanStarter,
			Status:             subscription.StatusTrialing,
			SubscriptionPeriod: subscription.PeriodMonthly,
			BillingCurrency:    &gbp,
			PriceTier:          subscription.PriceTierDeveloped,
			CancelAtPeriodEnd:  false,
		},
	}
	subs := &stubSubscriptionLister{rows: subsRows, total: 1}
	subsDir := &stubBillingDirectory{}
	subsRec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, subsDir).ServeHTTP(subsRec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))
	require.Equal(t, http.StatusOK, subsRec.Code)

	var subsBody struct {
		Data []struct {
			Amount *struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(subsRec.Body.Bytes(), &subsBody))
	require.Len(t, subsBody.Data, 1)
	require.NotNil(t, subsBody.Data[0].Amount)

	require.Equal(t, subsBody.Data[0].Amount.Amount, trialsBody.Data[0].Amount.Amount,
		"trials and subscriptions must agree on the resolved amount for the same plan/period/currency/tier")
	require.Equal(t, subsBody.Data[0].Amount.Currency, trialsBody.Data[0].Amount.Currency,
		"trials and subscriptions must agree on the resolved currency")
}

// The cross-endpoint invariant (#282's original defect): /admin/kpis's
// trials_expiring must equal this endpoint's pagination.total at the
// default window. Both handlers are driven against ONE shared fixture in
// this single test — two separate assertions against two independently
// configured stubs would still pass even if the handlers silently read
// different sources, which is exactly how a structurally-zero counter
// shipped before (#282).
func TestKPIsTrialsExpiringMatchesBillingTrialsPaginationTotal(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	rows := billingTrialsFixtureRows(asOf)

	// sharedFixture backs BOTH the kpis handler's Subscriptions.CountExpiring
	// and the billing-trials handler's TrialLister.ListExpiring from the
	// exact same row slice, so the two totals can only ever agree by
	// construction if both handlers actually read it.
	sharedFixture := &sharedTrialsFixture{rows: rows}

	// --- drive /admin/billing/trials ---
	dir := &stubBillingDirectory{names: map[string]string{
		"3f2504e0-4f89-11d3-9a0c-0305e82c3301": "Acme Trading",
		"3f2504e0-4f89-11d3-9a0c-0305e82c3302": "Beta Goods",
	}}
	trialsRec := httptest.NewRecorder()
	billingTrialsRouter(t, sharedFixture, dir).ServeHTTP(trialsRec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))
	require.Equal(t, http.StatusOK, trialsRec.Code)

	var trialsBody struct {
		Pagination struct {
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(trialsRec.Body.Bytes(), &trialsBody))

	// --- drive /admin/kpis?keys=trials_expiring ---
	estate := &stubEstateCounts{counts: &estatecounts.Counts{}}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}
	kpisRec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, sharedFixture).ServeHTTP(kpisRec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis?keys=trials_expiring", nil))
	require.Equal(t, http.StatusOK, kpisRec.Code)

	var kpisBody struct {
		Data map[string]int64 `json:"data"`
	}
	require.NoError(t, json.Unmarshal(kpisRec.Body.Bytes(), &kpisBody))

	require.Equal(t, kpisBody.Data["trials_expiring"], trialsBody.Pagination.Total,
		"kpis trials_expiring and billing-trials pagination.total must agree at the default window")
}

// TestBillingTrialsDaysRemainingUsesQueryInstantNotWallClock guards against
// days_remaining being computed from a fresh time.Now() rather than the
// same asOf instant used for the query (the mutation this project has
// already shipped once for kpis.go's `now`). This is deliberately NOT a
// golden-fixture assertion: billingTrialsFixtureAsOf is a fixed calendar
// instant, so a mutation to time.Now() only fails the golden test while the
// wall clock happens to land in the narrow window that reproduces the same
// days_remaining values — that window drifts and eventually closes. Here
// asOf is captured as time.Now() plus a 500-day offset at the moment the
// test runs, so the gap between asOf and whatever time.Now() the mutation
// reads back is always ~500 days, never day-granularity-close, on any
// calendar date this test executes.
func TestBillingTrialsDaysRemainingUsesQueryInstantNotWallClock(t *testing.T) {
	const offset = 500 * 24 * time.Hour
	asOf := time.Now().Add(offset)
	trialEndsAt := asOf.Add(3 * 24 * time.Hour)

	rows := []trial.ExpiringRow{
		{TenantID: "t-1", StoreID: "s-1", TrialEndsAt: trialEndsAt, Plan: "trial", Period: "monthly", Status: "trialing"},
	}
	trials := &stubTrialLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouterAt(t, trials, dir, asOf).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			DaysRemaining int `json:"days_remaining"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Equal(t, 3, body.Data[0].DaysRemaining,
		"days_remaining must be computed from the query's asOf instant, not a fresh time.Now()")
}

// TestBillingTrialsDaysRemainingFloorsAtZero covers daysRemaining's d <= 0
// branch: a trial ending at or before asOf must report 0, never a negative
// number.
func TestBillingTrialsDaysRemainingFloorsAtZero(t *testing.T) {
	asOf := billingTrialsFixtureAsOf
	rows := []trial.ExpiringRow{
		{TenantID: "t-1", StoreID: "s-1", TrialEndsAt: asOf, Plan: "trial", Period: "monthly", Status: "trialing"},
		{TenantID: "t-2", StoreID: "s-2", TrialEndsAt: asOf.Add(-2 * time.Hour), Plan: "trial", Period: "monthly", Status: "trialing"},
	}
	trials := &stubTrialLister{rows: rows, total: 2}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/trials", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			DaysRemaining int `json:"days_remaining"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	require.Equal(t, 0, body.Data[0].DaysRemaining, "trial ending exactly at asOf must report 0, not negative")
	require.Equal(t, 0, body.Data[1].DaysRemaining, "trial ended before asOf must floor at 0, not negative")
}

// sharedTrialsFixture implements both platformadmin.TrialLister (for the
// billing-trials endpoint) and platformadmin.Subscriptions (for the kpis
// endpoint) over the SAME row slice, so the cross-endpoint invariant test
// above is structurally forced to fail if either handler stops reading it.
type sharedTrialsFixture struct {
	rows []trial.ExpiringRow
}

func (f *sharedTrialsFixture) ListExpiring(_ context.Context, _ *gorm.DB, _ time.Time, _ time.Duration, _, _ int, _ trial.ListOptions) ([]trial.ExpiringRow, int64, error) {
	return f.rows, int64(len(f.rows)), nil
}

func (f *sharedTrialsFixture) CountExpiring(_ context.Context, _ *gorm.DB, _ time.Time, _ time.Duration) (int64, error) {
	return int64(len(f.rows)), nil
}

// TestBillingTrialsDaysRemainingMatchesMerchantFormula pins the console's
// days_remaining to the exact formula enrichTrialBanner uses
// (internal/handlers/admin/subscription.go): floor(hours/24), bumped to 1
// only when 0 < hours < 24, floored at zero. Before this fix the console
// used math.Ceil(hours/24), which reports one day MORE than the merchant
// sees for any non-integral remainder — an operator reading the console
// would quote a different number than the merchant's own dashboard.
//
// Table mirrors enrichTrialBanner's own case list, including the
// 0 < hours < 24 bump and the floor-at-zero case already covered by
// TestBillingTrialsDaysRemainingFloorsAtZero.
func TestBillingTrialsDaysRemainingMatchesMerchantFormula(t *testing.T) {
	asOf := billingTrialsFixtureAsOf

	cases := []struct {
		name      string
		hoursLeft time.Duration
		want      int
	}{
		{"0h left (exactly at asOf)", 0, 0},
		{"1h left", 1 * time.Hour, 1},
		{"23h left", 23 * time.Hour, 1},
		{"24h left", 24 * time.Hour, 1},
		{"25h left", 25 * time.Hour, 1},
		{"47h left", 47 * time.Hour, 1},
		{"48h left", 48 * time.Hour, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trialEndsAt := asOf.Add(tc.hoursLeft)
			rows := []trial.ExpiringRow{
				{TenantID: "t-1", StoreID: "s-1", TrialEndsAt: trialEndsAt, Plan: "trial", Period: "monthly", Status: "trialing"},
			}
			trials := &stubTrialLister{rows: rows, total: 1}
			dir := &stubBillingDirectory{}

			rec := httptest.NewRecorder()
			billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
				http.MethodGet, "/admin/billing/trials", nil))

			require.Equal(t, http.StatusOK, rec.Code)

			var body struct {
				Data []struct {
					DaysRemaining int `json:"days_remaining"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Data, 1)
			require.Equal(t, tc.want, body.Data[0].DaysRemaining)
		})
	}
}

// The query parameter must reach the lister, and the row must be labelled
// on the wire. Both directions matter: a handler that always passed true
// would widen a live contract, and one that never passed it would leave
// #358's endpoint undiscoverable.
func TestBillingTrials_IncludeStripeManagedReachesTheLister(t *testing.T) {
	dir := &stubBillingDirectory{names: map[string]string{}}

	t.Run("with the flag", func(t *testing.T) {
		rows := billingTrialsFixtureRows(billingTrialsFixtureAsOf)
		rows[0].StripeManaged = true
		trials := &stubTrialLister{rows: rows, total: int64(len(rows))}

		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials?include_stripe_managed=true", nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.True(t, trials.gotOpts.IncludeStripeManaged)
		// Raw bytes: a decoded map cannot distinguish an absent key from a
		// false one, and telling those apart is this field's whole job.
		require.Contains(t, rec.Body.String(), `"stripe_managed":true`)
	})

	t.Run("without the flag the default is unchanged", func(t *testing.T) {
		trials := &stubTrialLister{rows: nil, total: 0}
		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.False(t, trials.gotOpts.IncludeStripeManaged,
			"the default must stay #285's shipped contract")
	})

	t.Run("anything other than true is false", func(t *testing.T) {
		trials := &stubTrialLister{rows: nil, total: 0}
		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials?include_stripe_managed=1", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.False(t, trials.gotOpts.IncludeStripeManaged,
			"a widening flag must require the exact value, never a truthy-looking one")
	})

	t.Run("a card-less row is labelled false, not omitted", func(t *testing.T) {
		rows := billingTrialsFixtureRows(billingTrialsFixtureAsOf)
		for i := range rows {
			rows[i].StripeManaged = false
		}
		trials := &stubTrialLister{rows: rows, total: int64(len(rows))}
		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials?include_stripe_managed=true", nil))
		require.Contains(t, rec.Body.String(), `"stripe_managed":false`,
			"every row states its kind; an omitted false is indistinguishable from an older server")
	})
}
