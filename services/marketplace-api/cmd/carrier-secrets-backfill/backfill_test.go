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

// newTestChainStore builds a ChainStore around two independent FakeClients
// (bao-primary), exactly the shape internal/carriersecrets/chain_test.go
// uses, so tests exercise the real routing/Put/Get logic rather than a
// hand-rolled Store.
func newTestChainStore() (*carriersecrets.ChainStore, *carriersecrets.FakeClient, *carriersecrets.FakeClient) {
	bao := carriersecrets.NewFakeClient()
	gcp := carriersecrets.NewFakeClient()
	store := carriersecrets.NewChainStore(carriersecrets.ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      carriersecrets.BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})
	return store, bao, gcp
}

// seedGSMRow writes plaintext into the GCP fake at the resource a gsm://
// reference for scope would point at, and returns that reference — i.e. it
// recreates "a row still on GCP SM from before the OpenBao cutover".
func seedGSMRow(t *testing.T, gcp *carriersecrets.FakeClient, scope carriersecrets.Scope, plaintext string) string {
	t.Helper()
	resource := carriersecrets.SecretResource(testProjectID, testPrefix, scope)
	if err := gcp.CreateOrAddVersion(context.Background(), resource, []byte(plaintext)); err != nil {
		t.Fatalf("seed gsm secret: %v", err)
	}
	return carriersecrets.FormatReference(testProjectID, testPrefix, scope)
}

func scopeFor(field string) carriersecrets.Scope {
	return carriersecrets.Scope{TenantID: "tenant-1", Domain: "payment", Provider: "razorpay", Field: field}
}

// TestBackfill_SkipsNonGSMReferences asserts bao://, inline, and empty
// values are left completely untouched — no Store call, no DB update —
// while a genuine gsm:// row in the same batch is still migrated. This is
// the idempotency property: re-running the backfill after (or alongside)
// rows already on bao:// must be a no-op for those rows.
func TestBackfill_SkipsNonGSMReferences(t *testing.T) {
	store, _, gcp := newTestChainStore()
	ctx := context.Background()

	gsmScope := scopeFor("api_key")
	gsmRef := seedGSMRow(t, gcp, gsmScope, "the-plaintext-secret")

	rows := &recordingRowStore{
		rows: []Row{
			{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "row-1", Ref: gsmRef, Scope: gsmScope},
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

	if res.Examined != 4 {
		t.Fatalf("Examined = %d, want 4", res.Examined)
	}
	if res.Skipped != 3 {
		t.Fatalf("Skipped = %d, want 3 (bao://, inline, empty)", res.Skipped)
	}
	if res.Migrated != 1 {
		t.Fatalf("Migrated = %d, want 1 (the gsm:// row)", res.Migrated)
	}
	if res.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", res.Failed)
	}
	if len(rows.updates) != 1 {
		t.Fatalf("DB updates = %d, want exactly 1 (only the gsm:// row)", len(rows.updates))
	}
	if rows.updates[0].row.ID != "row-1" || rows.updates[0].row.Column != "api_key_encrypted" {
		t.Fatalf("unexpected row updated: %+v", rows.updates[0].row)
	}
}

// TestBackfill_DryRunWritesNothing asserts dry-run makes no writes of any
// kind: not to the underlying secret backend, and not to the DB. It still
// reports what it would have done.
func TestBackfill_DryRunWritesNothing(t *testing.T) {
	store, bao, gcp := newTestChainStore()
	ctx := context.Background()

	scope := scopeFor("api_key")
	ref := seedGSMRow(t, gcp, scope, "the-plaintext-secret")

	rows := &recordingRowStore{
		rows: []Row{{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "row-1", Ref: ref, Scope: scope}},
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

// TestBackfill_VerifiesReadBackBeforeUpdating is the safety-critical test:
// when the read-back through the new reference does not reproduce the
// original plaintext, the DB must NOT be updated and the row must count as
// failed, not migrated. This is what makes a migration recoverable — the
// old gsm:// reference (and the GCP secret behind it) is left exactly as
// it was.
func TestBackfill_VerifiesReadBackBeforeUpdating(t *testing.T) {
	inner, _, gcp := newTestChainStore()
	bao := carriersecrets.NewFakeClient()
	// Rebuild inner around the SAME gcp fake but a fresh bao fake so
	// mismatchStore can corrupt independently of what Put already wrote.
	inner = carriersecrets.NewChainStore(carriersecrets.ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      carriersecrets.BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})
	store := &mismatchStore{inner: inner, bao: bao}

	ctx := context.Background()
	scope := scopeFor("api_key")
	ref := seedGSMRow(t, gcp, scope, "the-plaintext-secret")

	rows := &recordingRowStore{
		rows: []Row{{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "row-1", Ref: ref, Scope: scope}},
	}

	b := &Backfiller{Rows: rows, Store: store, DryRun: false}
	res, err := b.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", res.Failed)
	}
	if res.Migrated != 0 {
		t.Fatalf("Migrated = %d, want 0 — a verification mismatch must never count as migrated", res.Migrated)
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
