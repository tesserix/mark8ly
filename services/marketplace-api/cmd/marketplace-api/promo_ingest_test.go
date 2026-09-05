package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/consolepromo"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// promoConfiguredCfg is configuredCfg plus the one new setting the promo
// ingest needs. 127.0.0.1:1 is refused immediately, so the goroutine this
// starts cannot sit on a real network call for the length of the test.
func promoConfiguredCfg() *config.Config {
	cfg := configuredCfg()
	cfg.ConsolePromoCatalogURL = "http://127.0.0.1:1/api/v1/billing/promo-catalog"
	return cfg
}

// TestPromoIngestUnconfigured pins the contract the plan required: with no
// URL set the ingest is skipped, no goroutine runs, promo_codes is untouched,
// and the service starts exactly as it did before this existed.
func TestPromoIngestUnconfigured(t *testing.T) {
	log, h := captureLogger()

	started := startPromoCatalogIngest(mode.Admin, &config.Config{}, &gorm.DB{}, log)

	require.False(t, started, "no CONSOLE_PROMO_CATALOG_URL must mean no goroutine")
	_, ok := h.find("promo catalog ingest disabled")
	require.True(t, ok, "unconfigured must say so exactly once, clearly: %v", h.all())
}

// TestPromoIngestCredentialsAloneAreNotEnough — the credentials are shared
// with the plan catalog, so a pod configured for that one must NOT start
// ingesting promos as a side effect. The promo URL is the switch.
func TestPromoIngestCredentialsAloneAreNotEnough(t *testing.T) {
	log, _ := captureLogger()

	cfg := configuredCfg() // plan-catalog credentials, no promo URL
	require.False(t, startPromoCatalogIngest(mode.Admin, cfg, &gorm.DB{}, log))
}

// TestPromoIngestNeedsADatabase — this ingest writes rows, so without a
// connection there is nothing it can usefully do. Skipping is silent because
// a nil db means the process is not in a state where the ingest is even
// applicable.
func TestPromoIngestNeedsADatabase(t *testing.T) {
	log, h := captureLogger()

	require.False(t, startPromoCatalogIngest(mode.Admin, promoConfiguredCfg(), nil, log))
	require.Empty(t, h.all())
}

// TestPromoIngestModeMatrix states the matrix implemented:
//
//	admin      -> started (the promo surfaces and #620's redemption are here)
//	storefront -> not started
//	both       -> started (local dev runs everything)
//
// The storefront case asserts SILENCE rather than a "disabled" line: there
// the ingest is not disabled, it is not applicable, and a line claiming
// otherwise would send someone hunting for credentials that pod is right not
// to use.
func TestPromoIngestModeMatrix(t *testing.T) {
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

			got := startPromoCatalogIngest(tc.m, promoConfiguredCfg(), &gorm.DB{}, log)

			require.Equal(t, tc.want, got)
			if tc.want {
				_, ok := h.find("promo catalog ingest enabled")
				require.True(t, ok, "an enabled ingest must announce itself: %v", h.all())
			} else {
				require.Empty(t, h.all(), "a mode that does not ingest must log nothing")
			}
		})
	}
}

// stubPromoFetcher is a consolepromo.Fetcher the test controls.
type stubPromoFetcher struct {
	cat consolepromo.Catalog
	err error
}

func (f *stubPromoFetcher) Fetch(context.Context) (consolepromo.Catalog, error) {
	return f.cat, f.err
}

// stubPromoStore accepts everything and records nothing; these tests are
// about what gets LOGGED, which is the whole product of this file.
type stubPromoStore struct{}

func (stubPromoStore) UpsertCodes(context.Context, []promo.PromoCode) error { return nil }
func (stubPromoStore) ExpireCodesNotIn(context.Context, []string, time.Time) (int, error) {
	return 2, nil
}

func TestSyncOnceLogsTheOutcome(t *testing.T) {
	log, h := captureLogger()
	f := &stubPromoFetcher{cat: consolepromo.Catalog{
		RevisionID: "rev-9",
		Codes: []consolepromo.Code{
			{Code: "EXTRA14DAYS", TrialExtensionDays: intPtr(14)},
			{Code: "BAD"}, // too short and no benefit — skipped, not fatal
		},
	}}

	res := syncOnceAndLog(consolepromo.NewSyncer(f, stubPromoStore{}, log), log)

	require.Equal(t, 1, res.Ingested)
	require.Equal(t, 1, res.Skipped)
	require.Equal(t, 2, res.Expired)

	rec, ok := h.find(promoIngestLogMsg)
	require.True(t, ok, "a completed ingest must log: %v", h.all())
	attrs := attrsOf(rec)
	require.Equal(t, "rev-9", attrs["revision_id"])
	require.Equal(t, int64(1), attrs["ingested"])
	require.Equal(t, int64(1), attrs["skipped"])
}

// TestSyncOnceLogsAFailureAndSaysNothingChanged — the failure line has to
// state the consequence, because "ingest failed" alone reads as data loss to
// whoever finds it at 3am. Nothing changed is the whole point.
func TestSyncOnceLogsAFailureAndSaysNothingChanged(t *testing.T) {
	log, h := captureLogger()
	f := &stubPromoFetcher{err: errors.New("console down")}

	res := syncOnceAndLog(consolepromo.NewSyncer(f, stubPromoStore{}, log), log)

	require.Zero(t, res.Ingested)
	rec, ok := h.find("promo_codes is unchanged")
	require.True(t, ok, "a failed ingest must say what it did NOT do: %v", h.all())
	logged, ok := attrsOf(rec)["error"].(error)
	require.True(t, ok, "the failure line must carry the error itself")
	require.Contains(t, logged.Error(), "console down")
}

func intPtr(v int) *int { return &v }
