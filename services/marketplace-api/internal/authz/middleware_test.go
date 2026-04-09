package authz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// errClient is a Client test double whose Check always errors.
type errClient struct{}

func (errClient) Check(context.Context, string, string, string) (bool, error) {
	return false, errors.New("boom")
}
func (errClient) CheckMembership(context.Context, string, string) (bool, error) {
	return false, errors.New("boom")
}
func (errClient) GetRole(context.Context, string, string) (Role, error) {
	return "", errors.New("boom")
}

func newRouter(mw *Middleware, required Role, userID, tenantID string) *gin.Engine {
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		if userID != "" {
			c.Set("user_id", userID)
		}
		if tenantID != "" {
			c.Set("tenant_id", tenantID)
		}
		mw.RequireTenantRelation(required)(c)
	}, func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func do(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)
	return w
}

func TestMiddleware_AuthorizedRequest_Allowed(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleAdmin, "t1")
	mw := NewMiddleware(f, nil)
	w := do(newRouter(mw, RoleAdmin, "u1", "t1"))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestMiddleware_DeniedRequest_Returns404(t *testing.T) {
	f := NewFakeClient()
	mw := NewMiddleware(f, nil)
	w := do(newRouter(mw, RoleAdmin, "u1", "t1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404", w.Code)
	}
}

func TestMiddleware_MissingUserID_Returns404(t *testing.T) {
	mw := NewMiddleware(NewFakeClient(), nil)
	w := do(newRouter(mw, RoleAdmin, "", "t1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404", w.Code)
	}
}

func TestMiddleware_MissingTenantID_Returns404(t *testing.T) {
	mw := NewMiddleware(NewFakeClient(), nil)
	w := do(newRouter(mw, RoleAdmin, "u1", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404", w.Code)
	}
}

func TestMiddleware_FGAError_Returns500(t *testing.T) {
	mw := NewMiddleware(errClient{}, nil)
	w := do(newRouter(mw, RoleAdmin, "u1", "t1"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d; want 500", w.Code)
	}
}

func TestMiddleware_RequireRoleAdmin_GrantedStaff_Returns404(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleStaff, "t1")
	mw := NewMiddleware(f, nil)
	w := do(newRouter(mw, RoleAdmin, "u1", "t1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d; want 404", w.Code)
	}
}

func TestMiddleware_RequireRoleStaff_GrantedAdmin_Allowed(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleAdmin, "t1")
	mw := NewMiddleware(f, nil)
	w := do(newRouter(mw, RoleStaff, "u1", "t1"))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200; body=%s", w.Code, w.Body.String())
	}
}
