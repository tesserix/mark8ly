package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
)

// fixturePath is a recorded plan-catalog response: the 78 rows the live
// endpoint serves, in the shape tesserix-home#427 fixed. Using a file rather
// than the network keeps these tests offline and deterministic — the live
// comparison is a deliberate manual run, described in this command's doc
// comment.
const fixturePath = "testdata/console_catalog.json"

// committedCatalogData is the file -source=console is expected to reproduce.
var committedCatalogData = filepath.Join("..", "..", "catalog_data.go")

// recordedRequest is what a fixture server saw. The requests are asserted,
// not just the returned catalog: a client that fetched the wrong mode, or
// asked for a token without the roles scope, would still hand back a
// well-formed catalog in a test whose stub answered regardless.
type recordedRequest struct {
	method string
	path   string
	query  string
	header http.Header
	form   url.Values
}

type fixtureServer struct {
	*httptest.Server
	mu       sync.Mutex
	token    []recordedRequest
	catalog  []recordedRequest
	response []byte
}

// newFixtureServer serves an OAuth token from /oauth/v2/token and the given
// catalog body from /api/v1/plan-catalog, recording every request.
func newFixtureServer(t *testing.T, body []byte) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{response: body}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token request body did not parse as a form: %v", err)
		}
		fs.record(&fs.token, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fixture-token","expires_in":3600}`))
	})
	mux.HandleFunc("/api/v1/plan-catalog", func(w http.ResponseWriter, r *http.Request) {
		fs.record(&fs.catalog, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fs.response)
	})
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fixtureServer) record(into *[]recordedRequest, r *http.Request) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	*into = append(*into, recordedRequest{
		method: r.Method,
		path:   r.URL.Path,
		query:  r.URL.RawQuery,
		header: r.Header.Clone(),
		form:   r.PostForm,
	})
}

func (fs *fixtureServer) requests() ([]recordedRequest, []recordedRequest) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]recordedRequest{}, fs.token...), append([]recordedRequest{}, fs.catalog...)
}

// testScope carries both reserved Zitadel scopes. The audience scope is what
// puts the project in the token's `aud`; the roles scope is what makes the
// token carry any capability at all. A token minted without both verifies and
// is then refused by the route, so the request assertion below checks for
// both by name.
const testScope = "openid urn:zitadel:iam:org:project:id:386377618200461939:aud urn:zitadel:iam:org:projects:roles"

func fixtureConfig(base string) consolecatalog.Config {
	return consolecatalog.Config{
		CatalogURL:   base + "/api/v1/plan-catalog",
		TokenURL:     base + "/oauth/v2/token",
		ClientID:     "fixture-client",
		ClientSecret: "fixture-secret",
		Scope:        testScope,
		Mode:         "test",
	}
}

func readFixture(t *testing.T) consolecatalog.Catalog {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reading %s: %v", fixturePath, err)
	}
	var catalog consolecatalog.Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decoding %s: %v", fixturePath, err)
	}
	if len(catalog.Prices) != 78 {
		t.Fatalf("%s holds %d prices, want the 78 a complete catalog has", fixturePath, len(catalog.Prices))
	}
	return catalog
}

func serve(t *testing.T, catalog consolecatalog.Catalog) *fixtureServer {
	t.Helper()
	body, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("encoding fixture catalog: %v", err)
	}
	return newFixtureServer(t, body)
}

// TestGenerateFromConsoleReproducesCommittedFile is the offline half of this
// task's proof: a well-formed console payload, read through the real client,
// renders catalog_data.go byte for byte.
func TestGenerateFromConsoleReproducesCommittedFile(t *testing.T) {
	fs := serve(t, readFixture(t))

	got, err := generateFromConsole(context.Background(), newConsoleClient(fixtureConfig(fs.URL)))
	if err != nil {
		t.Fatalf("generateFromConsole: %v", err)
	}

	want, err := os.ReadFile(committedCatalogData)
	if err != nil {
		t.Fatalf("reading %s: %v", committedCatalogData, err)
	}
	if got != string(want) {
		t.Fatalf("console-sourced generation (%d bytes) does not match the committed catalog_data.go (%d bytes)",
			len(got), len(want))
	}
}

// TestGenerateFromConsoleAsksForTheRightThing asserts the requests, not the
// answer. A stub that replies to anything makes a wrong mode, a missing
// bearer token or a scope short of the roles claim invisible — and this
// estate shipped a broken Stripe write for 18 days behind exactly that gap.
func TestGenerateFromConsoleAsksForTheRightThing(t *testing.T) {
	fs := serve(t, readFixture(t))

	if _, err := generateFromConsole(context.Background(), newConsoleClient(fixtureConfig(fs.URL))); err != nil {
		t.Fatalf("generateFromConsole: %v", err)
	}
	tokenReqs, catalogReqs := fs.requests()

	if len(tokenReqs) != 1 {
		t.Fatalf("token endpoint saw %d requests, want exactly 1", len(tokenReqs))
	}
	tok := tokenReqs[0]
	if tok.method != http.MethodPost {
		t.Errorf("token request method = %s, want POST", tok.method)
	}
	if ct := tok.header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("token Content-Type = %q, want application/x-www-form-urlencoded", ct)
	}
	for field, want := range map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     "fixture-client",
		"client_secret": "fixture-secret",
		"scope":         testScope,
	} {
		if got := tok.form.Get(field); got != want {
			t.Errorf("token form %s = %q, want %q", field, got, want)
		}
	}
	// Named individually as well as compared whole, so a future scope change
	// that drops one of the two reserved scopes fails on the scope it dropped.
	for _, required := range []string{
		"urn:zitadel:iam:org:project:id:386377618200461939:aud",
		"urn:zitadel:iam:org:projects:roles",
	} {
		if !strings.Contains(tok.form.Get("scope"), required) {
			t.Errorf("token scope %q is missing %q; short either reserved scope the token still verifies, "+
				"but the route answers 401", tok.form.Get("scope"), required)
		}
	}

	if len(catalogReqs) != 1 {
		t.Fatalf("catalog endpoint saw %d requests, want exactly 1", len(catalogReqs))
	}
	cat := catalogReqs[0]
	if cat.method != http.MethodGet {
		t.Errorf("catalog request method = %s, want GET", cat.method)
	}
	if cat.path != "/api/v1/plan-catalog" {
		t.Errorf("catalog path = %q, want /api/v1/plan-catalog", cat.path)
	}
	if cat.query != "mode=test" {
		t.Errorf("catalog query = %q, want mode=test — reading the wrong mode would regenerate this file from the wrong prices", cat.query)
	}
	if auth := cat.header.Get("Authorization"); auth != "Bearer fixture-token" {
		t.Errorf("catalog Authorization = %q, want the bearer token just minted", auth)
	}
}

// TestGenerateFromConsoleRefusesUnreachableConsole is the refusal that
// matters most. catalog_data.go is the fail-open fallback the serving path
// drops to when the console is down, so a console outage must fail the
// generation rather than rewrite the fallback with whatever it managed to
// read.
func TestGenerateFromConsoleRefusesUnreachableConsole(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"token endpoint 500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"catalog endpoint 503", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth/v2/token" {
				_, _ = w.Write([]byte(`{"access_token":"fixture-token","expires_in":3600}`))
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}},
		{"catalog endpoint 200 with no prices", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth/v2/token" {
				_, _ = w.Write([]byte(`{"access_token":"fixture-token","expires_in":3600}`))
				return
			}
			_, _ = w.Write([]byte(`{"mode":"test","prices":[]}`))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			out, err := generateFromConsole(context.Background(), newConsoleClient(fixtureConfig(srv.URL)))
			if err == nil {
				t.Fatalf("generation succeeded against an unavailable console and produced %d bytes", len(out))
			}
			if out != "" {
				t.Fatalf("generation failed but emitted %d bytes; the fallback must be left untouched", len(out))
			}
		})
	}

	t.Run("no server at all", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close() // nothing is listening any more

		out, err := generateFromConsole(context.Background(), newConsoleClient(fixtureConfig(url)))
		if err == nil {
			t.Fatalf("generation succeeded with nothing listening and produced %d bytes", len(out))
		}
		if out != "" {
			t.Fatalf("generation failed but emitted %d bytes", len(out))
		}
	})
}

// TestGenerateFromConsoleRefusesIncompletePayload separates "the console
// answered" from "the console answered fully". A 200 carrying most of the
// catalog is the case a status-code check waves through, and it is precisely
// the one that would quietly shrink the fallback.
func TestGenerateFromConsoleRefusesIncompletePayload(t *testing.T) {
	full := readFixture(t)

	short := full
	short.Prices = append([]consolecatalog.Price{}, full.Prices[:len(full.Prices)-1]...)
	dropped := full.Prices[len(full.Prices)-1]

	fs := serve(t, short)
	out, err := generateFromConsole(context.Background(), newConsoleClient(fixtureConfig(fs.URL)))
	if err == nil {
		t.Fatalf("generation accepted a payload one price short and produced %d bytes", len(out))
	}
	if out != "" {
		t.Fatalf("generation failed but emitted %d bytes; a partial catalog must never be written", len(out))
	}
	want := dropped.Tier + "/" + dropped.Plan + "/" + dropped.Period + "/" + dropped.Currency
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not name the missing amount (%s)", err, want)
	}
}
