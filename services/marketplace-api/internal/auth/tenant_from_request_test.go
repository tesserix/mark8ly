package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeMembershipChecker is a minimal test double for TenantMembershipChecker
// that doesn't require pulling in the real internal/authz package.
type fakeMembershipChecker struct {
	members map[string]map[string]bool // userID -> tenantID -> isMember
	err     error
}

func (f *fakeMembershipChecker) CheckMembership(_ context.Context, userID, tenantID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.members[userID][tenantID], nil
}

func TestTenantFromRequest_MemberIsAccepted(t *testing.T) {
	checker := &fakeMembershipChecker{
		members: map[string]map[string]bool{"user-1": {"tenant-a": true}},
	}
	gin.SetMode(gin.TestMode)

	var gotTenant, gotUser string
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "user-1"); c.Next() })
	r.Use(TenantFromRequest(checker, nil))
	r.GET("/probe", func(c *gin.Context) {
		gotTenant = c.GetString("tenant_id")
		gotUser = c.GetString("user_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(ActingTenantHeader, "tenant-a")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotTenant != "tenant-a" {
		t.Errorf("tenant_id = %q, want %q", gotTenant, "tenant-a")
	}
	if gotUser != "user-1" {
		t.Errorf("user_id = %q, want unchanged %q", gotUser, "user-1")
	}
}

func TestTenantFromRequest_NonMemberLeavesTenantEmpty(t *testing.T) {
	checker := &fakeMembershipChecker{
		members: map[string]map[string]bool{"user-1": {"tenant-a": true}},
	}
	gin.SetMode(gin.TestMode)

	var gotTenant, gotUser string
	reached := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "user-1"); c.Next() })
	r.Use(TenantFromRequest(checker, nil))
	r.GET("/probe", func(c *gin.Context) {
		reached = true
		gotTenant = c.GetString("tenant_id")
		gotUser = c.GetString("user_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(ActingTenantHeader, "tenant-b") // user-1 is not a member of tenant-b
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !reached {
		t.Fatal("middleware aborted the chain — it must always call c.Next()")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (this middleware never aborts)", w.Code)
	}
	if gotTenant != "" {
		t.Errorf("tenant_id = %q, want empty for a non-member", gotTenant)
	}
	if gotUser != "user-1" {
		t.Errorf("user_id = %q, want unchanged %q", gotUser, "user-1")
	}
}

func TestTenantFromRequest_AbsentHeaderLeavesTenantEmpty(t *testing.T) {
	checker := &fakeMembershipChecker{
		members: map[string]map[string]bool{"user-1": {"tenant-a": true}},
	}
	gin.SetMode(gin.TestMode)

	var gotTenant, gotUser string
	reached := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "user-1"); c.Next() })
	r.Use(TenantFromRequest(checker, nil))
	r.GET("/probe", func(c *gin.Context) {
		reached = true
		gotTenant = c.GetString("tenant_id")
		gotUser = c.GetString("user_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil) // no ActingTenantHeader set
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !reached {
		t.Fatal("middleware aborted the chain — it must always call c.Next()")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotTenant != "" {
		t.Errorf("tenant_id = %q, want empty when header absent", gotTenant)
	}
	if gotUser != "user-1" {
		t.Errorf("user_id = %q, want unchanged %q", gotUser, "user-1")
	}
}

func TestTenantFromRequest_FGAErrorFailsClosed(t *testing.T) {
	checker := &fakeMembershipChecker{err: errors.New("fga: connection refused")}
	gin.SetMode(gin.TestMode)

	var gotTenant, gotUser string
	reached := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "user-1"); c.Next() })
	r.Use(TenantFromRequest(checker, nil))
	r.GET("/probe", func(c *gin.Context) {
		reached = true
		gotTenant = c.GetString("tenant_id")
		gotUser = c.GetString("user_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(ActingTenantHeader, "tenant-a")
	w := httptest.NewRecorder()

	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("middleware panicked on FGA error: %v", p)
		}
	}()
	r.ServeHTTP(w, req)

	if !reached {
		t.Fatal("middleware aborted the chain on FGA error — it must always call c.Next()")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (FGA error must not 500)", w.Code)
	}
	if gotTenant != "" {
		t.Errorf("tenant_id = %q, want empty on FGA error (fail closed)", gotTenant)
	}
	if gotUser != "user-1" {
		t.Errorf("user_id = %q, want unchanged %q", gotUser, "user-1")
	}
}

func TestTenantFromRequest_NoUserIDLeavesTenantEmpty(t *testing.T) {
	// Defense in depth: if bearer auth somehow ran without setting
	// user_id, the membership check has no identity to check against, so
	// the tenant must not be bound either.
	checker := &fakeMembershipChecker{
		members: map[string]map[string]bool{"user-1": {"tenant-a": true}},
	}
	gin.SetMode(gin.TestMode)

	var gotTenant string
	reached := false
	r := gin.New()
	r.Use(TenantFromRequest(checker, nil))
	r.GET("/probe", func(c *gin.Context) {
		reached = true
		gotTenant = c.GetString("tenant_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(ActingTenantHeader, "tenant-a")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !reached {
		t.Fatal("middleware aborted the chain — it must always call c.Next()")
	}
	if gotTenant != "" {
		t.Errorf("tenant_id = %q, want empty with no user_id set", gotTenant)
	}
}

// TestTenantFromRequest_NilCheckerDoesNotPanic is the regression test for
// the "Nil-safe like TenantGate" claim in mobile_routes.go's doc comment,
// which was false before this guard existed: a nil TenantMembershipChecker
// with a present header and user_id reached checker.CheckMembership with
// no nil guard and panicked. It is unreachable from main.go's current
// wiring, but a future caller reading that doc comment could reasonably
// wire a nil checker on purpose (e.g. a GIP-only deployment that still
// unconditionally mounts TenantFromRequest); this must degrade exactly
// like "no header" / "no user_id", not crash the request.
func TestTenantFromRequest_NilCheckerDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotTenant string
	reached := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "user-1"); c.Next() })
	r.Use(TenantFromRequest(nil, nil))
	r.GET("/probe", func(c *gin.Context) {
		reached = true
		gotTenant = c.GetString("tenant_id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(ActingTenantHeader, "tenant-a")
	w := httptest.NewRecorder()

	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("middleware panicked with a nil checker: %v", p)
		}
	}()
	r.ServeHTTP(w, req)

	if !reached {
		t.Fatal("middleware aborted the chain with a nil checker — it must always call c.Next()")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a nil checker must not 500)", w.Code)
	}
	if gotTenant != "" {
		t.Errorf("tenant_id = %q, want empty with a nil checker", gotTenant)
	}
}
