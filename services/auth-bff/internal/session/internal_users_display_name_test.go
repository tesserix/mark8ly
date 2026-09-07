package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/internalauth"
)

type fakeResolver struct {
	name  string
	err   error
	calls int
}

func (f *fakeResolver) UserDisplayName(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.name, f.err
}

func displayNameRequest(t *testing.T, h *InternalUsersHandler, secret string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r.Group("/internal"))

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u-1/display-name", nil)
	if secret != "" {
		req.Header.Set(internalauth.Header, secret)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeDisplayName(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return body.Data.DisplayName
}

func TestDisplayNameReturnsResolvedName(t *testing.T) {
	res := &fakeResolver{name: "Jane Roe"}
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).WithDisplayNames(res)

	w := displayNameRequest(t, h, "s3cret")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200: %s", w.Code, w.Body.String())
	}
	if got := decodeDisplayName(t, w); got != "Jane Roe" {
		t.Errorf("display_name=%q, want %q", got, "Jane Roe")
	}
}

// A user with no name a person supplied is a 200 with a blank, not an
// error: the caller seeds an empty field rather than inventing one.
func TestDisplayNameBlankIsSuccess(t *testing.T) {
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).WithDisplayNames(&fakeResolver{name: ""})

	w := displayNameRequest(t, h, "s3cret")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200", w.Code)
	}
	if got := decodeDisplayName(t, w); got != "" {
		t.Errorf("display_name=%q, want empty", got)
	}
}

// No Zitadel client on this deployment: there is genuinely no name to
// give, which is a blank answer rather than a failure.
func TestDisplayNameWithoutResolverIsBlank(t *testing.T) {
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil)

	w := displayNameRequest(t, h, "s3cret")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200", w.Code)
	}
	if got := decodeDisplayName(t, w); got != "" {
		t.Errorf("display_name=%q, want empty", got)
	}
}

func TestDisplayNameUpstreamFailureIs502(t *testing.T) {
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).
		WithDisplayNames(&fakeResolver{err: errors.New("zitadel unreachable")})

	w := displayNameRequest(t, h, "s3cret")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("code=%d, want 502", w.Code)
	}
}

func TestDisplayNameRejectsWrongSecret(t *testing.T) {
	res := &fakeResolver{name: "Jane Roe"}
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).WithDisplayNames(res)

	w := displayNameRequest(t, h, "wrong")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d, want 401", w.Code)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times on a rejected request, want 0", res.calls)
	}
}

// An unset secret fails closed, exactly as the delete endpoint does.
func TestDisplayNameWithoutSecretIsUnavailable(t *testing.T) {
	res := &fakeResolver{name: "Jane Roe"}
	h := NewInternalUsersHandler(nil, nil, "", nil).WithDisplayNames(res)

	w := displayNameRequest(t, h, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d, want 503", w.Code)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times with no secret configured, want 0", res.calls)
	}
}
