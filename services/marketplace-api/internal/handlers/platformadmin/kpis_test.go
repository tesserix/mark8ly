package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/estatecounts"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// stubEstateCounts is a canned estatecounts.Client stand-in.
type stubEstateCounts struct {
	counts *estatecounts.Counts
	err    error
}

func (s *stubEstateCounts) Get(_ context.Context) (*estatecounts.Counts, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.counts == nil {
		s.counts = &estatecounts.Counts{}
	}
	return s.counts, nil
}

// stubSubscriptions is a canned subscription.Repository stand-in, narrowed
// to the one method the kpis handler calls.
type stubSubscriptions struct {
	count   int64
	err     error
	gotAsOf time.Time
}

func (s *stubSubscriptions) CountTrialsExpiring(_ context.Context, _ *gorm.DB, asOf time.Time) (int64, error) {
	s.gotAsOf = asOf
	if s.err != nil {
		return 0, s.err
	}
	return s.count, nil
}

// kpisFixture builds one consistent set of stubs used across the happy-path
// tests, so the golden fixture and the anti-drift test agree on values.
func kpisFixture() (*stubEstateCounts, *stubFunnelClient, *stubSubscriptions) {
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 42, StoresActive: 57}}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{
		FunnelCounts: onboardingfunnel.FunnelCounts{InFlight: 34},
	}}
	subs := &stubSubscriptions{count: 9}
	return estate, funnel, subs
}

func kpisRouter(t *testing.T, estate platformadmin.EstateCounts, funnel platformadmin.OnboardingFunnel, subs platformadmin.Subscriptions) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewKPIsHandler(estate, funnel, subs, nil, nil).Register(r.Group(""))
	return r
}

func kpiErrorKey(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Key string `json:"key"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Key
}

func kpiErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Message
}

// Registry-driven completeness: iterate KPIRegistry so a future key added
// there without handler support fails this test. Every instrumented key
// must appear in the 200 payload when requested alone; every uninstrumented
// key must 501 when requested by name.
func TestKPIsRegistryDrivenCompleteness(t *testing.T) {
	for _, key := range platformadmin.KPIRegistry {
		t.Run(key.Name, func(t *testing.T) {
			estate, funnel, subs := kpisFixture()

			rec := httptest.NewRecorder()
			kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
				http.MethodGet, "/admin/kpis?keys="+key.Name, nil))

			if key.Instrumented {
				require.Equal(t, http.StatusOK, rec.Code)
				var body struct {
					Data map[string]json.RawMessage `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				_, ok := body.Data[key.Name]
				require.True(t, ok, "instrumented key %q missing from 200 payload", key.Name)
			} else {
				require.Equal(t, http.StatusNotImplemented, rec.Code)
				require.Equal(t, key.Name, kpiErrorKey(t, rec))
			}
		})
	}
}

func TestKPIsUnknownKeyIs501NotARecognisedKey(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis?keys=not_a_real_kpi", nil))

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Equal(t, "not_implemented", errorCode(t, rec))
	require.Equal(t, "not_a_real_kpi", kpiErrorKey(t, rec))
	require.Contains(t, kpiErrorMessage(t, rec), "not a recognised key")
}

func TestKPIsKnownUninstrumentedKeyIs501KnownButNotInstrumented(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis?keys=gmv_today", nil))

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Equal(t, "not_implemented", errorCode(t, rec))
	require.Equal(t, "gmv_today", kpiErrorKey(t, rec))
	require.Contains(t, kpiErrorMessage(t, rec), "known but not instrumented")
}

// The two 501 messages are the only thing distinguishing the cases for a
// human reading logs — assert they actually differ.
func TestKPIs501MessagesDifferBetweenUnknownAndUninstrumented(t *testing.T) {
	estate, funnel, subs := kpisFixture()
	router := kpisRouter(t, estate, funnel, subs)

	unknownRec := httptest.NewRecorder()
	router.ServeHTTP(unknownRec, httptest.NewRequest(http.MethodGet, "/admin/kpis?keys=not_a_real_kpi", nil))

	uninstrumentedRec := httptest.NewRecorder()
	router.ServeHTTP(uninstrumentedRec, httptest.NewRequest(http.MethodGet, "/admin/kpis?keys=gmv_today", nil))

	require.NotEqual(t, kpiErrorMessage(t, unknownRec), kpiErrorMessage(t, uninstrumentedRec))
}

// A mixed request with one good key and one uninstrumented key must 501 as
// a whole — never a partial 200 with just the good key.
func TestKPIsMixedRequestWithUninstrumentedKeyIs501NotPartial200(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis?keys=tenants_active,gmv_today", nil))

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.NotContains(t, rec.Body.String(), `"data"`)
}

// Anti-drift: onboarding_in_flight must equal the InFlight the funnel stub
// reports, from the SAME stub value used to build the stub — so a future
// handler that recomputes rather than reuses fails this test.
func TestKPIsOnboardingInFlightMatchesFunnelStubExactly(t *testing.T) {
	const wantInFlight = int64(77)
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 1, StoresActive: 1}}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{
		FunnelCounts: onboardingfunnel.FunnelCounts{InFlight: wantInFlight},
	}}
	subs := &stubSubscriptions{count: 1}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			OnboardingInFlight int64 `json:"onboarding_in_flight"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, wantInFlight, body.Data.OnboardingInFlight)
	require.Equal(t, funnel.funnel.InFlight, body.Data.OnboardingInFlight)
}

// counterKeyNames lists every JSON key that could carry a real counter
// value in the 200 payload. Used to assert a 503 body carries NONE of
// them — a partial object is the failure mode these tests exist to catch.
var counterKeyNames = []string{"tenants_active", "stores_active", "onboarding_in_flight", "trials_expiring"}

func assertNoCounterKeys(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, name := range counterKeyNames {
		require.NotContains(t, rec.Body.String(), name)
	}
}

func TestKPIsEstateUnavailableIs503WithNoPartialBody(t *testing.T) {
	estate := &stubEstateCounts{err: estatecounts.ErrUnavailable}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}
	subs := &stubSubscriptions{count: 1}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
	assertNoCounterKeys(t, rec)
}

func TestKPIsFunnelUnavailableIs503WithNoPartialBody(t *testing.T) {
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 1, StoresActive: 1}}
	funnel := &stubFunnelClient{err: onboardingfunnel.ErrUnavailable}
	subs := &stubSubscriptions{count: 1}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
	assertNoCounterKeys(t, rec)
}

func TestKPIsSubscriptionRepositoryErrorIs503WithNoPartialBody(t *testing.T) {
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 1, StoresActive: 1}}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}
	subs := &stubSubscriptions{err: errors.New("db exploded")}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
	assertNoCounterKeys(t, rec)
}

// Every value in the 200 payload must decode as a bare JSON integer — a
// float 4.0 or a string "4" must fail. Asserted on the RAW JSON: decoding
// into an int would hide both cases.
var rawIntegerPattern = regexp.MustCompile(`^-?[0-9]+$`)

func TestKPIsValuesAreRawIntegersNotFloatsOrStrings(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.Data)
	for name, raw := range body.Data {
		require.Truef(t, rawIntegerPattern.MatchString(string(raw)),
			"kpi %q raw value %q is not a bare JSON integer", name, string(raw))
	}
}

// THE test. Real handler output compared to the committed contract.
func TestKPIsMatchesContract(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/kpis_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}
