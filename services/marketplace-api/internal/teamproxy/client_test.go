package teamproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type captured struct {
	method string
	path   string
	auth   string
	body   map[string]string
}

func newTestServer(t *testing.T, status int, respBody string, cap *captured) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.auth = r.Header.Get("X-Internal-Auth")
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			if len(b) > 0 {
				_ = json.Unmarshal(b, &cap.body)
			}
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
}

func TestListMembers(t *testing.T) {
	var cap captured
	srv := newTestServer(t, 200, `{"data":[{"email":"a@b.com","role":"owner","kind":"owner"}]}`, &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "sekret", srv.Client())
	members, err := c.ListMembers(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if cap.method != http.MethodGet || cap.path != "/internal/tenants/t1/members" {
		t.Fatalf("wrong request: %s %s", cap.method, cap.path)
	}
	if cap.auth != "sekret" {
		t.Fatalf("missing internal auth header, got %q", cap.auth)
	}
	if len(members) != 1 || members[0].Email != "a@b.com" || members[0].Kind != "owner" {
		t.Fatalf("decode failed: %+v", members)
	}
}

func TestCreateInvitationInjectsActorUID(t *testing.T) {
	var cap captured
	srv := newTestServer(t, 201, `{"data":{"id":"inv1","email":"new@b.com","role":"staff","status":"pending","expires_at":"","created_at":""}}`, &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "sekret", srv.Client())
	inv, err := c.CreateInvitation(context.Background(), "t1", "new@b.com", "staff", "uid-42")
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	if cap.method != http.MethodPost || cap.path != "/internal/tenants/t1/invitations" {
		t.Fatalf("wrong request: %s %s", cap.method, cap.path)
	}
	// The actor's GIP UID must be injected into the body — platform-api
	// authorises the invite against it.
	if cap.body["uid"] != "uid-42" || cap.body["email"] != "new@b.com" || cap.body["role"] != "staff" {
		t.Fatalf("body missing fields: %+v", cap.body)
	}
	if inv.ID != "inv1" {
		t.Fatalf("decode failed: %+v", inv)
	}
}

func TestUpdateMemberRoleBody(t *testing.T) {
	var cap captured
	srv := newTestServer(t, 200, `{"data":{"email":"x@b.com","role":"admin"}}`, &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "s", srv.Client())
	res, err := c.UpdateMemberRole(context.Background(), "t1", "x@b.com", "admin", "uid-1")
	if err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	if cap.method != http.MethodPatch || cap.path != "/internal/tenants/t1/members/role" {
		t.Fatalf("wrong request: %s %s", cap.method, cap.path)
	}
	if cap.body["new_role"] != "admin" || cap.body["email"] != "x@b.com" || cap.body["uid"] != "uid-1" {
		t.Fatalf("body wrong: %+v", cap.body)
	}
	if res.Role != "admin" {
		t.Fatalf("decode failed: %+v", res)
	}
}

func TestRevokeInvitation(t *testing.T) {
	var cap captured
	srv := newTestServer(t, 200, `{"data":{"revoked":true}}`, &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "s", srv.Client())
	if err := c.RevokeInvitation(context.Background(), "t1", "inv9", "uid-1"); err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	if cap.method != http.MethodDelete || cap.path != "/internal/tenants/t1/invitations/inv9" {
		t.Fatalf("wrong request: %s %s", cap.method, cap.path)
	}
	if cap.body["uid"] != "uid-1" {
		t.Fatalf("missing uid: %+v", cap.body)
	}
}

func TestDeleteTenantAccount_SendsAuthAndBody(t *testing.T) {
	var cap captured
	srv := newTestServer(t, http.StatusNoContent, "", &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "secret", srv.Client())
	if err := c.DeleteTenantAccount(context.Background(), "t1", "u1"); err != nil {
		t.Fatalf("DeleteTenantAccount: %v", err)
	}
	if cap.method != http.MethodDelete || cap.path != "/internal/tenants/t1/account" {
		t.Fatalf("wrong request: %s %s", cap.method, cap.path)
	}
	if cap.auth != "secret" {
		t.Fatalf("missing internal auth header, got %q", cap.auth)
	}
	if cap.body["uid"] != "u1" {
		t.Fatalf("missing uid: %+v", cap.body)
	}
}

func TestDeleteTenantAccount_Forbidden(t *testing.T) {
	var cap captured
	srv := newTestServer(t, 403, `{"error":"forbidden","message":"only the owner can delete the tenant"}`, &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "secret", srv.Client())
	err := c.DeleteTenantAccount(context.Background(), "t1", "u1")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 403 || apiErr.Code != "forbidden" {
		t.Fatalf("wrong APIError: %+v", apiErr)
	}
}

func TestAPIErrorPropagates(t *testing.T) {
	var cap captured
	srv := newTestServer(t, 403, `{"error":"forbidden","message":"an admin cannot change another admin"}`, &cap)
	defer srv.Close()

	c := NewClient(srv.URL, "s", srv.Client())
	_, err := c.UpdateMemberRole(context.Background(), "t1", "x@b.com", "admin", "uid-1")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 403 || apiErr.Code != "forbidden" {
		t.Fatalf("wrong APIError: %+v", apiErr)
	}
}
