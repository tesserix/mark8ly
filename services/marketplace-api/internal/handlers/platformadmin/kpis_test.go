package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
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
	count     int64
	err       error
	gotAsOf   time.Time
	gotWindow time.Duration
}

func (s *stubSubscriptions) CountExpiring(_ context.Context, _ *gorm.DB, asOf time.Time, window time.Duration) (int64, error) {
	s.gotAsOf = asOf
	s.gotWindow = window
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

// wantKPIValue maps a fixture key to the distinct, non-zero value
// kpisFixture() reports for it, so the assertion below can check the exact
// value carried through the payload rather than merely the key's presence.
// A missing entry here for a key that IS instrumented is itself a test bug
// (see the require.Contains fallthrough below), which is intentional: this
// map must be kept in lockstep with kpisFixture.
var wantKPIValue = map[string]int64{
	"tenants_active":       42,
	"stores_active":        57,
	"onboarding_in_flight": 34,
	"trials_expiring":      9,
}

// Registry-driven completeness: iterate KPIRegistry so a future key added
// there without handler support fails this test. Every instrumented key
// must appear in the 200 payload, WITH THE EXACT VALUE the stub reports for
// it, when requested alone; every uninstrumented key must 501 when
// requested by name.
//
// Asserting only presence (as this test used to) lets a registry key with
// no handler support through: Go's zero value for an unpopulated map entry
// is 0, and 0 satisfies "key is present" every time. Asserting the exact,
// distinct, non-zero fixture value closes that gap — a fabricated 0 now
// fails on the value.
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
				raw, ok := body.Data[key.Name]
				require.True(t, ok, "instrumented key %q missing from 200 payload", key.Name)

				want, known := wantKPIValue[key.Name]
				require.True(t, known, "test bug: wantKPIValue has no entry for instrumented key %q — add one alongside kpisFixture", key.Name)
				require.JSONEq(t, fmt.Sprintf("%d", want), string(raw),
					"kpi %q carried the wrong value — a fabricated zero would also pass a presence-only check", key.Name)
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

// The 501 for an uninstrumented key must not depend on upstream health: the
// registry check runs before any gather, so this must stay 501 even when
// every upstream is down (which would otherwise produce a 503). If the
// check ever moved after the gather, this test would catch it turning into
// a 503.
func TestKPIsKnownUninstrumentedKeyIs501EvenWhenAllUpstreamsFail(t *testing.T) {
	estate := &stubEstateCounts{err: estatecounts.ErrUnavailable}
	funnel := &stubFunnelClient{err: onboardingfunnel.ErrUnavailable}
	subs := &stubSubscriptions{err: errors.New("db exploded")}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis?keys=gmv_today", nil))

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Equal(t, "not_implemented", errorCode(t, rec))
	require.Equal(t, "gmv_today", kpiErrorKey(t, rec))
	require.Contains(t, kpiErrorMessage(t, rec), "known but not instrumented")
}

// The two 501 messages are the only thing distinguishing the cases for a
// human reading logs. Comparing the two raw messages for inequality does
// NOT prove that: both messages interpolate their own key name
// ("not_a_real_kpi" vs "gmv_today"), so they differ no matter what the
// surrounding template text says — even if a reviewer made both templates
// byte-for-byte identical, the strings would still differ on the key alone
// and this test would still pass. Assert each message's distinguishing
// phrase directly instead, so deleting either phrase from its template
// fails this test.
func TestKPIs501MessagesDifferBetweenUnknownAndUninstrumented(t *testing.T) {
	estate, funnel, subs := kpisFixture()
	router := kpisRouter(t, estate, funnel, subs)

	unknownRec := httptest.NewRecorder()
	router.ServeHTTP(unknownRec, httptest.NewRequest(http.MethodGet, "/admin/kpis?keys=not_a_real_kpi", nil))

	uninstrumentedRec := httptest.NewRecorder()
	router.ServeHTTP(uninstrumentedRec, httptest.NewRequest(http.MethodGet, "/admin/kpis?keys=gmv_today", nil))

	unknownMsg := kpiErrorMessage(t, unknownRec)
	uninstrumentedMsg := kpiErrorMessage(t, uninstrumentedRec)

	require.Contains(t, unknownMsg, "not a recognised key")
	require.NotContains(t, unknownMsg, "known but not instrumented")

	require.Contains(t, uninstrumentedMsg, "known but not instrumented")
	require.NotContains(t, uninstrumentedMsg, "not a recognised key")
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

// Anti-drift: trials_expiring must equal the count the Subscriptions stub
// reports, from the SAME distinct, non-zero stub value used to build the
// stub — so a wiring regression that silently drops the source (and falls
// back to Go's zero value) fails this test. This handler shipped exactly
// that fabricated-zero bug once; this pins the wiring against a repeat.
//
// It also pins the *window* the handler passes to CountExpiring against
// trial.DefaultExpiryWindow specifically — not merely "non-zero". A
// plausible-but-wrong horizon (e.g. a reviewer hardcoding 48*time.Hour) is
// exactly the class of bug this counter has already shipped once, for a
// different argument (querying the wrong column). asOf is checked for
// recency: the handler wires h.now() (time.Now, not injectable through
// NewKPIsHandler) straight through, so this proves it is not the zero
// time.Time{} and was captured at call time, without pinning an exact
// instant the handler cannot be made to produce deterministically.
func TestKPIsTrialsExpiringMatchesSubscriptionsStubExactly(t *testing.T) {
	const wantTrialsExpiring = int64(23)
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 1, StoresActive: 1}}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{
		FunnelCounts: onboardingfunnel.FunnelCounts{InFlight: 1},
	}}
	subs := &stubSubscriptions{count: wantTrialsExpiring}

	before := time.Now()
	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			TrialsExpiring int64 `json:"trials_expiring"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, wantTrialsExpiring, body.Data.TrialsExpiring)
	require.Equal(t, subs.count, body.Data.TrialsExpiring)

	require.Equal(t, trial.DefaultExpiryWindow, subs.gotWindow,
		"handler must pass trial.DefaultExpiryWindow to CountExpiring, not a hardcoded or otherwise-derived horizon")
	require.WithinRange(t, subs.gotAsOf, before, time.Now(),
		"handler must pass a live now() to CountExpiring, not a zero-value time.Time")
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

// A bare subscription-repository (DB) error wraps neither ErrUnavailable
// sentinel, so per the spec (binding over the brief, which said 503 for
// every error) it becomes 500 internal_error, not 503: a DB error means our
// own service is broken, and 503 would misleadingly suggest retrying helps.
func TestKPIsSubscriptionRepositoryErrorIs500WithNoPartialBody(t *testing.T) {
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 1, StoresActive: 1}}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}
	subs := &stubSubscriptions{err: errors.New("db exploded")}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
	assertNoCounterKeys(t, rec)
}

// A non-404 4xx from platform-api (e.g. a wrong shared secret returning
// 401/403) wraps neither ErrUnavailable sentinel either, so it takes the
// same 500 branch: our own configuration is broken, and retrying a wrong
// secret never helps.
func TestKPIsEstateNon5xxErrorIs500NotServiceUnavailable(t *testing.T) {
	estate := &stubEstateCounts{err: errors.New("estatecounts: platform-api 401")}
	funnel := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}
	subs := &stubSubscriptions{count: 1}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
	assertNoCounterKeys(t, rec)
}

// The funnel client's equivalent non-5xx case takes the same 500 branch,
// covering both upstreams that can produce a non-404 4xx.
func TestKPIsFunnelNon5xxErrorIs500NotServiceUnavailable(t *testing.T) {
	estate := &stubEstateCounts{counts: &estatecounts.Counts{TenantsActive: 1, StoresActive: 1}}
	funnel := &stubFunnelClient{err: errors.New("onboardingfunnel: platform-api 403")}
	subs := &stubSubscriptions{count: 1}

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
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

// A ?keys= value made only of empty/comma segments (e.g. "?keys=,,") must
// behave exactly like an absent "keys" param: all instrumented keys, not a
// 501 for an empty-string key and not an empty payload. See the comment on
// parseKPIKeys for the documented contract this proves.
func TestKPIsKeysOnlyEmptySegmentsFallsBackToAllInstrumented(t *testing.T) {
	estate, funnel, subs := kpisFixture()

	rec := httptest.NewRecorder()
	kpisRouter(t, estate, funnel, subs).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/kpis?keys=,,", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	for name := range wantKPIValue {
		_, ok := body.Data[name]
		require.True(t, ok, "instrumented key %q missing when keys=,, falls back to all instrumented keys", name)
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
