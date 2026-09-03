package main

import (
	"context"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

const (
	testProjectID = "test-project"
	testPrefix    = "mark8ly-test"
)

// recordingRowStore is a RowStore fake that hands back a fixed set of rows
// and records every UpdateReference call, so tests can assert "the DB was
// (or was not) written" directly instead of inferring it from a real
// database.
type recordingRowStore struct {
	rows    []Row
	updates []recordedUpdate
}

type recordedUpdate struct {
	row    Row
	newRef string
}

func (f *recordingRowStore) FetchAll(context.Context) ([]Row, error) {
	return f.rows, nil
}

func (f *recordingRowStore) UpdateReference(_ context.Context, row Row, newRef string) error {
	f.updates = append(f.updates, recordedUpdate{row: row, newRef: newRef})
	return nil
}

// newTestChainStore builds a bao-primary ChainStore around a FakeClient,
// exactly the shape internal/carriersecrets/chain_test.go uses, so tests
// exercise the real routing/Put/Get logic rather than a hand-rolled Store.
// There is no GCP fake any more — GCP Secret Manager was retired from
// ChainStore in mark8ly#621, so a gsm:// reference now fails unconditionally
// regardless of what (if anything) it would once have pointed at; see
// TestBackfill_GSMRowFailsSinceGCPRetired.
func newTestChainStore() (*carriersecrets.ChainStore, *carriersecrets.FakeClient) {
	bao := carriersecrets.NewFakeClient()
	store := carriersecrets.NewChainStore(carriersecrets.ChainConfig{
		Bao:     bao,
		Primary: carriersecrets.BackendBao,
	})
	return store, bao
}

// gsmRefFor formats the "gsm://" reference a legacy (pre-OpenBao-cutover)
// row for scope would have carried. It never actually writes anything
// anywhere — mark8ly#621 retired GCP Secret Manager, so no Store can
// resolve this value any more (see TestBackfill_GSMRowFailsSinceGCPRetired);
// the only thing a test needs from it is the reference string itself.
func gsmRefFor(scope carriersecrets.Scope) string {
	return carriersecrets.FormatReference(testProjectID, testPrefix, scope)
}

func scopeFor(field string) carriersecrets.Scope {
	return carriersecrets.Scope{TenantID: "tenant-1", Domain: "payment", Provider: "razorpay", Field: field}
}

// TestBackfill_SkipsNonGSMReferences asserts bao://, inline, and empty
// values are left completely untouched — no Store call, no DB update. This
// is the idempotency property: re-running the backfill against rows
// already on bao:// (or otherwise never on GCP SM) must be a no-op for
// every one of them.
func TestBackfill_SkipsNonGSMReferences(t *testing.T) {
	store, _ := newTestChainStore()
	ctx := context.Background()

	rows := &recordingRowStore{
		rows: []Row{
			{Table: "payment_gateway_configs", Column: "secret_key_encrypted", ID: "row-1", Ref: carriersecrets.FormatBaoReference(scopeFor("secret_key")), Scope: scopeFor("secret_key")},
			{Table: "payment_gateway_configs", Column: "webhook_secret_encrypted", ID: "row-1", Ref: "noop:already-inline", Scope: scopeFor("webhook_secret")},
			{Table: "shipping_carrier_configs", Column: "api_key_encrypted", ID: "row-2", Ref: "", Scope: scopeFor("api_key")},
		},
	}

	b := &Backfiller{Rows: rows, Store: store, DryRun: false}
	res, err := b.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Examined != 3 {
		t.Fatalf("Examined = %d, want 3", res.Examined)
	}
	if res.Skipped != 3 {
		t.Fatalf("Skipped = %d, want 3 (bao://, inline, empty)", res.Skipped)
	}
	if res.Migrated != 0 || res.Failed != 0 {
		t.Fatalf("Migrated/Failed = %d/%d, want 0/0 — nothing here is a gsm:// row", res.Migrated, res.Failed)
	}
	if len(rows.updates) != 0 {
		t.Fatalf("DB updates = %d, want 0", len(rows.updates))
	}
}

// TestBackfill_GSMRowFailsSinceGCPRetired pins the mark8ly#621 consequence
// for this tool: GCP Secret Manager was retired from ChainStore, so a
// genuine gsm:// row (one the census in verify.go would flag) can no
// longer be migrated by this job — Store.Get fails on it unconditionally,
// before Put or the read-back verification ever run. The row must count as
// Failed, never Migrated, and the DB must be left untouched. Recovering
// any such row is now a human-operator task (restore the plaintext by hand
// and re-save it, which lazy rewrap or a fresh backfill row will then pick
// up as bao://), not something this job can do automatically any more.
func TestBackfill_GSMRowFailsSinceGCPRetired(t *testing.T) {
	store, _ := newTestChainStore()
	ctx := context.Background()

	scope := scopeFor("api_key")
	rows := &recordingRowStore{
		rows: []Row{{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "row-1", Ref: gsmRefFor(scope), Scope: scope}},
	}

	b := &Backfiller{Rows: rows, Store: store, DryRun: false}
	res, err := b.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Examined != 1 {
		t.Fatalf("Examined = %d, want 1", res.Examined)
	}
	if res.Failed != 1 {
		t.Fatalf("Failed = %d, want 1 — GCP Secret Manager is retired, this row cannot resolve", res.Failed)
	}
	if res.Migrated != 0 {
		t.Fatalf("Migrated = %d, want 0", res.Migrated)
	}
	if len(rows.updates) != 0 {
		t.Fatalf("DB updates = %d, want 0 — a row that failed to resolve must never be updated", len(rows.updates))
	}
}

// TestBackfill_DryRunWritesNothing asserts dry-run makes no writes of any
// kind: not to the underlying secret backend, and not to the DB. It still
// reports what it would have done. Dry-run never calls Store at all (see
// Backfiller.Run), so this holds even though GCP Secret Manager is retired
// and a real (non-dry-run) attempt against the same row would now fail.
func TestBackfill_DryRunWritesNothing(t *testing.T) {
	store, bao := newTestChainStore()
	ctx := context.Background()

	scope := scopeFor("api_key")
	rows := &recordingRowStore{
		rows: []Row{{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "row-1", Ref: gsmRefFor(scope), Scope: scope}},
	}

	b := &Backfiller{Rows: rows, Store: store, DryRun: true}
	res, err := b.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Examined != 1 || res.Migrated != 1 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(rows.updates) != 0 {
		t.Fatalf("DB updates = %d, want 0 under dry-run", len(rows.updates))
	}
	baoPath := carriersecrets.BaoPath(scope)
	if bao.Has(baoPath) {
		t.Fatalf("bao path %q was written under dry-run — it must not be", baoPath)
	}
}

// mismatchStore wraps a real ChainStore but corrupts the value it just
// wrote on every Put, so the caller's subsequent verification read sees a
// different plaintext than what it asked to store. This is the fixture for
// TestBackfill_VerifiesReadBackBeforeUpdating — the single most important
// test in this package.
type mismatchStore struct {
	inner *carriersecrets.ChainStore
	bao   *carriersecrets.FakeClient
}

func (m *mismatchStore) Get(ctx context.Context, ref string) (string, error) {
	return m.inner.Get(ctx, ref)
}

func (m *mismatchStore) Put(ctx context.Context, scope carriersecrets.Scope, plaintext string) (string, error) {
	ref, err := m.inner.Put(ctx, scope, plaintext)
	if err != nil {
		return "", err
	}
	// Immediately overwrite the value that was just written so a
	// subsequent Get(ref) returns something other than plaintext —
	// simulating "the write silently landed wrong" without needing a
	// real broken backend.
	path := carriersecrets.BaoPath(scope)
	if err := m.bao.CreateOrAddVersion(ctx, path, []byte("corrupted-does-not-match-original")); err != nil {
		return "", err
	}
	return ref, nil
}

func (m *mismatchStore) Destroy(ctx context.Context, ref string) error {
	return m.inner.Destroy(ctx, ref)
}

var _ carriersecrets.Store = (*mismatchStore)(nil)

// TestBackfill_VerifiesReadBackBeforeUpdating is the safety-critical test
// for migrateRow's read-back verification: when the read-back through the
// new reference does not reproduce the original plaintext, the DB must NOT
// be updated and the row must count as failed, not migrated. This is what
// makes a migration recoverable.
//
// It calls b.migrateRow directly rather than going through Backfiller.Run:
// Run only ever invokes migrateRow for a genuine gsm:// row, and GCP Secret
// Manager was retired from ChainStore in mark8ly#621 — Store.Get on a
// gsm:// reference now fails unconditionally, before migrateRow ever
// reaches Put or the verification read (see
// TestBackfill_GSMRowFailsSinceGCPRetired). Calling migrateRow directly
// with a resolvable bao:// starting reference is what still lets this test
// exercise the verification logic itself, decoupled from the now-defunct
// gsm:// dispatch.
func TestBackfill_VerifiesReadBackBeforeUpdating(t *testing.T) {
	bao := carriersecrets.NewFakeClient()
	inner := carriersecrets.NewChainStore(carriersecrets.ChainConfig{
		Bao:     bao,
		Primary: carriersecrets.BackendBao,
	})
	store := &mismatchStore{inner: inner, bao: bao}

	ctx := context.Background()
	scope := scopeFor("api_key")
	if _, err := inner.Put(ctx, scope, "the-plaintext-secret"); err != nil {
		t.Fatalf("seed original value: %v", err)
	}
	oldRef := carriersecrets.FormatBaoReference(scope)

	rows := &recordingRowStore{}
	b := &Backfiller{Rows: rows, Store: store, DryRun: false}
	row := Row{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "row-1", Ref: oldRef, Scope: scope}

	if err := b.migrateRow(ctx, row); err == nil {
		t.Fatal("migrateRow() error = nil, want an error — the read-back must not reproduce the corrupted write")
	}
	if len(rows.updates) != 0 {
		t.Fatalf("DB updates = %d, want 0 — the DB must never be updated when read-back verification fails", len(rows.updates))
	}
}

func TestRequireBaoPrimary(t *testing.T) {
	if err := requireBaoPrimary("bao"); err != nil {
		t.Fatalf("requireBaoPrimary(bao) = %v, want nil", err)
	}
	for _, mode := range []string{"gcpsm", "inline", "", "bogus"} {
		if err := requireBaoPrimary(mode); err == nil {
			t.Fatalf("requireBaoPrimary(%q) = nil, want an error", mode)
		}
	}
}
