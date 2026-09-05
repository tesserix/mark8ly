package storefront

// Adversarial coverage for the store-membership gate.
//
// The bug this guards against is that membership creation used to be a
// SIDE EFFECT nobody had to ask for: OptionalCustomerAuth called the
// creating upsert on every authenticated request, so a customer of store1
// who so much as loaded a page at store2 became a customer of store2.
//
// So the tests below do not assert "no row afterwards" — a fixture can
// pin a shape the real path never produces. They make the create path
// itself a TRIPWIRE: the fake service fails the test from INSIDE
// JoinStore. Delete the gate, wire the session path back to the creating
// call, and these tests fail on the call, not on an inference about it.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

const testSessionSecret = "membership-test-cookie-secret"

var (
	testStoreID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testTenantID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func membershipTestStore() *stores.Store {
	return &stores.Store{
		ID:       testStoreID.String(),
		TenantID: testTenantID.String(),
		Slug:     "store-two",
		Status:   stores.StatusActive,
	}
}

// tripwireService is a CustomerProfileService whose write path fails the
// test. `member` is the membership LookupProfile reports, or nil for
// "this identity has not joined this store".
type tripwireService struct {
	t         *testing.T
	member    *customer.CustomerProfile
	allowJoin bool
	joinErr   error
	joins     int
}

func (f *tripwireService) LookupProfile(_ context.Context, _ uuid.UUID, _ string) (*customer.CustomerProfile, error) {
	if f.member == nil {
		return nil, customer.ErrNotFound
	}
	return f.member, nil
}

func (f *tripwireService) JoinStore(_ context.Context, in customer.JoinStoreInput, _ *gin.Context) (*customer.CustomerProfile, error) {
	f.joins++
	if !f.allowJoin {
		f.t.Errorf(
			"membership was created on a path that must never create one: JoinStore called for %q at store %s",
			in.Email, in.StoreID,
		)
		return nil, customer.ErrNotFound
	}
	if f.joinErr != nil {
		return nil, f.joinErr
	}
	return &customer.CustomerProfile{
		ID:      uuid.New(),
		StoreID: in.StoreID,
		Email:   in.Email,
		Status:  customer.StatusActive,
	}, nil
}

// signedCookie builds a valid mp_customer_session value scoped to store.
func signedCookie(t *testing.T, store *stores.Store, email string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"uid":        "zitadel-uid-1",
		"email":      email,
		"store_slug": store.Slug,
		"store_id":   store.ID,
		"tenant_id":  store.TenantID,
		"exp":        time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(testSessionSecret))
	mac.Write([]byte(b64))
	return b64 + "." + hex.EncodeToString(mac.Sum(nil))
}

func membershipTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// sessionRouter wires the real session chain: store context, then the
// read-only customer auth middleware.
func sessionRouter(t *testing.T, svc CustomerProfileService, tail ...gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := membershipTestStore()
	r.Use(func(c *gin.Context) { c.Set("store", store); c.Next() })
	r.Use(OptionalCustomerAuth(testSessionSecret, svc, membershipTestLogger()))
	handlers := append(tail, func(c *gin.Context) {
		_, hasProfile := c.Get(CustomerProfileKey)
		c.JSON(http.StatusOK, gin.H{
			"member":   hasProfile,
			"identity": c.GetString(CustomerIdentityEmailKey),
		})
	})
	r.GET("/s/:storeSlug/anything", handlers...)
	return r
}

func doAuthedGet(t *testing.T, r *gin.Engine, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/s/store-two/anything", nil)
	req.AddCookie(&http.Cookie{Name: "mp_customer_session", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestSessionPathNeverCreatesMembership is THE regression test for
// docs/superpowers/specs/2026-09-05-customer-store-membership-design.md.
// An authenticated customer with no membership at this store makes an
// ordinary authenticated request; the fake fails the test from inside
// JoinStore if the request creates one.
func TestSessionPathNeverCreatesMembership(t *testing.T) {
	svc := &tripwireService{t: t} // member == nil: joined store1, not store2
	r := sessionRouter(t, svc)

	w := doAuthedGet(t, r, signedCookie(t, membershipTestStore(), "shopper@example.com"))

	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body: %s", w.Code, w.Body.String())
	}
	var body struct {
		Member   bool   `json:"member"`
		Identity string `json:"identity"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Member {
		t.Fatal("a non-member resolved to a member — the session path minted a membership")
	}
	if body.Identity != "shopper@example.com" {
		t.Fatalf("identity should still be carried for the join offer, got %q", body.Identity)
	}
	if svc.joins != 0 {
		t.Fatalf("JoinStore called %d times from the session path", svc.joins)
	}
}

func TestSessionPathResolvesAnExistingMembership(t *testing.T) {
	svc := &tripwireService{t: t, member: &customer.CustomerProfile{
		ID:      uuid.New(),
		StoreID: testStoreID,
		Email:   "member@example.com",
		Status:  customer.StatusActive,
	}}
	r := sessionRouter(t, svc)

	w := doAuthedGet(t, r, signedCookie(t, membershipTestStore(), "member@example.com"))

	var body struct {
		Member bool `json:"member"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if !body.Member {
		t.Fatal("an existing member was not resolved — the gate must not lock out real customers")
	}
	if svc.joins != 0 {
		t.Fatal("an existing member must be looked up, not re-created")
	}
}

// The misleading-error failure mode: a customer with a perfectly good
// password must never be told their credentials are wrong.
func TestRequireCustomerAuthOffersTheJoinInsteadOfBlamingThePassword(t *testing.T) {
	svc := &tripwireService{t: t}
	r := sessionRouter(t, svc, RequireCustomerAuth())

	w := doAuthedGet(t, r, signedCookie(t, membershipTestStore(), "shopper@example.com"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for an authenticated non-member, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Error        string `json:"error"`
		Message      string `json:"message"`
		JoinRequired bool   `json:"join_required"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "membership_required" || !body.JoinRequired {
		t.Fatalf("response does not identify a joinable state: %s", w.Body.String())
	}
	for _, forbidden := range []string{"password", "incorrect", "Sign in again"} {
		if containsFold(body.Message, forbidden) {
			t.Fatalf("copy implies a credential problem (%q): %q", forbidden, body.Message)
		}
	}
	if !containsFold(body.Message, "join") {
		t.Fatalf("copy is not actionable — it never offers the join: %q", body.Message)
	}
}

func TestRequireCustomerAuthStill401sAGuest(t *testing.T) {
	svc := &tripwireService{t: t}
	r := sessionRouter(t, svc, RequireCustomerAuth())

	req := httptest.NewRequest(http.MethodGet, "/s/store-two/anything", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for a request with no credential, got %d", w.Code)
	}
}

func TestRequireCustomerIdentityAdmitsAnAuthenticatedNonMember(t *testing.T) {
	svc := &tripwireService{t: t}
	r := sessionRouter(t, svc, RequireCustomerIdentity())

	w := doAuthedGet(t, r, signedCookie(t, membershipTestStore(), "shopper@example.com"))
	if w.Code != http.StatusOK {
		t.Fatalf("the join guard rejected the exact customer it exists for: %d %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/s/store-two/anything", nil)
	guest := httptest.NewRecorder()
	r.ServeHTTP(guest, req)
	if guest.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for an unauthenticated join attempt, got %d", guest.Code)
	}
}

// joinRouter wires the real join endpoint behind the real guards.
func joinRouter(t *testing.T, svc *tripwireService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := membershipTestStore()
	h := NewCustomerAccountHandler(nil, nil, svc, membershipTestLogger())
	r.Use(func(c *gin.Context) { c.Set("store", store); c.Next() })
	r.Use(OptionalCustomerAuth(testSessionSecret, svc, membershipTestLogger()))
	r.POST("/s/:storeSlug/account/join", RequireCustomerIdentity(), h.Join)
	return r
}

func postJoin(t *testing.T, r *gin.Engine, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/s/store-two/account/join", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "mp_customer_session", Value: cookie})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestJoinCreatesTheMembershipForTheVerifiedIdentity(t *testing.T) {
	svc := &tripwireService{t: t, allowJoin: true}
	r := joinRouter(t, svc)

	w := postJoin(t, r, signedCookie(t, membershipTestStore(), "shopper@example.com"))
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body: %s", w.Code, w.Body.String())
	}
	if svc.joins != 1 {
		t.Fatalf("expected exactly one join, got %d", svc.joins)
	}
}

func TestJoinRejectsAnUnauthenticatedCaller(t *testing.T) {
	svc := &tripwireService{t: t} // JoinStore trips the test if reached
	r := joinRouter(t, svc)

	w := postJoin(t, r, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestJoinSurfacesABlockedMembershipAsBlocked(t *testing.T) {
	svc := &tripwireService{t: t, allowJoin: true, joinErr: customer.ErrBlocked}
	r := joinRouter(t, svc)

	w := postJoin(t, r, signedCookie(t, membershipTestStore(), "blocked@example.com"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for a blocked customer, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Error != "account_blocked" {
		t.Fatalf("want account_blocked, got %s", w.Body.String())
	}
	if containsFold(body.Message, "try again") {
		t.Fatalf("blocked copy must not read as retryable: %q", body.Message)
	}
}

// TestMobileSessionPathNeverCreatesMembership is the same tripwire for
// the bearer path. A gate applied only to the web storefront would leave
// the mobile apps under apps/ minting memberships silently.
func TestMobileSessionPathNeverCreatesMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &tripwireService{t: t}
	store := membershipTestStore()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("store", store)
		// What a verified Bearer token leaves behind.
		c.Set(CustomerGipUIDKey, "gip-uid-1")
		c.Set(CustomerEmailKey, "shopper@example.com")
		c.Next()
	})
	r.GET("/m/:storeSlug/anything",
		mobileCustomerProfileMW(svc, membershipTestLogger()),
		func(c *gin.Context) {
			_, hasProfile := c.Get(CustomerProfileKey)
			c.JSON(http.StatusOK, gin.H{
				"member":   hasProfile,
				"identity": c.GetString(CustomerIdentityEmailKey),
			})
		})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/m/store-two/anything", nil))

	var body struct {
		Member   bool   `json:"member"`
		Identity string `json:"identity"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Member {
		t.Fatal("the mobile bearer path resolved a non-member as a member")
	}
	if body.Identity != "shopper@example.com" {
		t.Fatalf("mobile identity not carried through for the join: %q", body.Identity)
	}
	if svc.joins != 0 {
		t.Fatalf("JoinStore called %d times from the mobile session path", svc.joins)
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
