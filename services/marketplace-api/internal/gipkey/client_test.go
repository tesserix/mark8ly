package gipkey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/api/apikeys/v2"
	"google.golang.org/api/option"
)

const testKeyResource = "projects/12345/locations/global/keys/abcd-1234"

func TestNoop(t *testing.T) {
	t.Parallel()
	c := Noop{}
	if err := c.AddDomain(context.Background(), "primasyss.com"); err != nil {
		t.Fatalf("Noop.AddDomain: %v", err)
	}
	if err := c.RemoveDomain(context.Background(), "primasyss.com"); err != nil {
		t.Fatalf("Noop.RemoveDomain: %v", err)
	}
}

func TestMergePatterns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		existing, want []string
		expect         []string
	}{
		{
			name:     "appends fresh patterns",
			existing: []string{"https://mark8ly.com/*"},
			want:     []string{"https://primasyss.com/*", "https://*.primasyss.com/*"},
			expect:   []string{"https://mark8ly.com/*", "https://primasyss.com/*", "https://*.primasyss.com/*"},
		},
		{
			name:     "idempotent on duplicates",
			existing: []string{"https://primasyss.com/*", "https://mark8ly.com/*"},
			want:     []string{"https://primasyss.com/*", "https://*.primasyss.com/*"},
			expect:   []string{"https://primasyss.com/*", "https://mark8ly.com/*", "https://*.primasyss.com/*"},
		},
		{
			name:     "preserves original ordering",
			existing: []string{"https://a.com/*", "https://b.com/*", "https://c.com/*"},
			want:     []string{"https://b.com/*", "https://d.com/*"},
			expect:   []string{"https://a.com/*", "https://b.com/*", "https://c.com/*", "https://d.com/*"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mergePatterns(tc.existing, tc.want)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Fatalf("mergePatterns = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestRemovePatterns(t *testing.T) {
	t.Parallel()
	got := removePatterns(
		[]string{"https://a.com/*", "https://primasyss.com/*", "https://b.com/*", "https://*.primasyss.com/*"},
		[]string{"https://primasyss.com/*", "https://*.primasyss.com/*"},
	)
	want := []string{"https://a.com/*", "https://b.com/*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removePatterns = %v, want %v", got, want)
	}
}

func TestSameSet(t *testing.T) {
	t.Parallel()
	if !sameSet([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatalf("sameSet should treat [a,b] and [b,a] as equal")
	}
	if sameSet([]string{"a"}, []string{"a", "b"}) {
		t.Fatalf("sameSet on different lengths must be false")
	}
}

// keyServer is an in-memory stand-in for Google's API Keys v2 service.
// It supports the two RPCs gipkey uses — Get and Patch — and lets the
// test drive optimistic-concurrency conflicts via the conflictsLeft
// counter so the retry path is actually exercised.
type keyServer struct {
	mu              sync.Mutex
	key             apikeys.V2Key
	patches         int
	gets            int
	conflictsLeft   int32
	updateMaskValue string
}

func (s *keyServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.mu.Lock()
			s.gets++
			body, err := json.Marshal(s.key)
			s.mu.Unlock()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case http.MethodPatch:
			if atomic.LoadInt32(&s.conflictsLeft) > 0 {
				atomic.AddInt32(&s.conflictsLeft, -1)
				http.Error(w, `{"error":{"code":409,"message":"conflict"}}`, http.StatusConflict)
				return
			}
			body, _ := io.ReadAll(r.Body)
			var patch apikeys.V2Key
			if err := json.Unmarshal(body, &patch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			s.patches++
			s.updateMaskValue = r.URL.Query().Get("updateMask")
			if patch.Restrictions != nil && patch.Restrictions.BrowserKeyRestrictions != nil {
				if s.key.Restrictions == nil {
					s.key.Restrictions = &apikeys.V2Restrictions{}
				}
				s.key.Restrictions.BrowserKeyRestrictions = patch.Restrictions.BrowserKeyRestrictions
			}
			s.key.Etag = "etag-" + http.StatusText(s.patches)
			out, _ := json.Marshal(s.key)
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func newClientForTest(t *testing.T, server *keyServer) *googleClient {
	t.Helper()
	ts := httptest.NewServer(server.handler())
	t.Cleanup(ts.Close)

	ctx := context.Background()
	c, err := New(ctx, testKeyResource, nil,
		option.WithEndpoint(ts.URL),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("gipkey.New: %v", err)
	}
	gc, ok := c.(*googleClient)
	if !ok {
		t.Fatalf("New returned %T, want *googleClient", c)
	}
	return gc
}

func TestGoogleClient_AddDomain_NoOpWhenAlreadyPresent(t *testing.T) {
	t.Parallel()
	s := &keyServer{
		key: apikeys.V2Key{
			Etag: "etag-0",
			Restrictions: &apikeys.V2Restrictions{
				BrowserKeyRestrictions: &apikeys.V2BrowserKeyRestrictions{
					AllowedReferrers: []string{
						"https://mark8ly.com/*",
						"https://*.mark8ly.com/*",
						"https://primasyss.com/*",
						"https://*.primasyss.com/*",
					},
				},
			},
		},
	}
	c := newClientForTest(t, s)
	if err := c.AddDomain(context.Background(), "primasyss.com"); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.patches != 0 {
		t.Fatalf("expected no Patch when allowlist already covers domain, got %d patches", s.patches)
	}
}

func TestGoogleClient_AddDomain_AppendsAndPreservesOthers(t *testing.T) {
	t.Parallel()
	s := &keyServer{
		key: apikeys.V2Key{
			Etag: "etag-0",
			Restrictions: &apikeys.V2Restrictions{
				ApiTargets: []*apikeys.V2ApiTarget{
					{Service: "identitytoolkit.googleapis.com"},
				},
				BrowserKeyRestrictions: &apikeys.V2BrowserKeyRestrictions{
					AllowedReferrers: []string{
						"https://mark8ly.com/*",
						"https://*.mark8ly.com/*",
					},
				},
			},
		},
	}
	c := newClientForTest(t, s)
	if err := c.AddDomain(context.Background(), "primasyss.com"); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.patches != 1 {
		t.Fatalf("expected exactly 1 Patch, got %d", s.patches)
	}
	got := s.key.Restrictions.BrowserKeyRestrictions.AllowedReferrers
	want := []string{
		"https://mark8ly.com/*",
		"https://*.mark8ly.com/*",
		"https://primasyss.com/*",
		"https://*.primasyss.com/*",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allowedReferrers = %v, want %v", got, want)
	}
	if len(s.key.Restrictions.ApiTargets) != 1 {
		t.Fatalf("apiTargets must be preserved, got %d entries", len(s.key.Restrictions.ApiTargets))
	}
	if !strings.Contains(s.updateMaskValue, "allowed_referrers") {
		t.Fatalf("updateMask should scope to allowed_referrers, got %q", s.updateMaskValue)
	}
}

func TestGoogleClient_RemoveDomain(t *testing.T) {
	t.Parallel()
	s := &keyServer{
		key: apikeys.V2Key{
			Etag: "etag-0",
			Restrictions: &apikeys.V2Restrictions{
				BrowserKeyRestrictions: &apikeys.V2BrowserKeyRestrictions{
					AllowedReferrers: []string{
						"https://mark8ly.com/*",
						"https://primasyss.com/*",
						"https://*.primasyss.com/*",
					},
				},
			},
		},
	}
	c := newClientForTest(t, s)
	if err := c.RemoveDomain(context.Background(), "primasyss.com"); err != nil {
		t.Fatalf("RemoveDomain: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	got := s.key.Restrictions.BrowserKeyRestrictions.AllowedReferrers
	want := []string{"https://mark8ly.com/*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after RemoveDomain, allowedReferrers = %v, want %v", got, want)
	}
}

func TestGoogleClient_AddDomain_RetriesOnConflict(t *testing.T) {
	t.Parallel()
	s := &keyServer{
		conflictsLeft: 2,
		key: apikeys.V2Key{
			Etag: "etag-0",
			Restrictions: &apikeys.V2Restrictions{
				BrowserKeyRestrictions: &apikeys.V2BrowserKeyRestrictions{
					AllowedReferrers: []string{"https://mark8ly.com/*"},
				},
			},
		},
	}
	c := newClientForTest(t, s)
	if err := c.AddDomain(context.Background(), "primasyss.com"); err != nil {
		t.Fatalf("AddDomain after conflicts: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.patches != 1 {
		t.Fatalf("expected 1 successful Patch after retries, got %d", s.patches)
	}
	if s.gets < 3 {
		t.Fatalf("expected at least 3 Gets (initial + 2 retries), got %d", s.gets)
	}
}

func TestGoogleClient_AddDomain_RejectsBadDomain(t *testing.T) {
	t.Parallel()
	s := &keyServer{}
	c := newClientForTest(t, s)
	if err := c.AddDomain(context.Background(), "localhost"); err == nil {
		t.Fatalf("AddDomain on unusable domain should error")
	}
}
