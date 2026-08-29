package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/routes"
)

// #323: RequireInternalAuthStrict is unit-tested, but nothing asserted
// WHICH routes sit behind it. Moving onboardingHandler.RegisterAnalytics
// from the strict group to the permissive one left `go build` and
// `go test ./...` entirely green — the downgrade shipped undetected.
//
// These tests exercise routes.MountInternal, the function main.go now
// calls, so the assertion covers the real mapping rather than a
// reconstruction of it.

const probeSecret = "s3cr3t"

// The expected mapping is declared HERE, not read from the package under
// test. Deriving it from routes.StrictSlots() would let an all-permissive
// implementation pass vacuously — the strict loop would simply iterate
// over nothing. Pinning it in the test means moving a handler between
// guards forces an edit to this list, which is the change a reviewer
// needs to see.
var (
	wantStrict = []string{
		"TenantDirectory", "TenantLifecycle", "OnboardingAnalytics",
		"EstateCounts", "EstateUsers", "AccountOperator",
	}
	wantPermissive = []string{
		"Tenant", "Store", "Invitation", "Auth", "MerchantAccount", "Notification",
	}
)

// The package's own declaration must match what this file expects, so the
// behavioural loops below cannot silently cover the wrong set.
func TestMountInternal_DeclaredSlotsMatchTheExpectedMapping(t *testing.T) {
	require.ElementsMatch(t, wantStrict, routes.StrictSlots(),
		"a handler moved between guards must be an explicit edit here")
	require.ElementsMatch(t, wantPermissive, routes.PermissiveSlots())
	require.NotEmpty(t, wantStrict, "a vacuous strict loop would prove nothing")
}

// probe returns a registrar that mounts GET /<name> and records that it
// was called, so a test can address each slot's route individually.
func probe(name string, calls *[]string) routes.Registrar {
	return func(g *gin.RouterGroup) {
		*calls = append(*calls, name)
		g.GET("/"+name, func(c *gin.Context) { c.String(http.StatusOK, name) })
	}
}

// fill returns an InternalHandlers with every field set to a probe named
// after the field, so no slot can be silently skipped.
func fill(calls *[]string) routes.InternalHandlers {
	var h routes.InternalHandlers
	v := reflect.ValueOf(&h).Elem()
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		v.Field(i).Set(reflect.ValueOf(probe(name, calls)))
	}
	return h
}

func get(t *testing.T, r *gin.Engine, path, header string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/internal/"+path, nil)
	if header != "" {
		req.Header.Set("X-Internal-Auth", header)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func newEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// The property #277 verified by hand, made permanent: with no secret
// configured, every estate-wide route refuses rather than serving the lot.
func TestMountInternal_StrictRoutesRefuseWhenSecretUnset(t *testing.T) {
	var calls []string
	r := newEngine()
	routes.MountInternal(r, "", fill(&calls))

	for _, name := range wantStrict {
		require.Equal(t, http.StatusServiceUnavailable, get(t, r, name, ""),
			"%s returns estate-wide data and must be fail-closed: an unconfigured "+
				"deploy has to refuse, not serve the whole platform", name)
	}
}

// The other half of the mapping. A permissive route is already scoped by
// something the caller had to know, so an empty secret must NOT break it
// — that is the dev/cutover escape hatch RequireInternalAuth exists for.
func TestMountInternal_PermissiveRoutesStillServeWhenSecretUnset(t *testing.T) {
	var calls []string
	r := newEngine()
	routes.MountInternal(r, "", fill(&calls))

	for _, name := range wantPermissive {
		require.Equal(t, http.StatusOK, get(t, r, name, ""),
			"%s is scoped by its caller; an empty secret must stay a no-op for it", name)
	}
}

// Guards against the strict test passing for the wrong reason — a 503 for
// every request regardless of configuration would satisfy it too.
func TestMountInternal_BothGroupsServeWithTheRightSecret(t *testing.T) {
	var calls []string
	r := newEngine()
	routes.MountInternal(r, probeSecret, fill(&calls))

	for _, name := range append(append([]string{}, wantStrict...), wantPermissive...) {
		require.Equal(t, http.StatusOK, get(t, r, name, probeSecret), "%s with the secret", name)
		require.Equal(t, http.StatusUnauthorized, get(t, r, name, "wrong"), "%s with a bad secret", name)
	}
}

// A field added to InternalHandlers and never mounted would be a route
// silently absent from the running service. Reflection over the struct
// means adding a field forces a decision about which guard it belongs
// behind, rather than defaulting to neither.
func TestMountInternal_MountsEveryDeclaredHandler(t *testing.T) {
	var calls []string
	h := fill(&calls)
	routes.MountInternal(newEngine(), probeSecret, h)

	declared := reflect.TypeOf(h).NumField()
	require.Len(t, calls, declared,
		"every field of InternalHandlers must be mounted exactly once")

	slots := append(append([]string{}, wantStrict...), wantPermissive...)
	require.ElementsMatch(t, slots, calls,
		"the expected mapping and the fields actually mounted must agree")
}

// authHandler and the merchant account routes are conditional in main.go,
// so a nil registrar is a normal configuration, not a bug.
func TestMountInternal_SkipsNilRegistrars(t *testing.T) {
	require.NotPanics(t, func() {
		routes.MountInternal(newEngine(), probeSecret, routes.InternalHandlers{})
	}, "an unconfigured optional handler must be skipped, not dereferenced")
}

var _ = fmt.Sprintf
