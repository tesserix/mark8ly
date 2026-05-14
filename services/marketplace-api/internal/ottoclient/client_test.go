package ottoclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNew_NilWhenUnconfigured covers the nil-safety contract: when
// either url or secret is empty (typical local dev without otto), the
// constructor returns nil so callers can safely no-op via h.otto == nil.
func TestNew_NilWhenUnconfigured(t *testing.T) {
	if New("", "secret") != nil {
		t.Error("expected nil when URL is empty")
	}
	if New("http://localhost", "") != nil {
		t.Error("expected nil when secret is empty")
	}
	if New("   ", "   ") != nil {
		t.Error("expected nil when both whitespace")
	}
	if c := New("http://localhost", "s3cret"); c == nil {
		t.Error("expected non-nil client when both fields set")
	}
}

// TestGetTranscript_HappyPath covers the on-the-wire contract: the
// client sends scope + auth headers, otto returns 200 with the payload,
// and the client decodes it into Transcript. We assert on every header
// otto's CustomerContext middleware checks so a future header rename on
// either side surfaces here.
func TestGetTranscript_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Internal-Auth"); got != "shh" {
			t.Errorf("X-Internal-Auth = %q, want %q", got, "shh")
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "tenant-123" {
			t.Errorf("X-Tenant-Id = %q, want %q", got, "tenant-123")
		}
		if got := r.Header.Get("X-Store-Id"); got != "store-456" {
			t.Errorf("X-Store-Id = %q, want %q", got, "store-456")
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v1/internal/otto/conversations/conv-789/transcript") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"conversation": {"id":"conv-789","case_id":"CS-260514-A7B2","status":"closed","created_at":"2026-05-14T10:00:00Z","closed_at":"2026-05-14T10:42:00Z"},
			"messages": [
				{"id":"m1","sender_type":"customer","sender_name":"Alice","body":"Hi","created_at":"2026-05-14T10:00:01Z"},
				{"id":"m2","sender_type":"staff","sender_name":"Sara","body":"Hello!","created_at":"2026-05-14T10:00:30Z"}
			]
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	if c == nil {
		t.Fatal("New returned nil")
	}
	tr, err := c.GetTranscript(context.Background(), "tenant-123", "store-456", "conv-789")
	if err != nil {
		t.Fatalf("GetTranscript: %v", err)
	}
	if tr.Conversation.CaseID != "CS-260514-A7B2" {
		t.Errorf("CaseID = %q", tr.Conversation.CaseID)
	}
	if len(tr.Messages) != 2 {
		t.Fatalf("Messages len = %d, want 2", len(tr.Messages))
	}
	if tr.Messages[0].Body != "Hi" {
		t.Errorf("Messages[0].Body = %q", tr.Messages[0].Body)
	}
}

// TestGetTranscript_NotFoundMapsToSentinel covers the 404 path: otto
// returns 404 when the conversation doesn't exist for the given scope.
// The client surfaces this as ErrNotFound so the handler can map back
// to a customer-facing 404 (and not a 500).
func TestGetTranscript_NotFoundMapsToSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	_, err := c.GetTranscript(context.Background(), "t", "s", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestGetTranscript_EmptyConvID short-circuits to ErrNotFound without
// hitting the wire — guards against a caller (handler bug) sending an
// empty conversation_id.
func TestGetTranscript_EmptyConvID(t *testing.T) {
	c := New("http://localhost", "shh")
	_, err := c.GetTranscript(context.Background(), "t", "s", "")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestGetTranscript_UpstreamFailure surfaces an opaque error on 5xx so
// the caller can log + map to 500. We don't want the client swallowing
// a 503 as "not found" — that would silently hide an outage.
func TestGetTranscript_UpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	_, err := c.GetTranscript(context.Background(), "t", "s", "conv-1")
	if err == nil {
		t.Fatal("expected error on 502")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("502 must not be mapped to ErrNotFound, got %v", err)
	}
}

// TestNew_TrimsTrailingSlash protects against a deployer setting
// OTTO_URL with a trailing slash — both forms must produce the same
// request URL.
func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := New("http://otto.example.com/", "shh")
	if c == nil || c.baseURL != "http://otto.example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://otto.example.com")
	}
}
