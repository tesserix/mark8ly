package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// captureHandler collects records so a test can assert on what was logged.
// The log line IS the deliverable of the resolver, so asserting
// on it is asserting on the feature, not on an implementation detail.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) all() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

// find returns the first record whose message contains sub.
func (h *captureHandler) find(sub string) (slog.Record, bool) {
	for _, r := range h.all() {
		if strings.Contains(r.Message, sub) {
			return r, true
		}
	}
	return slog.Record{}, false
}

func attrsOf(r slog.Record) map[string]any {
	out := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.Any()
		return true
	})
	return out
}

func captureLogger() (*slog.Logger, *captureHandler) {
	h := &captureHandler{}
	return slog.New(h), h
}

// fakeFetcher is a consolecatalog.Fetcher whose answer the test controls.
type fakeFetcher struct {
	mu  sync.Mutex
	cat consolecatalog.Catalog
	err error
}

func (f *fakeFetcher) Fetch(context.Context) (consolecatalog.Catalog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cat, f.err
}

func (f *fakeFetcher) set(cat consolecatalog.Catalog, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cat, f.err = cat, err
}

func testCatalog() consolecatalog.Catalog {
	return consolecatalog.Catalog{
		Mode:       "test",
		RevisionID: consolecatalog.SharedRevisionID,
		Prices: []consolecatalog.Price{
			{LookupKey: "starter_monthly", Currency: "usd", UnitAmountMinor: 1900},
		},
	}
}

func configuredCfg() *config.Config {
	return &config.Config{
		// 127.0.0.1:1 is refused immediately, so the goroutine this starts
		// cannot sit on a real network call for the length of the test.
		ConsoleCatalogURL:          "http://127.0.0.1:1/api/v1/billing/catalog",
		ConsoleCatalogTokenURL:     "http://127.0.0.1:1/oauth/v2/token",
		ConsoleCatalogClientID:     "id",
		ConsoleCatalogClientSecret: "secret",
		ConsoleCatalogMode:         "test",
		ConsoleCatalogInterval:     time.Hour,
		ConsoleCatalogCacheTTL:     time.Hour,
	}
}

// TestAdminCatalogResolveUnconfigured pins the contract catalog_parity.go
// states and this file repeats: absent credentials mean no goroutine, no
// reads, and one clear log line — behaviour identical to before the resolver
// existed. Enabling it must stay a pure config change.
func TestAdminCatalogResolveUnconfigured(t *testing.T) {
	log, h := captureLogger()

	started := startAdminCatalogResolve(mode.Admin, &config.Config{}, log)

	require.False(t, started, "no credentials must mean no goroutine")
	_, ok := h.find("cache read-path exercise disabled")
	require.True(t, ok, "unconfigured must say so exactly once, clearly: %v", h.all())
}

// TestAdminCatalogResolveModeMatrix states the matrix implemented:
//
//	admin      -> started (every runtime reader of the plan catalog is here)
//	storefront -> not started
//	both       -> started (local dev runs both this and the parity monitor)
//
// The storefront case additionally asserts SILENCE rather than a "disabled"
// line: on the storefront this feature is not disabled, it is not applicable
// — that pod has no dependency path to internal/billing/pricing at all — and
// a line claiming otherwise would send someone hunting for credentials that
// pod is right not to use.
func TestAdminCatalogResolveModeMatrix(t *testing.T) {
	for _, tc := range []struct {
		m    mode.Mode
		want bool
	}{
		{mode.Admin, true},
		{mode.Storefront, false},
		{mode.Both, true},
	} {
		t.Run(string(tc.m), func(t *testing.T) {
			log, h := captureLogger()

			got := startAdminCatalogResolve(tc.m, configuredCfg(), log)

			require.Equal(t, tc.want, got)
			if tc.want {
				_, ok := h.find("cache read-path exercise enabled")
				require.True(t, ok, "an enabled resolver must announce itself: %v", h.all())
			} else {
				require.Empty(t, h.all(), "a mode that does not run the resolver must log nothing")
			}
		})
	}
}

// TestResolveOnceLogsFresh is the happy path: a working console read logs at
// info with the source, revision and price count that make the line evidence
// rather than noise.
func TestResolveOnceLogsFresh(t *testing.T) {
	log, h := captureLogger()
	f := &fakeFetcher{cat: testCatalog()}
	cache := consolecatalog.NewCache(f, time.Hour, "test", log)

	res := resolveOnceAndLog(cache, log)

	require.Equal(t, consolecatalog.SourceFresh, res.Source)
	require.False(t, res.Stale)

	rec, ok := h.find(adminResolveLogMsg)
	require.True(t, ok, "the resolve must log: %v", h.all())
	require.Equal(t, slog.LevelInfo, rec.Level)

	a := attrsOf(rec)
	require.Equal(t, "fresh", a["source"])
	require.Equal(t, false, a["stale"])
	require.Equal(t, consolecatalog.SharedRevisionID, a["revision_id"])
	require.Equal(t, int64(1), a["prices"])
	require.Equal(t, false, a["revision_unexpected"])
	require.NotContains(t, a, "error")
}

// TestResolveOnceLogsColdStartFallback covers the case the cutover most needs
// to survive: a freshly rolled pod that has never reached the console. It must
// answer from the compiled catalog, log it, and not panic — a pod that cannot
// price a plan change is an outage, a pod pricing from the compiled snapshot
// is not.
func TestResolveOnceLogsColdStartFallback(t *testing.T) {
	log, h := captureLogger()
	f := &fakeFetcher{err: errors.New("dial tcp: connection refused")}
	cache := consolecatalog.NewCache(f, time.Hour, "test", log)

	res := resolveOnceAndLog(cache, log)

	require.Equal(t, consolecatalog.SourceCompiled, res.Source)
	require.True(t, res.Stale)
	require.NotEmpty(t, res.Catalog.Prices, "the compiled fallback must not be empty")

	rec, ok := h.find(adminResolveLogMsg)
	require.True(t, ok, "a degraded resolve must still log: %v", h.all())
	require.Equal(t, slog.LevelWarn, rec.Level)

	a := attrsOf(rec)
	require.Equal(t, "compiled", a["source"])
	require.Equal(t, true, a["stale"])
	require.Contains(t, a, "error")
}

// TestResolveOnceLogsStaleAfterFailedRefresh is the fail-open property seen
// from the caller's side: once a good catalog has been read, a later console
// failure must serve that catalog rather than evict it, and must say so.
func TestResolveOnceLogsStaleAfterFailedRefresh(t *testing.T) {
	log, _ := captureLogger()
	f := &fakeFetcher{cat: testCatalog()}
	// A one-nanosecond TTL expires between the two resolves without the test
	// having to sleep out a real window.
	cache := consolecatalog.NewCache(f, time.Nanosecond, "test", log)

	require.Equal(t, consolecatalog.SourceFresh, resolveOnceAndLog(cache, log).Source)

	f.set(consolecatalog.Catalog{}, errors.New("console 503"))
	log2, h2 := captureLogger()
	res := resolveOnceAndLog(cache, log2)

	require.Equal(t, consolecatalog.SourceStale, res.Source)
	require.True(t, res.Stale)
	require.Equal(t, consolecatalog.SharedRevisionID, res.Catalog.RevisionID,
		"a failed refresh must not evict the last-known catalog")

	rec, ok := h2.find(adminResolveLogMsg)
	require.True(t, ok, "a stale resolve must still log: %v", h2.all())
	require.Equal(t, slog.LevelWarn, rec.Level)
	require.Equal(t, "stale", attrsOf(rec)["source"])
}

// TestResolveOnceDoesNotBlockOnAHangingConsole guards the reason resolveTimeout
// exists. A console that accepts the connection and never answers must not pin
// the ticker goroutine: the context deadline has to reach the fetcher.
func TestResolveOnceDoesNotBlockOnAHangingConsole(t *testing.T) {
	log, h := captureLogger()
	cache := consolecatalog.NewCache(hangingFetcher{}, time.Hour, "test", log)

	done := make(chan consolecatalog.Resolution, 1)
	go func() { done <- resolveWithin(100*time.Millisecond, cache, log) }()

	select {
	case res := <-done:
		require.Equal(t, consolecatalog.SourceCompiled, res.Source)
		_, ok := h.find(adminResolveLogMsg)
		require.True(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("resolveOnceAndLog blocked on a hanging console")
	}
}

// hangingFetcher blocks until the caller's context is done, which is what a
// console that accepts a connection and never replies looks like from here.
type hangingFetcher struct{}

func (hangingFetcher) Fetch(ctx context.Context) (consolecatalog.Catalog, error) {
	<-ctx.Done()
	return consolecatalog.Catalog{}, ctx.Err()
}
