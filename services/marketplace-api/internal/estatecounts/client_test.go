package estatecounts_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/estatecounts"
)

func TestGet_SendsAuthHeaderAndPath(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("X-Internal-Auth")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"tenants_active":4,"stores_active":5}}`))
	}))
	defer srv.Close()

	c := estatecounts.NewClient(srv.URL, "secret-token", nil)
	_, err := c.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if gotPath != "/internal/estate/counts" {
		t.Errorf("path = %q, want %q", gotPath, "/internal/estate/counts")
	}
	if gotAuth != "secret-token" {
		t.Errorf("X-Internal-Auth = %q, want %q", gotAuth, "secret-token")
	}
}

func TestGet_ParsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"tenants_active":4,"stores_active":5}}`))
	}))
	defer srv.Close()

	c := estatecounts.NewClient(srv.URL, "secret", nil)
	got, err := c.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	want := &estatecounts.Counts{TenantsActive: 4, StoresActive: 5}
	if *got != *want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestGet_UpstreamServerError_ReturnsErrUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := estatecounts.NewClient(srv.URL, "secret", nil)
	got, err := c.Get(t.Context())
	if !errors.Is(err, estatecounts.ErrUnavailable) {
		t.Fatalf("Get() error = %v, want ErrUnavailable", err)
	}
	if got != nil {
		t.Errorf("Get() result = %+v, want nil on error", got)
	}
}

func TestGet_TransportFailure_ReturnsErrUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := srv.URL
	srv.Close() // closed server: connection refused

	c := estatecounts.NewClient(baseURL, "secret", nil)
	got, err := c.Get(t.Context())
	if !errors.Is(err, estatecounts.ErrUnavailable) {
		t.Fatalf("Get() error = %v, want ErrUnavailable", err)
	}
	if got != nil {
		t.Errorf("Get() result = %+v, want nil on error", got)
	}
}

func TestGet_EmptyEstate_YieldsZerosNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	c := estatecounts.NewClient(srv.URL, "secret", nil)
	got, err := c.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	want := &estatecounts.Counts{TenantsActive: 0, StoresActive: 0}
	if *got != *want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}
