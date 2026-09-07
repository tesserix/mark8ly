package storefront

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// signClaims builds a correctly signed mp_customer_session cookie from an
// arbitrary claim map, so a test can omit "uid" — something signedCookie
// cannot express.
func signClaims(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(testSessionSecret))
	mac.Write([]byte(b64))
	return b64 + "." + hex.EncodeToString(mac.Sum(nil))
}

func storeScopedClaims(store *stores.Store, email string) map[string]any {
	return map[string]any{
		"email":      email,
		"store_slug": store.Slug,
		"store_id":   store.ID,
		"tenant_id":  store.TenantID,
		"exp":        time.Now().Add(time.Hour).Unix(),
	}
}

func TestIdentityUIDReturnsZitadelUID(t *testing.T) {
	claims := sessionClaims{UID: "zitadel-uid-1"}
	if got := claims.IdentityUID(); got != "zitadel-uid-1" {
		t.Fatalf("IdentityUID() = %q, want %q", got, "zitadel-uid-1")
	}
}

// A cookie carrying no uid has no identity at all. It must never resolve
// to the empty string, which would compare equal to every other
// identity-less request (#793).
func TestIdentityUIDHasNoGipUIDFallback(t *testing.T) {
	store := membershipTestStore()
	claims := storeScopedClaims(store, "legacy@example.com")
	claims["gip_uid"] = "legacy-gip-uid"

	if _, err := validateSessionCookie(signClaims(t, claims), testSessionSecret); err == nil {
		t.Fatal("expected a gip_uid-only cookie to be rejected as missing identity")
	}
}

// The happy path still works: a Zitadel uid resolves normally, all the
// way through the session middleware.
func TestSessionWithZitadelUIDResolvesIdentity(t *testing.T) {
	store := membershipTestStore()
	claims := storeScopedClaims(store, "shopper@example.com")
	claims["uid"] = "zitadel-uid-1"

	got, err := validateSessionCookie(signClaims(t, claims), testSessionSecret)
	if err != nil {
		t.Fatalf("validateSessionCookie: %v", err)
	}
	if got.IdentityUID() != "zitadel-uid-1" {
		t.Fatalf("IdentityUID() = %q, want %q", got.IdentityUID(), "zitadel-uid-1")
	}

	r := identityRouter(t)
	w := doIdentityGet(t, r, signClaims(t, claims))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != `{"uid":"zitadel-uid-1"}` {
		t.Fatalf("body = %s, want the zitadel uid", body)
	}
}

// A legacy cookie carrying only gip_uid is treated as UNAUTHENTICATED —
// 401, sign in again — rather than as an authenticated request with an
// empty identity.
func TestSessionWithOnlyGipUIDIsUnauthenticated(t *testing.T) {
	store := membershipTestStore()
	claims := storeScopedClaims(store, "legacy@example.com")
	claims["gip_uid"] = "legacy-gip-uid"

	r := identityRouter(t)
	w := doIdentityGet(t, r, signClaims(t, claims))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body %s)", w.Code, w.Body.String())
	}
}

func identityRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := membershipTestStore()
	r.Use(func(c *gin.Context) { c.Set("store", store); c.Next() })
	r.Use(OptionalCustomerAuth(testSessionSecret, &tripwireService{t: t}, membershipTestLogger()))
	r.Use(RequireCustomerIdentity())
	r.GET("/whoami", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"uid": c.GetString(CustomerIdentityUIDKey)})
	})
	return r
}

func doIdentityGet(t *testing.T, r *gin.Engine, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.AddCookie(&http.Cookie{Name: "mp_customer_session", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
