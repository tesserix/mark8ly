package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// stubSubscriptionLister records the params ListAllSubscriptions was called
// with and returns canned rows/total, mirroring stubTrialLister in
// billing_trials_test.go.
type stubSubscriptionLister struct {
	rows  []subscription.StoreSubscription
	total int64
	err   error

	gotFilter subscription.CrossTenantFilter
	calls     int
}

func (s *stubSubscriptionLister) ListAllSubscriptions(_ context.Context, _ *gorm.DB, f subscription.CrossTenantFilter) ([]subscription.StoreSubscription, int64, error) {
	s.calls++
	s.gotFilter = f
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.rows, s.total, nil
}

// billingSubscriptionsRouter builds a router around the handler under test.
func billingSubscriptionsRouter(t *testing.T, subs platformadmin.SubscriptionLister, dir platformadmin.TenantDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBillingSubscriptionsHandler(subs, dir, nil, nil).Register(r.Group(""))
	return r
}

var (
	subsFixtureTenantA = uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	subsFixtureStoreA  = uuid.MustParse("aaaaaaaa-4f89-11d3-9a0c-0305e82c3301")
	subsFixtureTenantB = uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3302")
	subsFixtureStoreB  = uuid.MustParse("bbbbbbbb-4f89-11d3-9a0c-0305e82c3302")
)

// billingSubscriptionsFixtureRows backs the golden fixture. Row A is a
// priced, active subscription with a current_period_end. Row B is a `trial`
// plan — the catalog has no trial price by design — with a NULL
// current_period_end and a tenant absent from the directory stub, covering
// every omission rule in one fixture.
func billingSubscriptionsFixtureRows() []subscription.StoreSubscription {
	gbp := "gbp"
	periodEnd := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	return []subscription.StoreSubscription{
		{
			TenantID:           subsFixtureTenantA,
			StoreID:            subsFixtureStoreA,
			Plan:               subscription.PlanStarter,
			Status:             subscription.StatusActive,
			SubscriptionPeriod: subscription.PeriodMonthly,
			BillingCurrency:    &gbp,
			PriceTier:          subscription.PriceTierDeveloped,
			CurrentPeriodEnd:   &periodEnd,
			CancelAtPeriodEnd:  false,
		},
		{
			TenantID:           subsFixtureTenantB,
			StoreID:            subsFixtureStoreB,
			Plan:               subscription.PlanTrial,
			Status:             subscription.StatusTrialing,
			SubscriptionPeriod: subscription.PeriodMonthly,
			BillingCurrency:    nil,
			PriceTier:          subscription.PriceTierDeveloped,
			CurrentPeriodEnd:   nil,
			CancelAtPeriodEnd:  false,
		},
	}
}

// THE test. Real handler output compared to the committed contract.
func TestBillingSubscriptionsMatchesContract(t *testing.T) {
	rows := billingSubscriptionsFixtureRows()
	subs := &stubSubscriptionLister{rows: rows, total: int64(len(rows))}
	dir := &stubBillingDirectory{names: map[string]string{
		subsFixtureTenantA.String(): "Acme Trading",
		// subsFixtureTenantB deliberately absent from the directory.
	}}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/billing_subscriptions_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// A row whose plan has no catalog price (plan="trial") must have no
// `amount` key at all — asserted on the raw body, not a decoded struct,
// since a decoded *money field would silently read back as nil either way.
func TestBillingSubscriptionsUnpricedPlanOmitsAmountKey(t *testing.T) {
	rows := []subscription.StoreSubscription{
		{
			TenantID:           subsFixtureTenantB,
			StoreID:            subsFixtureStoreB,
			Plan:               subscription.PlanTrial,
			Status:             subscription.StatusTrialing,
			SubscriptionPeriod: subscription.PeriodMonthly,
			PriceTier:          subscription.PriceTierDeveloped,
		},
	}
	subs := &stubSubscriptionLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	_, hasAmount := body.Data[0]["amount"]
	require.False(t, hasAmount, "a plan with no catalog price must carry no amount key at all")
}

// A row with a priced plan carries `amount` with an uppercase currency.
func TestBillingSubscriptionsPricedPlanCarriesUppercaseCurrency(t *testing.T) {
	gbp := "gbp"
	rows := []subscription.StoreSubscription{
		{
			TenantID:           subsFixtureTenantA,
			StoreID:            subsFixtureStoreA,
			Plan:               subscription.PlanStarter,
			Status:             subscription.StatusActive,
			SubscriptionPeriod: subscription.PeriodMonthly,
			BillingCurrency:    &gbp,
			PriceTier:          subscription.PriceTierDeveloped,
		},
	}
	subs := &stubSubscriptionLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

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
	require.Equal(t, int64(1500), body.Data[0].Amount.Amount)
	require.Equal(t, "GBP", body.Data[0].Amount.Currency)
}

func TestBillingSubscriptionsEmptyIsArray(t *testing.T) {
	subs := &stubSubscriptionLister{rows: nil, total: 0}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":[],"pagination":{"page":1,"limit":50,"total":0}}`, rec.Body.String())

	// A page with no rows must not call the directory at all.
	require.Equal(t, 0, dir.calls, "empty page must not call tenantdirectory.List")
}

// Each of the eight subscription.Status* constants must be accepted as a
// `status` filter. Table-driven over the constants themselves (not a
// hand-copied list of strings) so a ninth status added to models.go without
// handler support fails this test rather than being silently unsupported.
func TestBillingSubscriptionsAcceptsAllEightStatuses(t *testing.T) {
	statuses := []subscription.SubscriptionStatus{
		subscription.StatusSignup,
		subscription.StatusTrialing,
		subscription.StatusActive,
		subscription.StatusPastDue,
		subscription.StatusPaymentActionRequired,
		subscription.StatusCancelScheduled,
		subscription.StatusExpired,
		subscription.StatusStoreClosed,
	}
	require.Len(t, statuses, 8, "this test must cover exactly the eight platform-console-facing statuses")

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			subs := &stubSubscriptionLister{}
			dir := &stubBillingDirectory{}

			rec := httptest.NewRecorder()
			billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
				http.MethodGet, "/admin/billing/subscriptions?status="+string(status), nil))

			require.Equal(t, http.StatusOK, rec.Code, "status=%s must be accepted", status)
			require.Equal(t, string(status), subs.gotFilter.Status)
		})
	}
}

// Each of the five subscription.Plan* constants must be accepted as a
// `plan` filter.
func TestBillingSubscriptionsAcceptsAllFivePlans(t *testing.T) {
	plans := []subscription.SubscriptionPlan{
		subscription.PlanTrial,
		subscription.PlanStarter,
		subscription.PlanStudio,
		subscription.PlanPro,
		subscription.PlanMarketplace,
	}
	require.Len(t, plans, 5)

	for _, plan := range plans {
		t.Run(string(plan), func(t *testing.T) {
			subs := &stubSubscriptionLister{}
			dir := &stubBillingDirectory{}

			rec := httptest.NewRecorder()
			billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
				http.MethodGet, "/admin/billing/subscriptions?plan="+string(plan), nil))

			require.Equal(t, http.StatusOK, rec.Code, "plan=%s must be accepted", plan)
			require.Equal(t, string(plan), subs.gotFilter.Plan)
		})
	}
}

// An unrecognised status must be 400 validation_error, never an empty 200 —
// an empty list must mean "none match", never "I did not understand you".
func TestBillingSubscriptionsUnknownStatusIs400(t *testing.T) {
	subs := &stubSubscriptionLister{}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions?status=nonsense", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "validation_error", errorCode(t, rec))
	require.Equal(t, 0, subs.calls, "an unrecognised status must never reach the repository")
}

// An unrecognised plan must be 400 validation_error, likewise never an
// empty 200.
func TestBillingSubscriptionsUnknownPlanIs400(t *testing.T) {
	subs := &stubSubscriptionLister{}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions?plan=nonsense", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "validation_error", errorCode(t, rec))
	require.Equal(t, 0, subs.calls, "an unrecognised plan must never reach the repository")
}

// limit=9999 must clamp to 500 both on the value sent to the repository AND
// on pagination.limit in the response — Task 2's repository test only
// caught a clamp removal incidentally via slice capacity; here it is cheap
// to assert both properly.
func TestBillingSubscriptionsLimitClampsAndReportsEffectiveValue(t *testing.T) {
	subs := &stubSubscriptionLister{}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions?limit=9999", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 500, subs.gotFilter.Limit, "limit must clamp to 500 before reaching the stub")

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
func TestBillingSubscriptionsOneTenantLookupPerPageDeduplicated(t *testing.T) {
	shared := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	other := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	rows := []subscription.StoreSubscription{
		{TenantID: shared, StoreID: uuid.New(), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
		{TenantID: shared, StoreID: uuid.New(), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
		{TenantID: other, StoreID: uuid.New(), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
		{TenantID: shared, StoreID: uuid.New(), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
	}
	subs := &stubSubscriptionLister{rows: rows, total: 4}
	dir := &stubBillingDirectory{names: map[string]string{
		shared.String(): "Shared Co", other.String(): "Other Co",
	}}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, dir.calls, "must make exactly one tenantdirectory.List call per page")
	require.ElementsMatch(t, []string{shared.String(), other.String()}, dir.gotIDs, "ids sent upstream must be deduplicated")
}

// A tenant absent from the directory response omits tenant_name; the row
// still appears.
func TestBillingSubscriptionsMissingTenantOmitsName(t *testing.T) {
	ghost := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	rows := []subscription.StoreSubscription{
		{TenantID: ghost, StoreID: uuid.New(), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
	}
	subs := &stubSubscriptionLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{names: map[string]string{}} // directory knows nothing

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

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
	require.Equal(t, ghost.String(), body.Data[0].TenantID)
}

func TestBillingSubscriptionsUpstreamUnavailableIs503NoDataKey(t *testing.T) {
	rows := []subscription.StoreSubscription{
		{TenantID: uuid.New(), StoreID: uuid.New(), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
	}
	subs := &stubSubscriptionLister{rows: rows, total: 1}
	dir := &stubBillingDirectory{err: tenantdirectory.ErrUnavailable}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasData := raw["data"]
	require.False(t, hasData, "a 503 must never carry a data key — no partial result")
}

func TestBillingSubscriptionsListErrorIs500(t *testing.T) {
	subs := &stubSubscriptionLister{err: errBoom}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
}

// Rows come back from ListAllSubscriptions already ordered (created_at
// DESC, per the repository). The handler must preserve that order, not
// resort it.
func TestBillingSubscriptionsPreservesOrder(t *testing.T) {
	rows := []subscription.StoreSubscription{
		{TenantID: uuid.New(), StoreID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
		{TenantID: uuid.New(), StoreID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
		{TenantID: uuid.New(), StoreID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), Plan: subscription.PlanTrial, Status: subscription.StatusTrialing, SubscriptionPeriod: subscription.PeriodMonthly},
	}
	subs := &stubSubscriptionLister{rows: rows, total: 3}
	dir := &stubBillingDirectory{}

	rec := httptest.NewRecorder()
	billingSubscriptionsRouter(t, subs, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/billing/subscriptions", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			StoreID string `json:"store_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 3)
	require.Equal(t, rows[0].StoreID.String(), body.Data[0].StoreID)
	require.Equal(t, rows[1].StoreID.String(), body.Data[1].StoreID)
	require.Equal(t, rows[2].StoreID.String(), body.Data[2].StoreID)
}
