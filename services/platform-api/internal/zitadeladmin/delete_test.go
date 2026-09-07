package zitadeladmin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/internal/idperr"
)

func TestDeleteAccount_SucceedsOn200(t *testing.T) {
	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.DeleteAccount(context.Background(), "user-1"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/v2/users/user-1") {
		t.Errorf("path = %s", gotPath)
	}
}

func TestDeleteAccount_IdempotentOnNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"User not found"}`))
	})
	if err := c.DeleteAccount(context.Background(), "user-1"); err != nil {
		t.Fatalf("expected nil on not-found (idempotent delete), got %v", err)
	}
}

func TestDeleteAccount_RejectsEmptyUID(t *testing.T) {
	c := &Client{}
	if err := c.DeleteAccount(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty uid")
	}
}

func TestDeleteAccount_UnavailableSurfacesAsUnavailable(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":14,"message":"backend unavailable"}`))
	})
	err := c.DeleteAccount(context.Background(), "user-1")
	if !errors.Is(err, idperr.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
