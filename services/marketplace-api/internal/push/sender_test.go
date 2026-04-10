package push

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSender_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		var messages []pushMessage
		if err := json.NewDecoder(r.Body).Decode(&messages); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}
		if messages[0].Title != "Test Title" {
			t.Errorf("expected title 'Test Title', got %q", messages[0].Title)
		}
		if messages[0].Data["deep_link"] != "/orders/123" {
			t.Errorf("expected deep_link '/orders/123', got %q", messages[0].Data["deep_link"])
		}

		resp := pushResponse{
			Data: []pushResult{
				{Status: "ok"},
				{Status: "ok"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	sender := NewSender(server.Client())
	// Override the URL by using a custom transport.
	sender = newTestSender(server)

	result, err := sender.Send(
		[]string{"ExponentPushToken[aaa]", "ExponentPushToken[bbb]"},
		"Test Title",
		"Test Body",
		"/orders/123",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.StaleTokens) != 0 {
		t.Errorf("expected 0 stale tokens, got %d", len(result.StaleTokens))
	}
}

func TestSender_Send_StaleToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := pushResponse{
			Data: []pushResult{
				{Status: "ok"},
				{
					Status:  "error",
					Details: map[string]interface{}{"error": "DeviceNotRegistered"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	sender := newTestSender(server)

	result, err := sender.Send(
		[]string{"ExponentPushToken[good]", "ExponentPushToken[stale]"},
		"Title",
		"Body",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.StaleTokens) != 1 {
		t.Fatalf("expected 1 stale token, got %d", len(result.StaleTokens))
	}
	if result.StaleTokens[0] != "ExponentPushToken[stale]" {
		t.Errorf("expected stale token 'ExponentPushToken[stale]', got %q", result.StaleTokens[0])
	}
}

func TestSender_Send_EmptyTokens(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	sender := newTestSender(server)

	result, err := sender.Send(nil, "Title", "Body", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.StaleTokens) != 0 {
		t.Errorf("expected 0 stale tokens, got %d", len(result.StaleTokens))
	}
	if called {
		t.Error("HTTP server should not have been called for empty tokens")
	}
}

func TestSender_Send_Batching(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)

		var messages []pushMessage
		if err := json.NewDecoder(r.Body).Decode(&messages); err != nil {
			t.Errorf("decode request body: %v", err)
			return
		}

		results := make([]pushResult, len(messages))
		for i := range results {
			results[i] = pushResult{Status: "ok"}
		}

		resp := pushResponse{Data: results}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	sender := newTestSender(server)

	// Generate 150 tokens — should result in 2 batches (100 + 50).
	tokens := make([]string, 150)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("ExponentPushToken[token%d]", i)
	}

	result, err := sender.Send(tokens, "Title", "Body", "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.StaleTokens) != 0 {
		t.Errorf("expected 0 stale tokens, got %d", len(result.StaleTokens))
	}
	if count := requestCount.Load(); count != 2 {
		t.Errorf("expected 2 HTTP requests (batching), got %d", count)
	}
}

// newTestSender creates a Sender that routes requests to the test server
// instead of the real Expo Push API.
func newTestSender(server *httptest.Server) *Sender {
	client := server.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		baseURL: server.URL,
	}
	return NewSender(client)
}

// rewriteTransport rewrites all request URLs to point at the test server.
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	return t.base.RoundTrip(req)
}
