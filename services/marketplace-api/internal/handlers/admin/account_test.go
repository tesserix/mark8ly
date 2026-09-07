package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/displayname"
	"github.com/mark8ly/marketplace-api/internal/userprofile"
)

// withHeaderIdentity injects the context keys HeaderTrustAuth sets on the
// web-admin path, so handler tests don't need the real middleware.
func withHeaderIdentity(userID, tenantID, email string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("tenant_id", tenantID)
		if email != "" {
			c.Set("user_email", email)
		}
		c.Next()
	}
}

// getProfile runs GET /account through the handler and returns the decoded
// profile DTO. authBFFURL is left empty so fetchMFAEnabled short-circuits
// without any network call.
func getProfile(t *testing.T, h *AccountHandler, userID, email string) (int, profileDTO) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(withHeaderIdentity(userID, "t1", email))
	r.GET("/account", h.GetProfile)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/account", nil)
	r.ServeHTTP(rec, req)

	var body struct {
		Data profileDTO `json:"data"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, body.Data
}

// TestGetProfile_SeedsRowOnFirstRead covers the upsert-on-read contract
// the whole admin UI leans on: a user with no profile row gets one, with
// their email mirrored from the session headers.
func TestGetProfile_SeedsRowOnFirstRead(t *testing.T) {
	store := userprofile.NewFakeStore()
	h := NewAccountHandler(store, "", "", nil)

	code, dto := getProfile(t, h, "uid-new", "pat@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200", code)
	}
	if dto.Email != "pat@example.com" {
		t.Errorf("wire email=%q, want pat@example.com", dto.Email)
	}
	row, ok := store.Rows["uid-new"]
	if !ok {
		t.Fatal("profile row was not created")
	}
	if row.Email != "pat@example.com" {
		t.Errorf("persisted email=%q, want pat@example.com", row.Email)
	}
}

// TestGetProfile_ExistingRow_MirrorsDriftedEmail covers the other half of
// loadOrSeed: an existing row whose email no longer matches the session.
func TestGetProfile_ExistingRow_MirrorsDriftedEmail(t *testing.T) {
	store := userprofile.NewFakeStore()
	store.Rows["uid-existing"] = userprofile.Profile{
		UserID:      "uid-existing",
		Email:       "old@example.com",
		DisplayName: "Ali Hand-Typed",
	}
	h := NewAccountHandler(store, "", "", nil)

	code, dto := getProfile(t, h, "uid-existing", "new@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200", code)
	}
	if dto.Email != "new@example.com" {
		t.Errorf("wire email=%q, want the drifted email mirrored", dto.Email)
	}
	if dto.Name != "Ali Hand-Typed" {
		t.Errorf("name=%q, want the stored name preserved", dto.Name)
	}
	if got := store.Rows["uid-existing"].Email; got != "new@example.com" {
		t.Errorf("persisted email=%q, want new@example.com", got)
	}
}

// TestGetProfile_SeedsDisplayNameFromProvider is the guard for the
// first-seed behaviour: the very first read of a profile must pick the
// merchant's name up out of the identity provider's account record.
// Removing the seed from loadOrSeed makes this fail.
func TestGetProfile_SeedsDisplayNameFromProvider(t *testing.T) {
	store := userprofile.NewFakeStore()
	h := NewAccountHandler(store, "", "", nil)
	h.SetDisplayNames(&displayname.FakeLookup{Names: map[string]string{"uid-google": "  Jane Roe  "}})

	code, dto := getProfile(t, h, "uid-google", "jane@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200", code)
	}
	if dto.Name != "Jane Roe" {
		t.Errorf("wire name=%q, want %q (trimmed)", dto.Name, "Jane Roe")
	}
	if got := store.Rows["uid-google"].DisplayName; got != "Jane Roe" {
		t.Errorf("persisted display_name=%q, want %q", got, "Jane Roe")
	}
}

// TestGetProfile_NoProviderName_SeedsBlank covers the population this
// feature does NOT help: every email/password sign-up, whose provider
// record carries no name a person supplied. The profile must still be
// created, with a blank name and a 200 — not an error, and emphatically
// not a name invented from the email.
func TestGetProfile_NoProviderName_SeedsBlank(t *testing.T) {
	store := userprofile.NewFakeStore()
	h := NewAccountHandler(store, "", "", nil)
	// uid absent from Names → ("", nil): account exists, has no name.
	h.SetDisplayNames(&displayname.FakeLookup{Names: map[string]string{}})

	code, dto := getProfile(t, h, "uid-password", "pat@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200", code)
	}
	if dto.Name != "" {
		t.Errorf("wire name=%q, want empty", dto.Name)
	}
	row, ok := store.Rows["uid-password"]
	if !ok {
		t.Fatal("profile row was not created")
	}
	if row.DisplayName != "" {
		t.Errorf("persisted display_name=%q, want empty", row.DisplayName)
	}
	if row.Email != "pat@example.com" {
		t.Errorf("persisted email=%q, want pat@example.com", row.Email)
	}
}

// TestGetProfile_LookupFails_StillCreatesProfile is the hard rule: a
// name lookup failure must never stop a profile from being created, and
// therefore never blocks a sign-in from landing on the admin dashboard.
func TestGetProfile_LookupFails_StillCreatesProfile(t *testing.T) {
	store := userprofile.NewFakeStore()
	h := NewAccountHandler(store, "", "", nil)
	h.SetDisplayNames(&displayname.FakeLookup{Err: errors.New("displayname: auth-bff returned 502")})

	code, dto := getProfile(t, h, "uid-broken", "sam@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200 despite the lookup failing", code)
	}
	if dto.Name != "" {
		t.Errorf("wire name=%q, want empty", dto.Name)
	}
	if _, ok := store.Rows["uid-broken"]; !ok {
		t.Fatal("profile row was not created after a failed name lookup")
	}
}

// TestGetProfile_NoLookupWired_SeedsBlank covers the deployment where no
// lookup is wired at all: unchanged pre-existing behaviour, no panic on
// the nil lookup.
func TestGetProfile_NoLookupWired_SeedsBlank(t *testing.T) {
	store := userprofile.NewFakeStore()
	h := NewAccountHandler(store, "", "", nil) // SetDisplayNames never called

	code, dto := getProfile(t, h, "uid-nolookup", "kim@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200", code)
	}
	if dto.Name != "" {
		t.Errorf("wire name=%q, want empty", dto.Name)
	}
	if _, ok := store.Rows["uid-nolookup"]; !ok {
		t.Fatal("profile row was not created")
	}
}

// TestGetProfile_ExistingRow_SkipsLookup proves the lookup is a
// first-seed cost and not a per-request one, and that it never
// overwrites a name the merchant set by hand. This is the same property
// isAutoUpdate:false protects on the Zitadel side; the two must not
// disagree.
func TestGetProfile_ExistingRow_SkipsLookup(t *testing.T) {
	store := userprofile.NewFakeStore()
	store.Rows["uid-existing"] = userprofile.Profile{
		UserID:      "uid-existing",
		Email:       "ali@example.com",
		DisplayName: "Ali Hand-Typed",
	}
	lookup := &displayname.FakeLookup{Names: map[string]string{"uid-existing": "Ali From The Provider"}}
	h := NewAccountHandler(store, "", "", nil)
	h.SetDisplayNames(lookup)

	code, dto := getProfile(t, h, "uid-existing", "ali@example.com")
	if code != http.StatusOK {
		t.Fatalf("code=%d, want 200", code)
	}
	if dto.Name != "Ali Hand-Typed" {
		t.Errorf("name=%q, want the stored name to win over the provider", dto.Name)
	}
	if lookup.Calls != 0 {
		t.Errorf("name lookup called %d times on an existing row, want 0", lookup.Calls)
	}
}
