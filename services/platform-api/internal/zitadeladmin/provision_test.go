package zitadeladmin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// The handlers below are TRIPWIRES: an unexpected path, method or body
// shape fails the test from inside the handler rather than being
// discovered (or not) by an assertion afterwards. This package has been
// bitten before by a fixture that asserted a request shape the real
// Zitadel never accepts — see the wrapped-oneof incident in the package
// doc — so the fake must reject anything the live API would.

func zitadelError(id, message string, grpcCode int) []byte {
	raw, _ := json.Marshal(map[string]any{
		"code":    grpcCode,
		"message": message,
		"details": []any{map[string]any{"id": id}},
	})
	return raw
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body %q: %v", raw, err)
	}
	return body
}

func newUser() HumanUser {
	return HumanUser{
		Email:     "teammate@example.com",
		FirstName: "Tea",
		LastName:  "Mmate",
		Password:  "correct-horse-battery-staple",
	}
}

// TestEnsureHumanUser_SendsTheVerifiedShape pins the exact body verified
// working in production, including the two protojson traps: no oneof
// wrapper, and isVerified sent explicitly as true (protojson elides zero
// values, so an omitted or false isVerified creates a user that
// resolveUserIDByEmail can then never find again).
func TestEnsureHumanUser_SendsTheVerifiedShape(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/users/human" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s, want POST /v2/users/human", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		body := decodeBody(t, r)

		org, _ := body["organization"].(map[string]any)
		if org == nil || org["orgId"] != "org-tesserix" {
			t.Errorf("organization = %v, want {orgId: org-tesserix}", body["organization"])
		}
		email, _ := body["email"].(map[string]any)
		if email == nil || email["email"] != "teammate@example.com" {
			t.Errorf("email = %v", body["email"])
		}
		if email["isVerified"] != true {
			t.Errorf("email.isVerified = %v, want true sent EXPLICITLY — protojson elides zero values, "+
				"so an absent isVerified reads as false and the user becomes unresolvable", email["isVerified"])
		}
		profile, _ := body["profile"].(map[string]any)
		if profile == nil || profile["givenName"] != "Tea" || profile["familyName"] != "Mmate" {
			t.Errorf("profile = %v", body["profile"])
		}
		pw, _ := body["password"].(map[string]any)
		if pw == nil || pw["password"] != "correct-horse-battery-staple" {
			t.Errorf("password = %v", body["password"])
		}
		if pw["changeRequired"] != false {
			t.Errorf("password.changeRequired = %v, want false so the invitee can sign in with what they just typed", pw["changeRequired"])
		}
		// A wrapper key named after a oneof is the exact bug the package
		// doc records; nothing here has one, and nothing may grow one.
		for _, forbidden := range []string{"medium", "type", "user"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("body carries a %q wrapper key — Zitadel v2 protojson FLATTENS oneofs", forbidden)
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"userId":"zid-1","details":{}}`))
	})

	id, err := c.EnsureHumanUser(context.Background(), newUser())
	if err != nil {
		t.Fatalf("EnsureHumanUser: %v", err)
	}
	if id != "zid-1" {
		t.Errorf("id = %q, want zid-1", id)
	}
}

// TestEnsureHumanUser_AlreadyExistsResolves pins that the live duplicate
// error id resolves to the existing account instead of failing the
// accept. classifyError maps that 409 to ErrUnavailable, so a
// status/sentinel-based check here would read as an outage.
func TestEnsureHumanUser_AlreadyExistsResolves(t *testing.T) {
	var searched bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/users/human":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write(zitadelError(zitadelErrIDUserAlreadyExists, "User already exists", 6))
		case "/v2/users":
			searched = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchResponse(humanEntry("zid-existing", "teammate@example.com", true)))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	})

	id, err := c.EnsureHumanUser(context.Background(), newUser())
	if err != nil {
		t.Fatalf("EnsureHumanUser: %v", err)
	}
	if id != "zid-existing" {
		t.Errorf("id = %q, want zid-existing", id)
	}
	if !searched {
		t.Error("the already-exists branch must resolve the existing user by email")
	}
}

// TestEnsureHumanUser_OtherErrorDoesNotResolve pins that only the
// duplicate id takes the resolve branch. A different 409 (or any other
// failure) must propagate — silently resolving one would let an
// unrelated failure return some other org member's id.
func TestEnsureHumanUser_OtherErrorDoesNotResolve(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/users/human" {
			t.Errorf("unexpected request %s %s — a non-duplicate failure must NOT fall through to a search", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(zitadelError("DOMAIN-HuJf6", "Password is too short", 3))
	})

	if _, err := c.EnsureHumanUser(context.Background(), newUser()); !errors.Is(err, gipadmin.ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}

func TestEnsureHumanUser_RefusesIncompleteInput(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("incomplete input must never reach the network (%s %s)", r.Method, r.URL.Path)
	})
	for name, in := range map[string]HumanUser{
		"no email":    {FirstName: "A", LastName: "B", Password: "pw"},
		"no name":     {Email: "a@b.com", Password: "pw"},
		"no password": {Email: "a@b.com", FirstName: "A", LastName: "B"},
	} {
		if _, err := c.EnsureHumanUser(context.Background(), in); err == nil {
			t.Errorf("%s: EnsureHumanUser = nil error, want a refusal", name)
		}
	}
}

// TestEnsureProjectGrant_SendsVerifiedShape pins the management-API body
// and the org header. Without this grant the mark8ly-admin project's
// projectRoleCheck refuses to finalize the OIDC flow (403 OIDC-foSyH49RvL).
func TestEnsureProjectGrant_SendsVerifiedShape(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/management/v1/users/zid-1/grants" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		if got := r.Header.Get("x-zitadel-orgid"); got != "org-tesserix" {
			t.Errorf("x-zitadel-orgid = %q, want org-tesserix", got)
		}
		body := decodeBody(t, r)
		if body["projectId"] != "proj-1" {
			t.Errorf("projectId = %v, want proj-1", body["projectId"])
		}
		roles, _ := body["roleKeys"].([]any)
		if len(roles) != 1 || roles[0] != "mark8ly.staff" {
			t.Errorf("roleKeys = %v, want [mark8ly.staff]", body["roleKeys"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"userGrantId":"g-1"}`))
	})

	if err := c.EnsureProjectGrant(context.Background(), "zid-1", "proj-1", []string{"mark8ly.staff"}); err != nil {
		t.Fatalf("EnsureProjectGrant: %v", err)
	}
}

// TestEnsureProjectGrant_AlreadyExistsIsSuccess pins idempotency: a
// re-accept must not fail because the grant survived the first attempt.
func TestEnsureProjectGrant_AlreadyExistsIsSuccess(t *testing.T) {
	for name, respond := range map[string]func(w http.ResponseWriter){
		"409 + grpc AlreadyExists": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write(zitadelError("COMMAND-XYZ", "User grant already exists", 6))
		},
		"message only": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(zitadelError("COMMAND-XYZ", "Errors.UserGrant.AlreadyExists", 3))
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { respond(w) })
			if err := c.EnsureProjectGrant(context.Background(), "zid-1", "proj-1", []string{"mark8ly.staff"}); err != nil {
				t.Fatalf("EnsureProjectGrant = %v, want nil (idempotent)", err)
			}
		})
	}
}

// TestEnsureProjectGrant_RealFailurePropagates pins that idempotency
// tolerance did not swallow genuine failures — a grant that never lands
// is an account that can never sign in.
func TestEnsureProjectGrant_RealFailurePropagates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(zitadelError("AUTHZ-1", "No permission", 7))
	})
	if err := c.EnsureProjectGrant(context.Background(), "zid-1", "proj-1", []string{"mark8ly.staff"}); err == nil {
		t.Fatal("EnsureProjectGrant = nil, want the failure to propagate")
	}
}

func newTestProvisioner(t *testing.T, handler http.HandlerFunc) *StaffProvisioner {
	t.Helper()
	p, err := NewStaffProvisioner(newTestClient(t, handler), "proj-1", []string{"mark8ly.staff"})
	if err != nil {
		t.Fatalf("NewStaffProvisioner: %v", err)
	}
	return p
}

// TestProvisionStaff_NewUser pins the create path and, critically, that
// the grant is ensured too — user-without-grant is one of the two
// half-provisioned states this whole change exists to make impossible.
func TestProvisionStaff_NewUser(t *testing.T) {
	var created, granted bool
	p := newTestProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/users":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchResponse()) // no match
		case "/v2/users/human":
			created = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"userId":"zid-new"}`))
		case "/management/v1/users/zid-new/grants":
			granted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"userGrantId":"g-1"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	})

	id, err := p.ProvisionStaff(context.Background(), "teammate@example.com", "Tea", "Mmate", "pw-123456789")
	if err != nil {
		t.Fatalf("ProvisionStaff: %v", err)
	}
	if id != "zid-new" || !created || !granted {
		t.Errorf("id=%q created=%v granted=%v, want zid-new/true/true", id, created, granted)
	}
}

// TestProvisionStaff_ExistingUserNeedsNoPassword pins resolve-first: an
// address that already has an account must not attempt a create (the
// handler fails the test if it does), and must still get the grant —
// an account made by another path holds no admin-project role.
func TestProvisionStaff_ExistingUserNeedsNoPassword(t *testing.T) {
	var granted bool
	p := newTestProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/users":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchResponse(humanEntry("zid-existing", "teammate@example.com", true)))
		case "/management/v1/users/zid-existing/grants":
			granted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"userGrantId":"g-1"}`))
		default:
			t.Errorf("unexpected request %s %s — an existing user must not be re-created", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	})

	id, err := p.ProvisionStaff(context.Background(), "teammate@example.com", "Tea", "Mmate", "")
	if err != nil {
		t.Fatalf("ProvisionStaff: %v", err)
	}
	if id != "zid-existing" || !granted {
		t.Errorf("id=%q granted=%v, want zid-existing/true", id, granted)
	}
}

// TestProvisionStaff_GrantFailureFailsTheWholeCall pins that a created
// user with no grant is reported as a failure, never as success.
func TestProvisionStaff_GrantFailureFailsTheWholeCall(t *testing.T) {
	p := newTestProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/users":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchResponse())
		case "/v2/users/human":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"userId":"zid-new"}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(zitadelError("X-1", "boom", 13))
		}
	})

	if _, err := p.ProvisionStaff(context.Background(), "teammate@example.com", "Tea", "Mmate", "pw-123456789"); err == nil {
		t.Fatal("ProvisionStaff = nil error, want a failure when the project grant does not land")
	}
}

// TestProvisionStaff_AmbiguousEmailRefuses pins that two accounts sharing
// an address is a refusal, not a coin flip over who gets admin access.
func TestProvisionStaff_AmbiguousEmailRefuses(t *testing.T) {
	p := newTestProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/users" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse(
			humanEntry("zid-a", "teammate@example.com", true),
			humanEntry("zid-b", "teammate@example.com", true),
		))
	})

	if _, err := p.ProvisionStaff(context.Background(), "teammate@example.com", "Tea", "Mmate", "pw"); !errors.Is(err, ErrAmbiguousEmail) {
		t.Fatalf("err = %v, want ErrAmbiguousEmail", err)
	}
}

func TestNewStaffProvisioner_RefusesEmptyConfig(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := NewStaffProvisioner(c, "", []string{"mark8ly.staff"}); err == nil {
		t.Error("NewStaffProvisioner with no project id = nil error, want a refusal")
	}
	if _, err := NewStaffProvisioner(c, "proj-1", nil); err == nil {
		t.Error("NewStaffProvisioner with no roles = nil error, want a refusal")
	}
}
