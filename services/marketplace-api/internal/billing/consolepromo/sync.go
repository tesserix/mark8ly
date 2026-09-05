package consolepromo

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/mark8ly/marketplace-api/internal/promo"
)

// Store is the write half of the ingest. It is an interface so the sync
// decisions above it — which codes are kept, which are expired, what happens
// when one code is malformed — are testable without a database.
type Store interface {
	// UpsertCodes inserts or updates rows keyed on code (promo_codes has a
	// unique index on it), so a re-sync is an update in place rather than a
	// duplicate.
	UpsertCodes(ctx context.Context, codes []promo.PromoCode) error

	// ExpireCodesNotIn expires every console-sourced code whose code string
	// is not in keep, by setting valid_until = at. It returns how many rows
	// it changed. Rows already expired at or before at are left alone so a
	// re-run does not keep rewriting them.
	ExpireCodesNotIn(ctx context.Context, keep []string, at time.Time) (int, error)
}

// Result is one sync's outcome.
type Result struct {
	// Ingested is how many definitions were mapped and written.
	Ingested int
	// Skipped is how many were rejected by the mapper.
	Skipped int
	// SkippedByReason breaks Skipped down. Sums to Skipped.
	SkippedByReason map[Reason]int
	// Expired is how many previously-ingested rows were expired for being
	// absent from the catalog.
	Expired int
	// RevisionID is the catalog revision this result describes.
	RevisionID string
}

// Syncer pulls the console's promo catalog into promo_codes.
type Syncer struct {
	fetcher Fetcher
	store   Store
	logger  *slog.Logger
	// now is injectable so expiry timestamps are assertable in tests without
	// reaching for the wall clock.
	now func() time.Time
}

// NewSyncer builds a Syncer. It performs no I/O.
func NewSyncer(f Fetcher, s Store, logger *slog.Logger) *Syncer {
	return &Syncer{fetcher: f, store: s, logger: logger, now: time.Now}
}

// Sync reads the catalog once and reconciles promo_codes against it.
//
// # A malformed code must not cost the batch
//
// Each definition is mapped independently. A rejection is logged, counted by
// reason and skipped; the rest of the batch is written. The alternative —
// failing the sync on the first bad row — would let one badly-filled console
// campaign withhold every other code from every merchant, which is a far
// worse outcome than one code being missing.
//
// # A withdrawn code is EXPIRED, never deleted
//
// promo_redemptions.promo_code_id is NOT NULL REFERENCES promo_codes(id), so
// a delete could fail outright on the foreign key — and where it succeeded
// it would erase the audit trail of who redeemed what. Expiry achieves the
// same thing for every caller that matters: promo.Validate already refuses a
// code past valid_until (validator.go), and the row survives for the
// redemptions that point at it.
//
// # Which codes count as "still in the catalog"
//
// Every code the console published, INCLUDING the ones this sync could not
// map. A definition we failed to parse is present in the catalog and merely
// unreadable by us; expiring it would turn a mapping bug in this service
// into a withdrawn campaign for a merchant. Only genuine absence expires a
// code.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	cat, err := s.fetcher.Fetch(ctx)
	if err != nil {
		// Nothing is written. An unreadable catalog changes nothing at all:
		// previously ingested rows stay exactly as they are.
		return Result{}, fmt.Errorf("consolepromo: fetch: %w", err)
	}

	now := s.now()
	res := Result{RevisionID: cat.RevisionID, SkippedByReason: map[Reason]int{}}

	rows := make([]promo.PromoCode, 0, len(cat.Codes))
	keep := make([]string, 0, len(cat.Codes))
	for _, in := range cat.Codes {
		if in.Code != "" {
			keep = append(keep, in.Code)
		}
		row, err := MapCode(in, now)
		if err != nil {
			reason := ReasonOf(err)
			res.Skipped++
			res.SkippedByReason[reason]++
			CodesSkippedTotal.WithLabelValues(string(reason)).Inc()
			s.warn("consolepromo: skipping a published promo code the mapper rejected",
				"code", in.Code, "reason", string(reason), "error", err)
			continue
		}
		rows = append(rows, row)
	}

	if len(rows) > 0 {
		if err := s.store.UpsertCodes(ctx, rows); err != nil {
			return res, fmt.Errorf("consolepromo: upsert: %w", err)
		}
	}
	res.Ingested = len(rows)

	// Expire AFTER the upsert. In this order a code that is both republished
	// and briefly absent from an intermediate state can never end up
	// expired; in the other order an upsert failure would leave codes
	// expired with nothing written back.
	expired, err := s.store.ExpireCodesNotIn(ctx, keep, now)
	if err != nil {
		return res, fmt.Errorf("consolepromo: expire withdrawn codes: %w", err)
	}
	res.Expired = expired

	CodesIngested.Set(float64(res.Ingested))
	CodesExpiredTotal.Add(float64(res.Expired))
	LastSuccessTimestamp.Set(float64(now.Unix()))

	s.info("consolepromo: promo catalog ingested",
		"revision_id", res.RevisionID, "mode", cat.Mode,
		"ingested", res.Ingested, "skipped", res.Skipped, "expired", res.Expired,
		"skipped_reasons", reasonSummary(res.SkippedByReason))
	return res, nil
}

// reasonSummary renders the skip breakdown in a stable order, so two log
// lines describing the same outcome are the same text and can be diffed.
func reasonSummary(byReason map[Reason]int) string {
	if len(byReason) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(byReason))
	for r, n := range byReason {
		parts = append(parts, fmt.Sprintf("%s=%d", r, n))
	}
	sort.Strings(parts)
	out := parts[0]
	for _, p := range parts[1:] {
		out += "," + p
	}
	return out
}

func (s *Syncer) info(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Info(msg, args...)
	}
}

func (s *Syncer) warn(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, args...)
	}
}
