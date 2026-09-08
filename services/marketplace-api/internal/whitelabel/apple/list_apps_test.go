package apple_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
)

// recordedRequest captures what the client actually sent, so the tests
// assert the request and not merely the parsed result — a client that
// never authenticated would still satisfy a parse-only assertion.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
}

type ascRecorder struct {
	mu   sync.Mutex
	reqs []recordedRequest
}

func (r *ascRecorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, recordedRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Query:  req.URL.RawQuery,
		Auth:   req.Header.Get("Authorization"),
	})
}

func (r *ascRecorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.reqs))
	copy(out, r.reqs)
	return out
}

// newListAppsServer serves the given JSON bodies in order, one per
// request, and records every request it receives.
func newListAppsServer(t *testing.T, status int, bodies ...string) (*httptest.Server, *ascRecorder) {
	t.Helper()
	rec := &ascRecorder{}
	var n int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		mu.Lock()
		i := n
		n++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK || i >= len(bodies) {
			return
		}
		_, _ = w.Write([]byte(bodies[i]))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func newListAppsClient(t *testing.T, baseURL string) *apple.Client {
	t.Helper()
	p8 := genP8(t)
	cli, err := apple.New(apple.Config{
		BaseURL: baseURL,
		CredsFetcher: func(context.Context) (apple.Credentials, error) {
			return apple.Credentials{P8: p8, IssuerID: "iss", KeyID: "kid"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return cli
}

// assertReadOnly fails if the client sent anything other than a GET —
// discovery must never mutate state at Apple.
func assertReadOnly(t *testing.T, reqs []recordedRequest) {
	t.Helper()
	if len(reqs) == 0 {
		t.Fatal("no requests recorded; want at least one GET")
	}
	for i, r := range reqs {
		if r.Method != http.MethodGet {
			t.Errorf("request %d method = %s, want GET (ListApps must not mutate)", i, r.Method)
		}
		if !strings.HasPrefix(r.Auth, "Bearer ") {
			t.Errorf("request %d Authorization = %q, want Bearer <jwt>", i, r.Auth)
		}
	}
}

const twoAppsBody = `{
  "data": [
    {"type":"apps","id":"1111","attributes":{"bundleId":"com.merchant.one","name":"One"}},
    {"type":"apps","id":"2222","attributes":{"bundleId":"com.merchant.two","name":"Two"}}
  ],
  "links": {"self": "https://example.invalid/v1/apps"}
}`

func TestClient_ListApps_MultipleApps(t *testing.T) {
	srv, rec := newListAppsServer(t, http.StatusOK, twoAppsBody)
	cli := newListAppsClient(t, srv.URL)

	apps, err := cli.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}

	reqs := rec.all()
	assertReadOnly(t, reqs)
	if reqs[0].Path != "/v1/apps" {
		t.Errorf("path = %s, want /v1/apps", reqs[0].Path)
	}
	if !strings.Contains(reqs[0].Query, "limit=") {
		t.Errorf("query = %q, want a limit param", reqs[0].Query)
	}

	if len(apps) != 2 {
		t.Fatalf("len(apps) = %d, want 2", len(apps))
	}
	if apps[0].ID != "1111" || apps[0].BundleID != "com.merchant.one" || apps[0].Name != "One" {
		t.Errorf("apps[0] = %+v, want id 1111 / com.merchant.one / One", apps[0])
	}
	if apps[1].ID != "2222" || apps[1].BundleID != "com.merchant.two" {
		t.Errorf("apps[1] = %+v, want id 2222 / com.merchant.two", apps[1])
	}
}

func TestClient_ListApps_SingleApp(t *testing.T) {
	body := `{"data":[{"type":"apps","id":"9999","attributes":{"bundleId":"com.merchant.only","name":"Only"}}]}`
	srv, rec := newListAppsServer(t, http.StatusOK, body)
	cli := newListAppsClient(t, srv.URL)

	apps, err := cli.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	assertReadOnly(t, rec.all())
	if len(apps) != 1 {
		t.Fatalf("len(apps) = %d, want 1", len(apps))
	}
	if apps[0].ID != "9999" {
		t.Errorf("apps[0].ID = %q, want 9999", apps[0].ID)
	}
}

// An account with no apps is a fact, not an error — the caller must be
// able to tell it apart from a rejected key.
func TestClient_ListApps_EmptyData(t *testing.T) {
	srv, rec := newListAppsServer(t, http.StatusOK, `{"data":[]}`)
	cli := newListAppsClient(t, srv.URL)

	apps, err := cli.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps on empty data = %v, want nil error", err)
	}
	assertReadOnly(t, rec.all())
	if len(apps) != 0 {
		t.Errorf("len(apps) = %d, want 0", len(apps))
	}
}

func TestClient_ListApps_Unauthorized(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv, rec := newListAppsServer(t, status)
			cli := newListAppsClient(t, srv.URL)

			apps, err := cli.ListApps(context.Background())
			if !errors.Is(err, apple.ErrUnauthorized) {
				t.Fatalf("err = %v, want wraps ErrUnauthorized", err)
			}
			if apps != nil {
				t.Errorf("apps = %+v, want nil on error", apps)
			}
			assertReadOnly(t, rec.all())
		})
	}
}

// A 500 is not the same fact as a revoked key, and neither is an empty
// list: the error must surface and must not masquerade as either.
func TestClient_ListApps_ServerError(t *testing.T) {
	srv, rec := newListAppsServer(t, http.StatusInternalServerError)
	cli := newListAppsClient(t, srv.URL)

	apps, err := cli.ListApps(context.Background())
	if err == nil {
		t.Fatal("ListApps on 500 = nil error, want error")
	}
	if errors.Is(err, apple.ErrUnauthorized) {
		t.Errorf("err = %v, want a plain error, not ErrUnauthorized", err)
	}
	if apps != nil {
		t.Errorf("apps = %+v, want nil on error", apps)
	}
	assertReadOnly(t, rec.all())
}

// A 404 must NOT be swallowed here. call() maps 404 to success for the
// idempotent write paths; a read that did the same would report "no
// apps" for an endpoint it never actually read.
func TestClient_ListApps_NotFoundIsAnError(t *testing.T) {
	srv, _ := newListAppsServer(t, http.StatusNotFound)
	cli := newListAppsClient(t, srv.URL)

	if _, err := cli.ListApps(context.Background()); err == nil {
		t.Error("ListApps on 404 = nil error, want error (404 is not an empty account)")
	}
}

func TestClient_ListApps_MalformedJSON(t *testing.T) {
	srv, _ := newListAppsServer(t, http.StatusOK, `{"data": [`)
	cli := newListAppsClient(t, srv.URL)

	if _, err := cli.ListApps(context.Background()); err == nil {
		t.Error("ListApps on malformed body = nil error, want error")
	}
}

func TestClient_ListApps_FollowsNextLink(t *testing.T) {
	rec := &ascRecorder{}
	var mu sync.Mutex
	var n int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		mu.Lock()
		i := n
		n++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		if i == 0 {
			fmt.Fprintf(w, `{"data":[{"id":"p1","attributes":{"bundleId":"com.a","name":"A"}}],"links":{"next":"%s/v1/apps?cursor=abc"}}`, srv.URL)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"p2","attributes":{"bundleId":"com.b","name":"B"}}]}`))
	}))
	defer srv.Close()

	cli := newListAppsClient(t, srv.URL)
	apps, err := cli.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}

	reqs := rec.all()
	assertReadOnly(t, reqs)
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2 (first page + links.next)", len(reqs))
	}
	if !strings.Contains(reqs[1].Query, "cursor=abc") {
		t.Errorf("second request query = %q, want cursor=abc from links.next", reqs[1].Query)
	}
	if len(apps) != 2 || apps[0].ID != "p1" || apps[1].ID != "p2" {
		t.Errorf("apps = %+v, want both pages concatenated", apps)
	}
}

// links.next pointing off-host must be refused: the Bearer token rides
// on every hop.
func TestClient_ListApps_RefusesOffHostNextLink(t *testing.T) {
	body := `{"data":[],"links":{"next":"https://attacker.invalid/v1/apps"}}`
	srv, _ := newListAppsServer(t, http.StatusOK, body)
	cli := newListAppsClient(t, srv.URL)

	if _, err := cli.ListApps(context.Background()); err == nil {
		t.Error("ListApps with off-host links.next = nil error, want refusal")
	}
}

// A links.next that never terminates must hit the documented cap rather
// than page forever.
func TestClient_ListApps_PageCap(t *testing.T) {
	var srv *httptest.Server
	rec := &ascRecorder{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"data":[{"id":"x","attributes":{}}],"links":{"next":"%s/v1/apps?cursor=loop"}}`, srv.URL)
	}))
	defer srv.Close()

	cli := newListAppsClient(t, srv.URL)
	apps, err := cli.ListApps(context.Background())
	if !errors.Is(err, apple.ErrTooManyAppPages) {
		t.Fatalf("err = %v, want wraps ErrTooManyAppPages", err)
	}
	if apps != nil {
		t.Errorf("apps = %+v, want nil rather than a truncated list", apps)
	}
	assertReadOnly(t, rec.all())
}

func TestClient_ListApps_CredsFailureSurfaces(t *testing.T) {
	sentinel := errors.New("creds read failed")
	cli, err := apple.New(apple.Config{
		BaseURL: "https://example.invalid",
		CredsFetcher: func(context.Context) (apple.Credentials, error) {
			return apple.Credentials{}, sentinel
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := cli.ListApps(context.Background()); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wraps the creds sentinel", err)
	}
}

func TestFakeClient_ListApps(t *testing.T) {
	f := apple.NewFakeClient()
	f.Apps = []apple.App{{ID: "a1", BundleID: "com.a"}, {ID: "a2", BundleID: "com.b"}}

	apps, err := f.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 2 || apps[0].ID != "a1" {
		t.Errorf("apps = %+v, want the two configured apps", apps)
	}
	if f.ListAppsCallCount != 1 {
		t.Errorf("ListAppsCallCount = %d, want 1", f.ListAppsCallCount)
	}

	// Returned slice is a copy — mutating it must not touch the fake.
	apps[0].ID = "mutated"
	if f.Apps[0].ID != "a1" {
		t.Errorf("f.Apps[0].ID = %q, want a1 (ListApps must return a copy)", f.Apps[0].ID)
	}

	f.ListAppsErr = errors.New("boom")
	if _, err := f.ListApps(context.Background()); err == nil {
		t.Error("ListApps with ListAppsErr = nil error, want the configured error")
	}
}

// FakeClient must satisfy ClientAPI including the new method.
var _ apple.ClientAPI = (*apple.FakeClient)(nil)
var _ apple.ClientAPI = (*apple.Client)(nil)
