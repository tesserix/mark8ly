package cfclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCF stands in for api.cloudflare.com. Each handler asserts the
// caller's auth header (no token leakage) and serves a minimal
// CF-shaped response.
func fakeCF(t *testing.T, recordID, zoneID string, found bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !found {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []map[string]string{
				{"id": zoneID, "name": r.URL.Query().Get("name")},
			},
		})
	})

	mux.HandleFunc("/zones/"+zoneID+"/dns_records", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{}})
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  map[string]string{"id": recordID},
			})
		default:
			http.Error(w, "bad", http.StatusBadRequest)
		}
	})

	mux.HandleFunc("/zones/"+zoneID+"/dns_records/"+recordID, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"result":null}`))
	})

	return httptest.NewServer(mux)
}

// Cloudflare IDs are 32-char lowercase hex strings — match real shape
// so the cfclient's regex guard accepts them.
const fakeZoneID = "0123456789abcdef0123456789abcdef"
const fakeRecordID = "fedcba9876543210fedcba9876543210"

func TestAddDomain_HappyPath(t *testing.T) {
	srv := fakeCF(t, fakeRecordID, fakeZoneID, true)
	defer srv.Close()

	c := New("edge.mark8ly.com", WithBaseURL(srv.URL))
	zoneID, recordID, err := c.AddDomain(context.Background(), "shop.example.com", "tok")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if zoneID != fakeZoneID || recordID != fakeRecordID {
		t.Fatalf("unexpected ids: %s / %s", zoneID, recordID)
	}
}

func TestAddDomain_ZoneNotFound(t *testing.T) {
	srv := fakeCF(t, fakeRecordID, fakeZoneID, false)
	defer srv.Close()

	c := New("edge.mark8ly.com", WithBaseURL(srv.URL))
	_, _, err := c.AddDomain(context.Background(), "shop.example.com", "tok")
	if err == nil {
		t.Fatal("expected error for missing zone")
	}
	if !strings.Contains(err.Error(), "No Cloudflare zone") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestAddDomain_RejectsBadFQDN(t *testing.T) {
	c := New("edge.mark8ly.com")
	_, _, err := c.AddDomain(context.Background(), "noTLD", "tok")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAddDomain_RequiresCnameTarget(t *testing.T) {
	c := New("")
	_, _, err := c.AddDomain(context.Background(), "shop.example.com", "tok")
	if err == nil {
		t.Fatal("expected error when cnameTarget empty")
	}
}

func TestRemoveDomain_Idempotent(t *testing.T) {
	c := New("edge.mark8ly.com")
	if err := c.RemoveDomain(context.Background(), "", "", "tok"); err != nil {
		t.Fatalf("expected no-op when zone/record empty, got %v", err)
	}
}

func TestRemoveDomain_RejectsMalformedIDs(t *testing.T) {
	c := New("edge.mark8ly.com")
	// IDs shorter than 32 hex chars or containing slashes must be
	// rejected before we paste them into the URL — defends against a
	// tampered DB row tricking the client into hitting an
	// attacker-controlled path on api.cloudflare.com.
	cases := [][2]string{
		{"shortzone", "shortrecord"},
		{strings.Repeat("a", 32), "../../etc/passwd"},
		{"contains/slash", strings.Repeat("a", 32)},
	}
	for _, c2 := range cases {
		if err := c.RemoveDomain(context.Background(), c2[0], c2[1], "tok"); err == nil {
			t.Fatalf("expected rejection for %v", c2)
		}
	}
}

func TestApexOf(t *testing.T) {
	cases := map[string]string{
		"shop.example.com":     "example.com",
		"deep.sub.example.com": "example.com",
		"example.com":          "example.com",
	}
	for in, want := range cases {
		got, err := apexOf(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
	if _, err := apexOf("noTLD"); err == nil {
		t.Fatal("expected error for tldless input")
	}
}
