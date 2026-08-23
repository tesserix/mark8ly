package main

import (
	"net/http"
	"testing"
	"time"
)

// A server with no ReadHeaderTimeout lets a client hold a connection open
// indefinitely mid-header (Slowloris); IdleTimeout bounds kept-alive
// connections that go silent.
func TestNewHTTPServer_SetsTimeouts(t *testing.T) {
	srv := newHTTPServer(8080, http.NotFoundHandler())

	if srv.Addr != ":8080" {
		t.Fatalf("Addr = %q, want \":8080\"", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Fatalf("IdleTimeout = %v, want 120s", srv.IdleTimeout)
	}
}
