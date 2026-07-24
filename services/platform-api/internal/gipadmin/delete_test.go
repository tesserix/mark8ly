package gipadmin

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestDeleteAccount_SucceedsOn200(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/accounts:delete") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.DeleteAccount(context.Background(), "uid-1"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteAccount_IdempotentOnUserNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"USER_NOT_FOUND"}}`))
	})
	if err := c.DeleteAccount(context.Background(), "uid-1"); err != nil {
		t.Fatalf("expected nil on USER_NOT_FOUND, got %v", err)
	}
}

func TestDeleteAccount_RejectsEmptyUID(t *testing.T) {
	c := &AdminClient{}
	if err := c.DeleteAccount(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty uid")
	}
}
