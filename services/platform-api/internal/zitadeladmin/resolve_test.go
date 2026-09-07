package zitadeladmin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/mark8ly/platform-api/internal/idperr"
)

func searchResponse(entries ...map[string]any) []byte {
	// A zero-match search on the real instance OMITS "result" entirely
	// rather than returning an empty array — protojson elides empty
	// values (verified live 2026-09-04, see the package doc). Reproducing
	// that exactly matters: a regression that started keying off the
	// key's presence instead of len() would pass against a `{"result":[]}`
	// fixture and fail in production.
	body := map[string]any{"details": map[string]any{}}
	if len(entries) > 0 {
		body["result"] = entries
	}
	raw, _ := json.Marshal(body)
	return raw
}

func humanEntry(userID, email string, verified bool) map[string]any {
	return map[string]any{
		"userId": userID,
		"human": map[string]any{
			"email": map[string]any{
				"email":      email,
				"isVerified": verified,
			},
		},
	}
}

func TestResolveUserIDByEmail_Found(t *testing.T) {
	var gotOrgHeader string
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotOrgHeader = r.Header.Get("x-zitadel-orgid")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse(humanEntry("user-1", "merchant@example.com", true)))
	})

	id, err := c.resolveUserIDByEmail(context.Background(), "Merchant@Example.com")
	if err != nil {
		t.Fatalf("resolveUserIDByEmail: %v", err)
	}
	if id != "user-1" {
		t.Errorf("id = %q, want user-1", id)
	}
	if gotOrgHeader != "org-tesserix" {
		t.Errorf("x-zitadel-orgid = %q, want org-tesserix", gotOrgHeader)
	}
	q, ok := gotBody["queries"].([]any)
	if !ok || len(q) != 1 {
		t.Fatalf("queries = %v", gotBody["queries"])
	}
}

func TestResolveUserIDByEmail_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse())
	})

	_, err := c.resolveUserIDByEmail(context.Background(), "nobody@example.com")
	if !errors.Is(err, idperr.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestResolveUserIDByEmail_UnverifiedEmailDoesNotMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse(humanEntry("user-1", "merchant@example.com", false)))
	})

	_, err := c.resolveUserIDByEmail(context.Background(), "merchant@example.com")
	if !errors.Is(err, idperr.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound for an unverified-email match", err)
	}
}

// An ambiguous match must not silently pick one. This is the explicit
// scenario called out in the task brief.
func TestResolveUserIDByEmail_AmbiguousRefuses(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse(
			humanEntry("user-1", "merchant@example.com", true),
			humanEntry("user-2", "merchant@example.com", true),
		))
	})

	_, err := c.resolveUserIDByEmail(context.Background(), "merchant@example.com")
	if !errors.Is(err, ErrAmbiguousEmail) {
		t.Fatalf("err = %v, want ErrAmbiguousEmail", err)
	}
	// Ambiguity is NOT one of idperr's sentinels: it must fall through
	// internal/auth/handler.go's switch to the generic 500 default rather
	// than being reported as "not found" (false) or "unavailable"
	// (misdirecting anyone reading the log).
	for _, sentinel := range []error{
		idperr.ErrUserNotFound, idperr.ErrInvalidOobCode, idperr.ErrWeakPassword,
		idperr.ErrUnauthenticated, idperr.ErrTooManyAttempts, idperr.ErrUnavailable,
	} {
		if errors.Is(err, sentinel) {
			t.Errorf("ambiguous-match error unexpectedly matches sentinel %v", sentinel)
		}
	}
}
