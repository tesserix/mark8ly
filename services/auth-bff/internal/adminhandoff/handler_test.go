package adminhandoff_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/adminhandoff"
	"github.com/mark8ly/auth-bff/internal/session"
)

func mintCode(t *testing.T, claims adminhandoff.Claims, key string) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

type fakeFGA struct {
	allow  bool
	err    error
	called bool
	gotUID string
	gotTID string
}

func (f *fakeFGA) CheckMembership(_ context.Context, uid, tid string) (bool, error) {
	f.called = true
	f.gotUID = uid
	f.gotTID = tid
	return f.allow, f.err
}

type fakeMinter struct {
	called bool
	gotS   session.Session
	gotDom string
	err    error
}

func (m *fakeMinter) MintWithDomain(w http.ResponseWriter, s session.Session, domain string) error {
	m.called = true
	m.gotS = s
	m.gotDom = domain
	if m.err != nil {
		return m.err
	}
	http.SetCookie(w, &http.Cookie{Name: "m8_session", Value: "test", Domain: domain, Path: "/"})
	return nil
}

func makeRouter(h *adminhandoff.Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/auth")
	h.Register(g)
	return r
}

func handlerClaims() adminhandoff.Claims {
	return adminhandoff.Claims{
		Kind:       adminhandoff.CodeKind,
		UID:        "uid-1",
		Email:      "m@primasyss.com",
		TenantID:   "tenant-1",
		StoreID:    "store-1",
		TargetHost: "admin.primasyss.com",
		Exp:        time.Now().Add(30 * time.Second).Unix(),
	}
}

func postCode(t *testing.T, router *gin.Engine, code string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"code":"` + code + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/mint-from-admin-handoff", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestMint_HappyPath(t *testing.T) {
	fga := &fakeFGA{allow: true}
	minter := &fakeMinter{}
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      fga,
		Sessions: minter,
	})
	router := makeRouter(h)

	code := mintCode(t, handlerClaims(), testKey)
	w := postCode(t, router, code)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}
	if !fga.called || fga.gotTID != "tenant-1" {
		t.Errorf("fga: not called or wrong tenant: %+v", fga)
	}
	if !minter.called || minter.gotDom != "admin.primasyss.com" || minter.gotS.UID != "uid-1" {
		t.Errorf("minter: not called correctly: %+v", minter)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "Domain=admin.primasyss.com") {
		t.Errorf("missing Domain= attribute: %q", w.Header().Get("Set-Cookie"))
	}
}

func TestMint_RejectsExpired(t *testing.T) {
	c := handlerClaims()
	c.Exp = time.Now().Add(-time.Second).Unix()
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      &fakeFGA{allow: true},
		Sessions: &fakeMinter{},
	})
	w := postCode(t, makeRouter(h), mintCode(t, c, testKey))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", w.Code)
	}
}

func TestMint_RejectsBadSignature(t *testing.T) {
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      &fakeFGA{allow: true},
		Sessions: &fakeMinter{},
	})
	// Mint with wrong key.
	w := postCode(t, makeRouter(h), mintCode(t, handlerClaims(), "another-32-byte-key-for-testing!"))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", w.Code)
	}
}

func TestMint_RejectsNonMember(t *testing.T) {
	fga := &fakeFGA{allow: false}
	minter := &fakeMinter{}
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      fga,
		Sessions: minter,
	})
	w := postCode(t, makeRouter(h), mintCode(t, handlerClaims(), testKey))
	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", w.Code)
	}
	if minter.called {
		t.Errorf("minter should not be called when FGA denies")
	}
}

func TestMint_RejectsCanonicalTargetHost(t *testing.T) {
	c := handlerClaims()
	c.TargetHost = "admin.mark8ly.com"
	minter := &fakeMinter{}
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      &fakeFGA{allow: true},
		Sessions: minter,
	})
	w := postCode(t, makeRouter(h), mintCode(t, c, testKey))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
	if minter.called {
		t.Errorf("minter should not be called when target host is canonical")
	}
}

// TestMint_PreservesAuthContext is the regression test for the third
// rebuild site D1's amendment names: the cross-TLD handoff must not
// silently drop AuthContext off the Session it mints.
func TestMint_PreservesAuthContext(t *testing.T) {
	minter := &fakeMinter{}
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      &fakeFGA{allow: true},
		Sessions: minter,
	})

	c := handlerClaims()
	c.AuthContext = "break_glass"
	w := postCode(t, makeRouter(h), mintCode(t, c, testKey))
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}
	if !minter.called {
		t.Fatal("minter was not called")
	}
	if minter.gotS.AuthContext != "break_glass" {
		t.Errorf("AuthContext = %q, want %q — adminhandoff dropped it", minter.gotS.AuthContext, "break_glass")
	}
}

func TestMint_RejectsFGAError(t *testing.T) {
	fga := &fakeFGA{err: errors.New("boom")}
	h := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  testKey,
		FGA:      fga,
		Sessions: &fakeMinter{},
	})
	w := postCode(t, makeRouter(h), mintCode(t, handlerClaims(), testKey))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500", w.Code)
	}
}
