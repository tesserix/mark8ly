package stores

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const upsertBody = `{"id":"11111111-1111-1111-1111-111111111111",
"tenant_id":"22222222-2222-2222-2222-222222222222","slug":"attacker",
"name":"Attacker","country_code":"AU","currency_code":"AUD","timezone":"UTC"}`

func postUpsert(t *testing.T, secret, sent string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	// repo is nil on purpose: a request that reaches the handler body would
	// panic, so a clean 401 is proof the guard rejected it before then.
	NewInternalHandler(nil).RegisterRoutes(r.Group("/internal"), secret)

	req := httptest.NewRequest("POST", "/internal/stores/upsert", bytes.NewBufferString(upsertBody))
	req.Header.Set("Content-Type", "application/json")
	if sent != "" {
		req.Header.Set("X-Internal-Auth", sent)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// POST /internal/stores/upsert rewrites the store→tenant projection that
// StoreMiddleware reads to decide tenant isolation. Unauthenticated, it
// lets a caller reassign any store to any tenant. Only a NetworkPolicy
// stood behind it.
func TestStoresUpsert_RequiresInternalSecret(t *testing.T) {
	require.Equal(t, http.StatusUnauthorized, postUpsert(t, "the-secret", "").Code,
		"no X-Internal-Auth must be rejected")

	require.Equal(t, http.StatusUnauthorized, postUpsert(t, "the-secret", "wrong").Code,
		"a wrong X-Internal-Auth must be rejected")
}

// NOT asserted here: that an empty secret closes the route. It does not —
// auth.InternalSecretAuth skips the comparison entirely when the secret is
// "", so an unset MARKETPLACE_INTERNAL_AUTH_SECRET opens every route that
// uses it. That fail-open is real and tracked separately as its own
// go-live item; enshrining it in a test here would be the wrong fix and
// asserting the opposite would be a lie about today's behaviour.
