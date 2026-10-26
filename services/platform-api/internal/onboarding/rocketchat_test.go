package onboarding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRocketChatDoesNotAcceptAnotherSendersMessage(t *testing.T) {
	posts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			t.Error("must not overwrite a different sender's message")
		}
		_, _ = w.Write([]byte(`{"success":true,"message":{"_id":"mark8ly-onboarding-tenant-123","rid":"room","u":{"_id":"someone-else"}}}`))
	}))
	defer server.Close()
	h, err := NewRocketChatHandler(RocketChatConfig{URL: server.URL, UserID: "bot", Token: "token", RoomID: "room", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if err := h(t.Context(), json.RawMessage(`{"tenant_id":"tenant-123","store_id":"store-123"}`)); err == nil {
		t.Fatal("accepted mismatched message")
	}
	if posts != 0 {
		t.Fatal("unexpected send")
	}
}

func TestRocketChatHonorsCancellationAndRejectsRedirects(t *testing.T) {
	forwarded := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++ }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	h, err := NewRocketChatHandler(RocketChatConfig{URL: server.URL, UserID: "bot", Token: "token", RoomID: "room", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"tenant_id":"tenant-123","store_id":"store-123"}`)
	if err := h(t.Context(), payload); err == nil {
		t.Fatal("accepted redirect")
	}
	if forwarded != 0 {
		t.Fatal("credentials forwarded on redirect")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := h(ctx, payload); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestRocketChatFormatsSignupAndDeduplicatesDelivery(t *testing.T) {
	var saved map[string]any
	posts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "test-token" || r.Header.Get("X-User-Id") != "test-bot" {
			t.Error("missing authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if saved == nil {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"success":false}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": saved})
			return
		}
		if r.URL.Path != "/api/v1/chat.sendMessage" {
			t.Error("unexpected endpoint")
		}
		var body struct {
			Message map[string]any `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		posts++
		saved = body.Message
		saved["u"] = map[string]any{"_id": "test-bot"}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": saved})
	}))
	defer server.Close()
	handler, err := NewRocketChatHandler(RocketChatConfig{URL: server.URL, UserID: "test-bot", Token: "test-token", RoomID: "onboarding-room", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"tenant_id":"tenant-123","store_id":"store-123","owner_name":"Jane @all","owner_email":"jane@example.invalid","business_name":"Example **Store**","slug":"example","country":"AU","currency":"AUD","timezone":"Australia/Melbourne","signed_up_at":"2026-10-26T04:00:00Z","classification":"real","admin_url":"https://example-admin.mark8ly.com","storefront_url":"https://example.mark8ly.com"}`)
	for range 2 {
		if err := handler(t.Context(), payload); err != nil {
			t.Fatal(err)
		}
	}
	if posts != 1 {
		t.Fatalf("posted %d times, want 1", posts)
	}
	if saved["rid"] != "onboarding-room" || saved["_id"] != "mark8ly-onboarding-tenant-123" {
		t.Fatalf("wrong destination or id: %v", saved)
	}
	raw, _ := json.Marshal(saved)
	for _, want := range []string{"jane@example.invalid", "Australia/Melbourne", "AUD", "tenant-123", "2026-10-26T04:00:00Z", "https://example.mark8ly.com"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(string(raw), "@all") || strings.Contains(string(raw), "**Store**") {
		t.Fatal("user text can inject mentions or formatting")
	}
}

func TestRocketChatFailureDoesNotExposeCredentialsOrResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte("test-token jane@example.invalid"))
	}))
	defer server.Close()
	h, err := NewRocketChatHandler(RocketChatConfig{URL: server.URL, UserID: "test-bot", Token: "test-token", RoomID: "room", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	err = h(t.Context(), json.RawMessage(`{"tenant_id":"tenant-123","store_id":"store-123"}`))
	if err == nil || strings.Contains(err.Error(), "test-token") || strings.Contains(err.Error(), "jane@") {
		t.Fatalf("unsafe or missing failure: %v", err)
	}
}

func TestRocketChatRejectsInsecureOrIncompleteConfiguration(t *testing.T) {
	for _, cfg := range []RocketChatConfig{{}, {URL: "http://example.com", UserID: "bot", Token: "token", RoomID: "room"}, {URL: "https://user:password@example.com", UserID: "bot", Token: "token", RoomID: "room"}} {
		if _, err := NewRocketChatHandler(cfg); err == nil {
			t.Fatal("accepted invalid configuration")
		}
	}
}
