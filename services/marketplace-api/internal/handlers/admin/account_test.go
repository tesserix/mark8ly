package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

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
