package gipadmin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

// No HTTP-stubbing helper exists yet in this package: claims_test.go only
// exercises pure functions (existingTenantIDs) and the empty-arg validation
// path on a zero-value &AdminClient{}, which returns before any HTTP call is
// made. doAdmin hardcodes the identitytoolkit.googleapis.com host, so there
// is no base-URL field to inject through. newTestClient instead swaps the
// unexported httpClient's Transport to redirect requests to an httptest
// server, and stubs tokenSource so no ADC lookup is attempted. This is the
// natural in-package seam given the existing (lack of) test infrastructure.

// stubTokenSource satisfies oauth2.TokenSource without hitting ADC.
type stubTokenSource struct{}

func (stubTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: "test-token"}, nil
}

// redirectTransport rewrites the scheme/host of every outgoing request to
// point at an httptest server, leaving path, method, and body untouched.
type redirectTransport struct {
	target *url.URL
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestClient builds an AdminClient wired to an httptest server running
// handler, with a stubbed token source. The server is closed automatically
// via t.Cleanup.
func newTestClient(t *testing.T, handler http.HandlerFunc) *AdminClient {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}

	return &AdminClient{
		cfg: Config{
			ProjectID: "test-project",
			TenantID:  "MP-Internal-e986p",
			WebAPIKey: "test-web-api-key",
		},
		httpClient:  &http.Client{Transport: &redirectTransport{target: target}},
		tokenSource: stubTokenSource{},
	}
}
