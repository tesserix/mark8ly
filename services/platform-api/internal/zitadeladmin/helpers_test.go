package zitadeladmin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient builds a Client wired to an httptest server running
// handler. The server is closed automatically via t.Cleanup.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := New(Config{
		BaseURL: srv.URL,
		Token:   "test-pat-thirtytwo-bytes-for-testing-only",
		OrgID:   "org-tesserix",
	}, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}
