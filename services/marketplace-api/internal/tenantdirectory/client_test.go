package tenantdirectory_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

func TestListSendsAuthAndParsesEnvelope(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Internal-Auth")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"t1","name":"Acme","owner_email":"a@example.com","status":"active","created_at":"2026-08-22T10:00:00Z"}],"pagination":{"page":1,"limit":50,"total":1}}`))
	}))
	defer srv.Close()

	c := tenantdirectory.NewClient(srv.URL, "s3cret", srv.Client())
	got, err := c.List(context.Background(), tenantdirectory.ListParams{Q: "acme", Limit: 50})
	require.NoError(t, err)

	require.Equal(t, "s3cret", gotAuth)
	require.Contains(t, gotQuery, "q=acme")
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Tenants, 1)
	require.Equal(t, "Acme", got.Tenants[0].Name)
}

// An empty upstream page must arrive as an allocated slice, so the handler
// cannot marshal nil.
func TestListEmptyIsAllocated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"page":1,"limit":50,"total":0}}`))
	}))
	defer srv.Close()

	got, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		List(context.Background(), tenantdirectory.ListParams{})
	require.NoError(t, err)
	require.NotNil(t, got.Tenants)
	require.Empty(t, got.Tenants)
}

func TestGetParsesStoreRollup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"t1","name":"Acme","owner_email":"a@example.com","status":"active","created_at":"2026-08-22T10:00:00Z","stores":[{"id":"s1","slug":"acme","name":"Acme Store","status":"active"}]}}`))
	}))
	defer srv.Close()

	got, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		Get(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, got.Stores, 1)
	require.Equal(t, "acme", got.Stores[0].Slug)
}

func TestGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"no such tenant"}`))
	}))
	defer srv.Close()

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).Get(context.Background(), "missing")
	require.ErrorIs(t, err, tenantdirectory.ErrNotFound)
}

// A 5xx upstream must NOT look like an empty result — that is the failure the
// handler turns into 503 rather than a misleading 200 with no tenants.
func TestUpstream5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		List(context.Background(), tenantdirectory.ListParams{})
	require.ErrorIs(t, err, tenantdirectory.ErrUnavailable)
}

func TestTransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed: connection refused

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		List(context.Background(), tenantdirectory.ListParams{})
	require.ErrorIs(t, err, tenantdirectory.ErrUnavailable)
}

// The email is URL-encoded via url.Values, not string-concatenated, so a
// "+" in a legitimate address survives the round trip intact.
func TestFindByOwnerEmailSendsAuthAndEncodesQuery(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Internal-Auth")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("email")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"t1","name":"Acme","owner_email":"founder+tag@acme.example","status":"active","created_at":"2026-08-22T10:00:00Z"}}`))
	}))
	defer srv.Close()

	c := tenantdirectory.NewClient(srv.URL, "s3cret", srv.Client())
	got, err := c.FindByOwnerEmail(context.Background(), "founder+tag@acme.example")
	require.NoError(t, err)

	require.Equal(t, "s3cret", gotAuth)
	require.Equal(t, "/internal/tenants/by-owner-email", gotPath)
	require.Equal(t, "founder+tag@acme.example", gotQuery)
	require.Equal(t, "Acme", got.Name)
}

func TestFindByOwnerEmailParsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"t1","name":"Acme","owner_email":"a@example.com","status":"active","created_at":"2026-08-22T10:00:00Z"}}`))
	}))
	defer srv.Close()

	got, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		FindByOwnerEmail(context.Background(), "a@example.com")
	require.NoError(t, err)
	require.Equal(t, "t1", got.ID)
	require.Equal(t, "a@example.com", got.OwnerEmail)
	require.Equal(t, 2026, got.CreatedAt.Year())
}

func TestFindByOwnerEmailNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"no such tenant"}`))
	}))
	defer srv.Close()

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		FindByOwnerEmail(context.Background(), "missing@example.com")
	require.ErrorIs(t, err, tenantdirectory.ErrNotFound)
}

func TestFindByOwnerEmailUpstream5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		FindByOwnerEmail(context.Background(), "a@example.com")
	require.ErrorIs(t, err, tenantdirectory.ErrUnavailable)
}

func TestFindByOwnerEmailTransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed: connection refused

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		FindByOwnerEmail(context.Background(), "a@example.com")
	require.ErrorIs(t, err, tenantdirectory.ErrUnavailable)
}

// TestListSendsIDs asserts ListParams.IDs is joined and sent as a single
// comma-separated `ids` query parameter, so #285's trials endpoint can
// resolve a page of tenants in one batch call.
func TestListSendsIDs(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"page":1,"limit":50,"total":0}}`))
	}))
	defer srv.Close()

	c := tenantdirectory.NewClient(srv.URL, "s", srv.Client())
	_, err := c.List(context.Background(), tenantdirectory.ListParams{IDs: []string{"a", "b"}})
	require.NoError(t, err)
	require.Contains(t, gotQuery, "ids=a%2Cb")
}
