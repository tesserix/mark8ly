package stripewebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const ourURL = "https://api.mark8ly.com/api/v1/webhooks/the-bondi-store/stripe"

type call struct {
	method string
	path   string
	body   string
}

// stripeStub records every call and serves canned endpoint lists.
type stripeStub struct {
	existing  []map[string]string // id + url pairs already registered
	calls     []call
	createErr int  // non-zero → create responds with this status
	noSecret  bool // create responds 200 but without a secret
	pages     [][]map[string]string
}

func (s *stripeStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.calls = append(s.calls, call{r.Method, r.URL.Path + "?" + r.URL.RawQuery, string(raw)})

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/webhook_endpoints":
			if len(s.pages) > 0 {
				page := s.pages[0]
				s.pages = s.pages[1:]
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": page, "has_more": len(s.pages) > 0,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": s.existing, "has_more": false,
			})
		case r.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{"deleted": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/webhook_endpoints":
			if s.createErr != 0 {
				w.WriteHeader(s.createErr)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"message": "Invalid API Key provided"},
				})
				return
			}
			out := map[string]any{"id": "we_new"}
			if !s.noSecret {
				out["secret"] = "whsec_abc123"
			}
			_ = json.NewEncoder(w).Encode(out)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func provisioner(t *testing.T, s *stripeStub) *Provisioner {
	return New().WithAPIBase(s.server(t).URL)
}

func (s *stripeStub) counts() (gets, posts, deletes int) {
	for _, c := range s.calls {
		switch c.method {
		case http.MethodGet:
			gets++
		case http.MethodPost:
			posts++
		case http.MethodDelete:
			deletes++
		}
	}
	return
}

func TestEnsureCreatesWhenNoEndpointExists(t *testing.T) {
	s := &stripeStub{}
	res, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, false, nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if res.Action != ActionCreated {
		t.Errorf("action = %q, want created", res.Action)
	}
	if res.Secret != "whsec_abc123" {
		t.Errorf("secret = %q, want the created signing secret", res.Secret)
	}
	_, _, deletes := s.counts()
	if deletes != 0 {
		t.Errorf("deletes = %d, want 0 — nothing existed to replace", deletes)
	}
}

// THE idempotency case. Saving payment settings repeatedly must not churn
// the endpoint: a delete+recreate rotates the signing secret and drops any
// event in flight.
func TestEnsureIsIdempotentWhenSecretAlreadyHeld(t *testing.T) {
	s := &stripeStub{existing: []map[string]string{{"id": "we_1", "url": ourURL}}}
	res, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, true, nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Errorf("action = %q, want unchanged", res.Action)
	}
	if res.Secret != "" {
		t.Errorf("secret = %q, want empty — caller already holds it", res.Secret)
	}
	_, posts, deletes := s.counts()
	if posts != 0 || deletes != 0 {
		t.Errorf("posts=%d deletes=%d, want 0/0 — endpoint must not be touched", posts, deletes)
	}
}

// An endpoint exists but we hold no secret: unrecoverable by reading,
// because Stripe returns `secret` only at creation. Replace it.
func TestEnsureReplacesEndpointWhenSecretMissing(t *testing.T) {
	s := &stripeStub{existing: []map[string]string{{"id": "we_1", "url": ourURL}}}
	res, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, false, nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if res.Action != ActionReplaced {
		t.Errorf("action = %q, want replaced", res.Action)
	}
	if res.Secret == "" {
		t.Error("secret is empty, want the recreated endpoint's secret")
	}
	_, _, deletes := s.counts()
	if deletes != 1 {
		t.Errorf("deletes = %d, want exactly 1", deletes)
	}
}

// A merchant's own unrelated endpoints must never be deleted.
func TestEnsureIgnoresOtherEndpoints(t *testing.T) {
	s := &stripeStub{existing: []map[string]string{
		{"id": "we_theirs", "url": "https://merchant.example.com/stripe"},
		{"id": "we_other", "url": "https://api.mark8ly.com/api/v1/webhooks/another-store/stripe"},
	}}
	res, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, false, nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if res.Action != ActionCreated {
		t.Errorf("action = %q, want created — no endpoint matched our URL", res.Action)
	}
	_, _, deletes := s.counts()
	if deletes != 0 {
		t.Errorf("deletes = %d, want 0 — another store's endpoint must not be touched", deletes)
	}
}

// Ours sitting past the first page must not read as "absent", or every
// save would register another duplicate endpoint.
func TestEnsureFindsEndpointOnLaterPage(t *testing.T) {
	first := make([]map[string]string, 0, 100)
	for i := 0; i < 100; i++ {
		first = append(first, map[string]string{
			"id": fmt.Sprintf("we_%d", i), "url": fmt.Sprintf("https://x.example.com/%d", i),
		})
	}
	s := &stripeStub{pages: [][]map[string]string{
		first,
		{{"id": "we_ours", "url": ourURL}},
	}}
	res, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, true, nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Errorf("action = %q, want unchanged — ours was on page 2", res.Action)
	}
	gets, posts, _ := s.counts()
	if gets != 2 {
		t.Errorf("gets = %d, want 2 pages walked", gets)
	}
	if posts != 0 {
		t.Errorf("posts = %d, want 0 — a duplicate endpoint must not be created", posts)
	}
}

func TestEnsureSubscribesToTheOrderLifecycleEvents(t *testing.T) {
	s := &stripeStub{}
	if _, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, false, nil); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	var created string
	for _, c := range s.calls {
		if c.method == http.MethodPost {
			created = c.body
		}
	}
	for _, want := range []string{"checkout.session.completed", "payment_intent.succeeded"} {
		if !strings.Contains(created, want) {
			t.Errorf("create body missing %q; got %s", want, created)
		}
	}
}

func TestEnsureSurfacesStripeErrors(t *testing.T) {
	s := &stripeStub{createErr: http.StatusUnauthorized}
	_, err := provisioner(t, s).Ensure(context.Background(), "sk_bad", ourURL, false, nil)
	if err == nil {
		t.Fatal("want an error for a rejected key")
	}
	if !strings.Contains(err.Error(), "Invalid API Key") {
		t.Errorf("err = %v, want Stripe's own message surfaced", err)
	}
}

// An endpoint with no signing secret receives events we can never verify —
// strictly worse than having none, because Stripe reports delivery as fine.
func TestEnsureRejectsCreateWithoutSecret(t *testing.T) {
	s := &stripeStub{noSecret: true}
	_, err := provisioner(t, s).Ensure(context.Background(), "sk_test_x", ourURL, false, nil)
	if err == nil {
		t.Fatal("want an error when Stripe returns no signing secret")
	}
}

func TestEndpointURL(t *testing.T) {
	got := EndpointURL("https://api.mark8ly.com/", "the-bondi-store")
	if got != ourURL {
		t.Errorf("EndpointURL = %q, want %q", got, ourURL)
	}
}

func TestEnsureValidatesInputs(t *testing.T) {
	s := &stripeStub{}
	p := provisioner(t, s)
	if _, err := p.Ensure(context.Background(), "", ourURL, false, nil); err == nil {
		t.Error("want an error for an empty secret key")
	}
	if _, err := p.Ensure(context.Background(), "sk_test_x", "", false, nil); err == nil {
		t.Error("want an error for an empty endpoint url")
	}
}
