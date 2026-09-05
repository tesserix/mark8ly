package consolepromo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/promo"
)

// fakeFetcher answers with a fixed catalog or a fixed error.
type fakeFetcher struct {
	cat   Catalog
	err   error
	calls int
}

func (f *fakeFetcher) Fetch(context.Context) (Catalog, error) {
	f.calls++
	if f.err != nil {
		return Catalog{}, f.err
	}
	return f.cat, nil
}

// fakeStore records what the Syncer asked it to do. A fake rather than a
// database because every decision under test — which codes are written,
// which are kept, what a malformed one costs the batch — is made above the
// storage layer, and TEST_DATABASE_URL is not always set.
type fakeStore struct {
	upserted   [][]promo.PromoCode
	keptCalls  [][]string
	expireAt   []time.Time
	expireN    int
	upsertErr  error
	expireErr  error
	upsertHits int
}

func (s *fakeStore) UpsertCodes(_ context.Context, codes []promo.PromoCode) error {
	s.upsertHits++
	if s.upsertErr != nil {
		return s.upsertErr
	}
	cp := make([]promo.PromoCode, len(codes))
	copy(cp, codes)
	s.upserted = append(s.upserted, cp)
	return nil
}

func (s *fakeStore) ExpireCodesNotIn(_ context.Context, keep []string, at time.Time) (int, error) {
	if s.expireErr != nil {
		return 0, s.expireErr
	}
	cp := make([]string, len(keep))
	copy(cp, keep)
	s.keptCalls = append(s.keptCalls, cp)
	s.expireAt = append(s.expireAt, at)
	return s.expireN, nil
}

func newSyncer(f Fetcher, s Store) *Syncer {
	sy := NewSyncer(f, s, nil)
	sy.now = func() time.Time { return now }
	return sy
}

func percentCode(code, percent string) Code {
	return Code{Code: code, Discount: &Discount{
		Kind: DiscountKindPercentOff, PercentOff: json.Number(percent), Duration: DurationForever,
	}}
}

func TestSync_WritesMappedCodes(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{RevisionID: "rev-1", Codes: []Code{
		percentCode("LAUNCH50", "50.00"),
		{Code: "EXTRA14DAYS", TrialExtensionDays: ptr(14)},
	}}}
	store := &fakeStore{}

	res, err := newSyncer(f, store).Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Ingested != 2 || res.Skipped != 0 {
		t.Fatalf("ingested=%d skipped=%d, want 2 and 0", res.Ingested, res.Skipped)
	}
	if res.RevisionID != "rev-1" {
		t.Fatalf("revision_id = %q, want rev-1", res.RevisionID)
	}
	if len(store.upserted) != 1 || len(store.upserted[0]) != 2 {
		t.Fatalf("upserted %v, want one batch of two", store.upserted)
	}
}

// TestSync_OneBadCodeDoesNotAbortTheBatch is the property that keeps one
// badly-filled console campaign from withholding every other code from every
// merchant.
func TestSync_OneBadCodeDoesNotAbortTheBatch(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{RevisionID: "rev-2", Codes: []Code{
		percentCode("GOODCODE1", "10.00"),
		percentCode("BADPCT123", "not-a-number"),
		{Code: "NOBENEFIT1"},
		percentCode("GOODCODE2", "20.00"),
	}}}
	store := &fakeStore{}

	res, err := newSyncer(f, store).Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Ingested != 2 {
		t.Fatalf("ingested = %d, want the 2 good codes", res.Ingested)
	}
	if res.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2", res.Skipped)
	}
	if res.SkippedByReason[ReasonPercentOff] != 1 || res.SkippedByReason[ReasonNoBenefit] != 1 {
		t.Fatalf("skipped by reason = %v, want one each of %q and %q",
			res.SkippedByReason, ReasonPercentOff, ReasonNoBenefit)
	}

	got := map[string]bool{}
	for _, row := range store.upserted[0] {
		got[row.Code] = true
	}
	if !got["GOODCODE1"] || !got["GOODCODE2"] {
		t.Fatalf("written codes = %v, want both good codes", got)
	}
}

// TestSync_AnUnmappableCodeIsStillKept — a definition this service could not
// parse is PRESENT in the catalog, merely unreadable. Expiring it would turn
// a mapping bug here into a withdrawn campaign for a merchant.
func TestSync_AnUnmappableCodeIsStillKept(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{Codes: []Code{
		percentCode("GOODCODE1", "10.00"),
		percentCode("BADPCT123", "wat"),
	}}}
	store := &fakeStore{}

	if _, err := newSyncer(f, store).Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(store.keptCalls) != 1 {
		t.Fatalf("expire called %d times, want once", len(store.keptCalls))
	}
	keep := store.keptCalls[0]
	if len(keep) != 2 || keep[0] != "GOODCODE1" || keep[1] != "BADPCT123" {
		t.Fatalf("keep = %v, want both published codes including the unmappable one", keep)
	}
}

// TestSync_WithdrawnCodesAreExpiredAtTheSyncInstant — expiry, never deletion
// (promo_redemptions references these rows), stamped with the sync's own
// clock so the timestamp is assertable.
func TestSync_WithdrawnCodesAreExpiredAtTheSyncInstant(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{Codes: []Code{percentCode("STILLHERE1", "10.00")}}}
	store := &fakeStore{expireN: 3}

	res, err := newSyncer(f, store).Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Expired != 3 {
		t.Fatalf("expired = %d, want 3", res.Expired)
	}
	if len(store.expireAt) != 1 || !store.expireAt[0].Equal(now) {
		t.Fatalf("expired at %v, want the sync instant %v", store.expireAt, now)
	}
}

// TestSync_EmptyCatalogExpiresEverythingAndWritesNothing — no published
// codes is a legitimate state (no campaign is running), distinct from a
// failed read, which is covered below.
func TestSync_EmptyCatalogExpiresEverythingAndWritesNothing(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{RevisionID: "rev-empty"}}
	store := &fakeStore{expireN: 5}

	res, err := newSyncer(f, store).Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Ingested != 0 || res.Expired != 5 {
		t.Fatalf("ingested=%d expired=%d, want 0 and 5", res.Ingested, res.Expired)
	}
	if store.upsertHits != 0 {
		t.Fatalf("upsert called %d times for an empty catalog, want none", store.upsertHits)
	}
	if len(store.keptCalls) != 1 || len(store.keptCalls[0]) != 0 {
		t.Fatalf("keep = %v, want one call with an empty list", store.keptCalls)
	}
}

// TestSync_AFailedReadChangesNothing is the distinction the ErrUnavailable
// doc insists on: "we could not ask" must never be read as "there are no
// codes", which would expire the lot.
func TestSync_AFailedReadChangesNothing(t *testing.T) {
	f := &fakeFetcher{err: ErrUnavailable}
	store := &fakeStore{expireN: 99}

	_, err := newSyncer(f, store).Sync(context.Background())
	if err == nil {
		t.Fatal("expected an error from a failed fetch")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want it to wrap ErrUnavailable", err)
	}
	if store.upsertHits != 0 || len(store.keptCalls) != 0 {
		t.Fatalf("a failed read wrote to the store: upserts=%d expires=%d",
			store.upsertHits, len(store.keptCalls))
	}
}

// TestSync_NotPublishedChangesNothing — 404 is likewise not an empty
// catalog.
func TestSync_NotPublishedChangesNothing(t *testing.T) {
	f := &fakeFetcher{err: ErrNotPublished}
	store := &fakeStore{}

	if _, err := newSyncer(f, store).Sync(context.Background()); !errors.Is(err, ErrNotPublished) {
		t.Fatalf("error = %v, want ErrNotPublished", err)
	}
	if store.upsertHits != 0 || len(store.keptCalls) != 0 {
		t.Fatal("a 404 wrote to the store")
	}
}

// TestSync_UpsertFailureSkipsTheExpirySweep — expiring codes when nothing
// was written back would leave the table strictly worse than before.
func TestSync_UpsertFailureSkipsTheExpirySweep(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{Codes: []Code{percentCode("LAUNCH50", "50.00")}}}
	store := &fakeStore{upsertErr: errors.New("boom")}

	if _, err := newSyncer(f, store).Sync(context.Background()); err == nil {
		t.Fatal("expected the upsert failure to surface")
	}
	if len(store.keptCalls) != 0 {
		t.Fatal("the expiry sweep ran after a failed upsert")
	}
}

func TestSync_ExpireFailureSurfaces(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{Codes: []Code{percentCode("LAUNCH50", "50.00")}}}
	store := &fakeStore{expireErr: errors.New("boom")}

	res, err := newSyncer(f, store).Sync(context.Background())
	if err == nil {
		t.Fatal("expected the expiry failure to surface")
	}
	// The upsert did happen, and the result says so — a partial sync must be
	// reported as what it was, not erased.
	if res.Ingested != 1 {
		t.Fatalf("ingested = %d, want the 1 code that was written before the sweep failed", res.Ingested)
	}
}

func TestReasonSummary_IsStable(t *testing.T) {
	if got := reasonSummary(nil); got != "none" {
		t.Fatalf("reasonSummary(nil) = %q, want \"none\"", got)
	}
	got := reasonSummary(map[Reason]int{ReasonPercentOff: 2, ReasonNoBenefit: 1})
	want := "bad_percent_off=2,no_benefit=1"
	if got != want {
		t.Fatalf("reasonSummary = %q, want %q", got, want)
	}
}

func TestConfig_Configured(t *testing.T) {
	full := Config{
		CatalogURL: "https://console/promo", TokenURL: "https://idp/token",
		ClientID: "id", ClientSecret: "secret",
	}
	if !full.Configured() {
		t.Fatal("a fully populated config reported unconfigured")
	}
	// Each credential is individually load-bearing, and an empty URL is the
	// switch that turns the whole ingest off.
	for name, mutate := range map[string]func(*Config){
		"no url":    func(c *Config) { c.CatalogURL = "" },
		"no token":  func(c *Config) { c.TokenURL = "" },
		"no id":     func(c *Config) { c.ClientID = "" },
		"no secret": func(c *Config) { c.ClientSecret = "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := full
			mutate(&c)
			if c.Configured() {
				t.Fatal("reported configured with a missing setting")
			}
		})
	}
}
