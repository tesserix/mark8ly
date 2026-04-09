package stores_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, secret string) (*stores.HTTPClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	return stores.NewHTTPClient(srv.URL, secret, srv.Client()), srv
}

func TestHTTPClient_GetStore_HappyPath_200(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/stores/s1" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"s1","tenant_id":"t1","slug":"acme","name":"Acme","country_code":"US","currency_code":"USD","timezone":"UTC","status":"active"}}`))
	}, "")
	defer srv.Close()
	got, err := c.GetStore(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil || got.ID != "s1" || got.TenantID != "t1" || got.Slug != "acme" || got.CurrencyCode != "USD" {
		t.Fatalf("unexpected store: %+v", got)
	}
}

func TestHTTPClient_GetStore_404_ReturnsNilNil(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, "")
	defer srv.Close()
	got, err := c.GetStore(context.Background(), "t1", "s1")
	if err != nil || got != nil {
		t.Fatalf("expected (nil,nil); got (%v,%v)", got, err)
	}
}

func TestHTTPClient_GetStore_WrongTenant_ReturnsNilNil(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"s1","tenant_id":"other","slug":"acme","name":"A","country_code":"US","currency_code":"USD","timezone":"UTC","status":"active"}}`))
	}, "")
	defer srv.Close()
	got, err := c.GetStore(context.Background(), "t1", "s1")
	if err != nil || got != nil {
		t.Fatalf("expected (nil,nil); got (%v,%v)", got, err)
	}
}

func TestHTTPClient_GetStore_5xx_Returns_ErrPlatformUnavailable(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}, "")
	defer srv.Close()
	_, err := c.GetStore(context.Background(), "t1", "s1")
	if err == nil || !errors.Is(err, stores.ErrPlatformUnavailable) {
		t.Fatalf("expected ErrPlatformUnavailable; got %v", err)
	}
}

func TestHTTPClient_GetStore_NetworkError_Returns_ErrPlatformUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	c := stores.NewHTTPClient(srv.URL, "", srv.Client())
	srv.Close()
	_, err := c.GetStore(context.Background(), "t1", "s1")
	if err == nil || !errors.Is(err, stores.ErrPlatformUnavailable) {
		t.Fatalf("expected ErrPlatformUnavailable; got %v", err)
	}
}

func TestHTTPClient_GetStore_SecretHeader_Sent(t *testing.T) {
	var gotSecret string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Internal-Auth")
		_, _ = w.Write([]byte(`{"data":{"id":"s1","tenant_id":"t1","slug":"a","name":"a","country_code":"US","currency_code":"USD","timezone":"UTC","status":"active"}}`))
	}, "sek")
	defer srv.Close()
	if _, err := c.GetStore(context.Background(), "t1", "s1"); err != nil {
		t.Fatal(err)
	}
	if gotSecret != "sek" {
		t.Errorf("expected sek; got %q", gotSecret)
	}
}

func TestHTTPClient_GetStore_NoSecret_NoHeader(t *testing.T) {
	var gotSecret string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Internal-Auth")
		_, _ = w.Write([]byte(`{"data":{"id":"s1","tenant_id":"t1","slug":"a","name":"a","country_code":"US","currency_code":"USD","timezone":"UTC","status":"active"}}`))
	}, "")
	defer srv.Close()
	if _, err := c.GetStore(context.Background(), "t1", "s1"); err != nil {
		t.Fatal(err)
	}
	if gotSecret != "" {
		t.Errorf("expected absent header; got %q", gotSecret)
	}
}
