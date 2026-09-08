package ssousers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/sso"
	"github.com/mark8ly/marketplace-api/internal/ssousers"
)

// The client must satisfy the interface JIT provisioning consumes. Asserted
// here because that interface had NO implementation at all until #820, which
// is why the SSO login route could not be mounted.
var _ sso.UsersRepo = (*ssousers.Client)(nil)

const testSecret = "internal-auth-for-tests"

type capture struct {
	path   string
	auth   string
	body   map[string]any
	status int
	reply  string
}

func serve(t *testing.T, c *capture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.path = r.URL.Path
		c.auth = r.Header.Get("X-Internal-Auth")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &c.body)

		status := c.status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(c.reply))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFindByTenantAndEmail_ReturnsTheMemberIdentity(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	c := &capture{reply: `{"data":{"user_id":"` + userID.String() + `","found":true}}`}
	srv := serve(t, c)

	got, found, err := ssousers.NewClient(srv.URL, testSecret, nil).
		FindByTenantAndEmail(context.Background(), tenantID, "someone@example.com")

	if err != nil {
		t.Fatalf("FindByTenantAndEmail: %v", err)
	}
	if !found || got != userID {
		t.Fatalf("got %s found=%v, want %s true", got, found, userID)
	}
	if c.path != "/internal/tenants/"+tenantID.String()+"/sso-users/lookup" {
		t.Errorf("path = %q", c.path)
	}
	if c.auth != testSecret {
		t.Errorf("internal auth header = %q — the route refuses without it", c.auth)
	}
	if c.body["email"] != "someone@example.com" {
		t.Errorf("body = %v", c.body)
	}
}

func TestFindByTenantAndEmail_NotFoundIsNotAnError(t *testing.T) {
	c := &capture{reply: `{"data":{"user_id":"","found":false}}`}
	srv := serve(t, c)

	got, found, err := ssousers.NewClient(srv.URL, testSecret, nil).
		FindByTenantAndEmail(context.Background(), uuid.New(), "nobody@example.com")

	if err != nil {
		t.Fatalf("FindByTenantAndEmail: %v", err)
	}
	if found || got != uuid.Nil {
		t.Fatalf("got %s found=%v, want the zero identity and false", got, found)
	}
}

// A failed lookup must NOT read as "no such user": JIT's next step on false is
// to provision, so an outage would create a duplicate account per login.
func TestFindByTenantAndEmail_AFailureIsNotANegativeAnswer(t *testing.T) {
	c := &capture{status: http.StatusInternalServerError, reply: `{"error":"boom"}`}
	srv := serve(t, c)

	_, found, err := ssousers.NewClient(srv.URL, testSecret, nil).
		FindByTenantAndEmail(context.Background(), uuid.New(), "someone@example.com")

	if err == nil {
		t.Fatal("a 500 was reported as a successful lookup")
	}
	if found {
		t.Fatal("a failure reported found=true")
	}
}

func TestCreateForTenant_SendsTheAssertedNameAndTheDefaultRole(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	c := &capture{reply: `{"data":{"user_id":"` + userID.String() + `"}}`}
	srv := serve(t, c)

	got, err := ssousers.NewClient(srv.URL, testSecret, nil).CreateForTenant(
		context.Background(), tenantID, "new.person@example.com",
		map[string]any{"firstName": "New", "lastName": "Person"}, "ignored")

	if err != nil {
		t.Fatalf("CreateForTenant: %v", err)
	}
	if got != userID {
		t.Errorf("got %s, want %s", got, userID)
	}
	if c.path != "/internal/tenants/"+tenantID.String()+"/sso-users" {
		t.Errorf("path = %q", c.path)
	}
	if c.body["first_name"] != "New" || c.body["last_name"] != "Person" {
		t.Errorf("names = %v", c.body)
	}
	// The role JIT passes is deliberately ignored: an IdP assertion says who
	// someone is, not how much to trust them.
	if c.body["role"] != ssousers.DefaultRole {
		t.Errorf("role = %v, want %q", c.body["role"], ssousers.DefaultRole)
	}
}

// Zitadel's own attribute names differ per IdP. Missing names are fine —
// platform-api derives them from the email — but a name that WAS asserted must
// not be dropped.
func TestCreateForTenant_ReadsTheCommonAttributeSpellings(t *testing.T) {
	for _, attrs := range []map[string]any{
		{"given_name": "Jo", "family_name": "Lee"},
		{"givenName": "Jo", "familyName": "Lee"},
		{"firstName": "Jo", "surname": "Lee"},
	} {
		c := &capture{reply: `{"data":{"user_id":"` + uuid.New().String() + `"}}`}
		srv := serve(t, c)

		if _, err := ssousers.NewClient(srv.URL, testSecret, nil).CreateForTenant(
			context.Background(), uuid.New(), "jo@example.com", attrs, ""); err != nil {
			t.Fatalf("CreateForTenant: %v", err)
		}
		if c.body["first_name"] != "Jo" || c.body["last_name"] != "Lee" {
			t.Errorf("attrs %v produced names %v", attrs, c.body)
		}
	}
}

func TestCreateForTenant_MissingNamesAreLeftToPlatformAPI(t *testing.T) {
	c := &capture{reply: `{"data":{"user_id":"` + uuid.New().String() + `"}}`}
	srv := serve(t, c)

	if _, err := ssousers.NewClient(srv.URL, testSecret, nil).CreateForTenant(
		context.Background(), uuid.New(), "jo@example.com", nil, ""); err != nil {
		t.Fatalf("CreateForTenant: %v", err)
	}
	if c.body["first_name"] != nil && c.body["first_name"] != "" {
		t.Errorf("first_name = %v, want empty so platform-api derives it", c.body["first_name"])
	}
}

// uuid.Nil would read as a valid identity and bind the SSO login to the zero
// user, so a non-UUID id has to be an error rather than a fallback.
func TestParseFailure_IsAnErrorNotTheZeroIdentity(t *testing.T) {
	c := &capture{reply: `{"data":{"user_id":"not-a-uuid","found":true}}`}
	srv := serve(t, c)

	_, found, err := ssousers.NewClient(srv.URL, testSecret, nil).
		FindByTenantAndEmail(context.Background(), uuid.New(), "someone@example.com")

	if err == nil {
		t.Fatal("a non-UUID user id was accepted")
	}
	if found {
		t.Fatal("a malformed id reported found=true")
	}
}
