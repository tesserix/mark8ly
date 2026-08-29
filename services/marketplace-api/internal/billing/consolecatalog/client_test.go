package consolecatalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
)

// fakeConsole serves a token endpoint and a catalog endpoint, counting both
// so the tests can assert what was and was not called.
type fakeConsole struct {
	tokenCalls   int32
	catalogCalls int32
	etag         string
	body         consolecatalog.Catalog
	catalogState func(w http.ResponseWriter, r *http.Request) bool // return true if handled
}

func (f *fakeConsole) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v2/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.tokenCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok", "token_type": "Bearer", "expires_in": 3600,
		})
	})
	mux.HandleFunc("/api/v1/plan-catalog", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.catalogCalls, 1)
		if f.catalogState != nil && f.catalogState(w, r) {
			return
		}
		if r.Header.Get("If-None-Match") == f.etag && f.etag != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", f.etag)
		_ = json.NewEncoder(w).Encode(f.body)
	})
	return mux
}

func newFake(t *testing.T) (*fakeConsole, *consolecatalog.Client) {
	t.Helper()
	f := &fakeConsole{
		etag: `"rev-1"`,
		body: consolecatalog.Catalog{Mode: "test", RevisionID: "rev-1", Prices: []consolecatalog.Price{
			{LookupKey: "k", Currency: "usd", UnitAmountMinor: 1900, Tier: "developed"},
		}},
	}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c := consolecatalog.NewClient(consolecatalog.Config{
		CatalogURL: srv.URL + "/api/v1/plan-catalog",
		TokenURL:   srv.URL + "/oauth/v2/token",
		ClientID:   "id", ClientSecret: "secret", Scope: "openid",
		Mode: "test",
	}, nil)
	return f, c
}

func TestClient_FetchesTheCatalogWithABearerToken(t *testing.T) {
	f, c := newFake(t)
	got, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.Equal(t, "rev-1", got.RevisionID)
	require.Len(t, got.Prices, 1)
	require.Equal(t, int32(1), atomic.LoadInt32(&f.tokenCalls))
}

// A token is good for an hour; minting one per read would turn a cached
// catalog into two network round trips on the payment path's warm-up.
func TestClient_ReusesTheTokenAcrossFetches(t *testing.T) {
	f, c := newFake(t)
	for i := 0; i < 3; i++ {
		_, err := c.Fetch(context.Background())
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&f.tokenCalls),
		"the token must be reused until it nears expiry, not minted per fetch")
	require.Equal(t, int32(3), atomic.LoadInt32(&f.catalogCalls))
}

// The endpoint serves Cache-Control: no-cache with an ETag, so a caller that
// already holds the current revision should pay only the round trip. A 304
// carries no body — treating it as an empty catalog would be catastrophic.
func TestClient_A304ReturnsTheLastKnownCatalogNotAnEmptyOne(t *testing.T) {
	_, c := newFake(t)
	first, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, first.Prices, 1)

	second, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, second.Prices, 1, "a 304 must yield the retained catalog, never an empty one")
	require.Equal(t, "rev-1", second.RevisionID)
}

func TestClient_UpstreamFailureIsAnError(t *testing.T) {
	f, c := newFake(t)
	f.catalogState = func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusServiceUnavailable)
		return true
	}
	_, err := c.Fetch(context.Background())
	require.Error(t, err, "the cache layer decides how to degrade; the client must not hide a failure")
}

// A 404 means the mode has never been published. The console returns it
// deliberately rather than an empty 200, because caching "the catalog is
// empty" is worse than caching nothing: it would let this service price
// nothing at all, silently.
func TestClient_UnpublishedModeIsADistinctError(t *testing.T) {
	f, c := newFake(t)
	f.catalogState = func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	_, err := c.Fetch(context.Background())
	require.ErrorIs(t, err, consolecatalog.ErrNotPublished)
}

func TestClient_RespectsContextCancellation(t *testing.T) {
	_, c := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Fetch(ctx)
	require.Error(t, err)
}

var _ = time.Second
