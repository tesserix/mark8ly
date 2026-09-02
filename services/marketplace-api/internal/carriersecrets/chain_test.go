package carriersecrets

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

const (
	testProjectID = "test-project"
	testPrefix    = "mark8ly-test"
)

// recordingClient wraps FakeClient and counts calls per method so tests can
// assert not just "the right backend saw the call" but "the OTHER backend
// saw zero calls" — proving a read/write never reached the wrong backend.
type recordingClient struct {
	*FakeClient
	createCalls int
	accessCalls int
	deleteCalls int
}

func newRecordingClient() *recordingClient {
	return &recordingClient{FakeClient: NewFakeClient()}
}

func (r *recordingClient) CreateOrAddVersion(ctx context.Context, name string, payload []byte) error {
	r.createCalls++
	return r.FakeClient.CreateOrAddVersion(ctx, name, payload)
}

func (r *recordingClient) AccessLatest(ctx context.Context, name string) ([]byte, error) {
	r.accessCalls++
	return r.FakeClient.AccessLatest(ctx, name)
}

func (r *recordingClient) DeleteSecret(ctx context.Context, name string) error {
	r.deleteCalls++
	return r.FakeClient.DeleteSecret(ctx, name)
}

func (r *recordingClient) totalCalls() int {
	return r.createCalls + r.accessCalls + r.deleteCalls
}

// erroringClient always fails, simulating a backend that is down.
type erroringClient struct {
	err error
}

func (e *erroringClient) CreateOrAddVersion(context.Context, string, []byte) error {
	return e.err
}

func (e *erroringClient) AccessLatest(context.Context, string) ([]byte, error) {
	return nil, e.err
}

func (e *erroringClient) DeleteSecret(context.Context, string) error {
	return e.err
}

func testScope() Scope {
	return Scope{
		TenantID: "tenant-1",
		Domain:   "payment",
		Provider: "razorpay",
		Field:    "api_key",
	}
}

func TestChainStore_GetRoutesByPrefix(t *testing.T) {
	scope := testScope()
	enc := crypto.NewNoopEncryptor()

	noopCipher, err := enc.Encrypt("noop-secret")
	if err != nil {
		t.Fatalf("Encrypt(noop): %v", err)
	}

	aesKey := make([]byte, 32)
	aesEnc, err := crypto.NewAESEncryptor(aesKey)
	if err != nil {
		t.Fatalf("NewAESEncryptor: %v", err)
	}
	aesCipher, err := aesEnc.Encrypt("aes-secret")
	if err != nil {
		t.Fatalf("Encrypt(aes): %v", err)
	}

	tests := []struct {
		name    string
		seed    func(bao, gcp *recordingClient)
		enc     crypto.Encryptor
		ref     string
		want    string
		wantBao int
		wantGCP int
	}{
		{
			name: "bao reference reaches only bao",
			seed: func(bao, gcp *recordingClient) {
				_ = bao.CreateOrAddVersion(context.Background(), BaoPath(scope), []byte("bao-secret"))
			},
			ref:     FormatBaoReference(scope),
			want:    "bao-secret",
			wantBao: 1, // the seed CreateOrAddVersion call plus the Get's AccessLatest
			wantGCP: 0,
		},
		{
			name: "gsm reference reaches only gcp",
			seed: func(bao, gcp *recordingClient) {
				_ = gcp.CreateOrAddVersion(context.Background(), SecretResource(testProjectID, testPrefix, scope), []byte("gcp-secret"))
			},
			ref:     GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope),
			want:    "gcp-secret",
			wantBao: 0,
			wantGCP: 1,
		},
		{
			name: "noop inline reference reaches neither backend",
			enc:  enc,
			ref:  noopCipher,
			want: "noop-secret",
		},
		{
			name: "aes inline reference reaches neither backend",
			enc:  aesEnc,
			ref:  aesCipher,
			want: "aes-secret",
		},
		{
			name: "empty reference reaches neither backend",
			ref:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bao := newRecordingClient()
			gcp := newRecordingClient()
			if tt.seed != nil {
				tt.seed(bao, gcp)
			}
			seedCreateCalls := bao.createCalls + gcp.createCalls

			store := NewChainStore(ChainConfig{
				Bao:          bao,
				GCP:          gcp,
				Encryptor:    tt.enc,
				Primary:      BackendBao,
				GCPProjectID: testProjectID,
				GCPPrefix:    testPrefix,
			})

			got, err := store.Get(context.Background(), tt.ref)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Get() = %q, want %q", got, tt.want)
			}

			// Subtract the seed writes so we only assert on what Get itself did.
			if bao.accessCalls != tt.wantBao {
				t.Errorf("bao.accessCalls = %d, want %d", bao.accessCalls, tt.wantBao)
			}
			if gcp.accessCalls != tt.wantGCP {
				t.Errorf("gcp.accessCalls = %d, want %d", gcp.accessCalls, tt.wantGCP)
			}
			// The backend NOT addressed by this reference must see zero calls
			// of any kind beyond the seed write.
			if tt.wantBao == 0 && bao.totalCalls() != 0 {
				t.Errorf("bao saw %d calls, want 0 (backend not addressed by this reference)", bao.totalCalls())
			}
			if tt.wantGCP == 0 && gcp.totalCalls() != 0 {
				t.Errorf("gcp saw %d calls, want 0 (backend not addressed by this reference)", gcp.totalCalls())
			}
			_ = seedCreateCalls
		})
	}
}

func TestChainStore_GetUnknownPrefixErrors(t *testing.T) {
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	input := "s3://some-bucket/some-key"
	_, err := store.Get(context.Background(), input)
	if err == nil {
		t.Fatal("Get() error = nil, want error naming the unknown prefix")
	}
	// The error must be safe to log verbatim — it must NEVER echo any part
	// of the input reference, since an unrecognised value can be a raw
	// pre-encryption credential (see TestChainStore_GetUnknownPrefixErrorNeverLeaksValue).
	if strings.Contains(err.Error(), input) || strings.Contains(err.Error(), "s3://") {
		t.Errorf("Get() error = %q, must not contain any part of the input reference %q", err.Error(), input)
	}
	if !strings.Contains(err.Error(), "unrecognised") {
		t.Errorf("Get() error = %q, want it to say the reference is unrecognised", err.Error())
	}
	if bao.totalCalls() != 0 || gcp.totalCalls() != 0 {
		t.Errorf("unknown-prefix Get() must not touch either backend; bao=%d gcp=%d", bao.totalCalls(), gcp.totalCalls())
	}
}

func TestChainStore_GetEmptyReference(t *testing.T) {
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	got, err := store.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("Get(\"\") error = %v", err)
	}
	if got != "" {
		t.Errorf("Get(\"\") = %q, want \"\"", got)
	}
	if bao.totalCalls() != 0 || gcp.totalCalls() != 0 {
		t.Errorf("empty reference must not touch either backend; bao=%d gcp=%d", bao.totalCalls(), gcp.totalCalls())
	}
}

func TestChainStore_PutPrimaryBao(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	ref, err := store.Put(context.Background(), scope, "top-secret")
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	want := FormatBaoReference(scope)
	if ref != want {
		t.Errorf("Put() = %q, want %q", ref, want)
	}
	if bao.createCalls != 1 {
		t.Errorf("bao.createCalls = %d, want 1", bao.createCalls)
	}
	if gcp.totalCalls() != 0 {
		t.Errorf("gcp saw %d calls, want 0 — Put(primary=Bao) must never touch GCP", gcp.totalCalls())
	}
}

// TestChainStore_PutDoesNotFallBackOnBaoError is THE critical test: when
// OpenBao is down, Put must fail — never silently fall back to writing GCP.
// A silent fallback would mint gsm:// references after cutover and make the
// phase-5 "fallback counter has been zero" evidence a lie.
func TestChainStore_PutDoesNotFallBackOnBaoError(t *testing.T) {
	scope := testScope()
	baoErr := errors.New("openbao: connection refused")
	bao := &erroringClient{err: baoErr}
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	ref, err := store.Put(context.Background(), scope, "top-secret")
	if err == nil {
		t.Fatal("Put() error = nil, want error — OpenBao is down")
	}
	if !errors.Is(err, baoErr) {
		t.Errorf("Put() error = %v, want it to wrap %v", err, baoErr)
	}
	if ref != "" {
		t.Errorf("Put() ref = %q, want \"\" on error", ref)
	}
	if gcp.totalCalls() != 0 {
		t.Fatalf("gcp saw %d calls — Put() fell back to GCP after Bao failed, which must never happen", gcp.totalCalls())
	}
}

// TestChainStore_BaoReadDoesNotFallBackToGCP: a bao:// read against a
// failing OpenBao must not try GCP — the value is not there, and falling
// back would turn a transient OpenBao failure into a confusing not-found
// against the wrong backend.
func TestChainStore_BaoReadDoesNotFallBackToGCP(t *testing.T) {
	scope := testScope()
	baoErr := errors.New("openbao: connection refused")
	bao := &erroringClient{err: baoErr}
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	_, err := store.Get(context.Background(), FormatBaoReference(scope))
	if err == nil {
		t.Fatal("Get() error = nil, want error — OpenBao is down")
	}
	if !errors.Is(err, baoErr) {
		t.Errorf("Get() error = %v, want it to wrap %v", err, baoErr)
	}
	if gcp.totalCalls() != 0 {
		t.Fatalf("gcp saw %d calls — bao:// read fell back to GCP, which must never happen", gcp.totalCalls())
	}
}

// TestChainStore_GSMReadIncrementsFallbackCounter: reading a gsm:// row
// while primary is Bao must increment the fallback counter — this is the
// migration's only evidence that GCP SM still has live readers.
func TestChainStore_GSMReadIncrementsFallbackCounter(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	resource := SecretResource(testProjectID, testPrefix, scope)
	if err := gcp.CreateOrAddVersion(context.Background(), resource, []byte("legacy-secret")); err != nil {
		t.Fatalf("seed gcp: %v", err)
	}

	var gotLabel string
	var gotIncrement int64
	calls := 0
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
		Counter: func(label string, increment int64) {
			calls++
			gotLabel = label
			gotIncrement = increment
		},
	})

	plaintext, err := store.Get(context.Background(), GSMRefPrefix+resource)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if plaintext != "legacy-secret" {
		t.Fatalf("Get() = %q, want %q", plaintext, "legacy-secret")
	}
	if calls != 1 {
		t.Fatalf("counter called %d times, want 1", calls)
	}
	if gotLabel != FallbackReadMetric {
		t.Errorf("counter label = %q, want %q", gotLabel, FallbackReadMetric)
	}
	if gotIncrement != 1 {
		t.Errorf("counter increment = %d, want 1", gotIncrement)
	}
}

// TestChainStore_BaoReadDoesNotIncrementFallbackCounter: reading a bao://
// row must NOT increment the fallback counter.
func TestChainStore_BaoReadDoesNotIncrementFallbackCounter(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	if err := bao.CreateOrAddVersion(context.Background(), BaoPath(scope), []byte("current-secret")); err != nil {
		t.Fatalf("seed bao: %v", err)
	}

	calls := 0
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
		Counter: func(string, int64) {
			calls++
		},
	})

	plaintext, err := store.Get(context.Background(), FormatBaoReference(scope))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if plaintext != "current-secret" {
		t.Fatalf("Get() = %q, want %q", plaintext, "current-secret")
	}
	if calls != 0 {
		t.Errorf("counter called %d times, want 0 for a bao:// read", calls)
	}
}

// TestChainStore_MaybeRewrapUpgradesGSM: MaybeRewrap upgrades a gsm://
// reference to bao:// when primary is Bao.
func TestChainStore_MaybeRewrapUpgradesGSM(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	oldRef := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	newRef, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
	if !changed {
		t.Fatal("MaybeRewrap() changed = false, want true for a gsm:// ref when primary=Bao")
	}
	want := FormatBaoReference(scope)
	if newRef != want {
		t.Errorf("MaybeRewrap() newRef = %q, want %q", newRef, want)
	}
	if bao.createCalls != 1 {
		t.Errorf("bao.createCalls = %d, want 1", bao.createCalls)
	}
	if gcp.totalCalls() != 0 {
		t.Errorf("gcp saw %d calls, want 0 — MaybeRewrap(primary=Bao) must never touch GCP", gcp.totalCalls())
	}

	// The rewrap must actually be readable back from Bao.
	got, err := store.Get(context.Background(), newRef)
	if err != nil {
		t.Fatalf("Get(newRef) error = %v", err)
	}
	if got != "the-secret" {
		t.Errorf("Get(newRef) = %q, want %q", got, "the-secret")
	}
}

// TestChainStore_MaybeRewrapNoopForBaoRef: MaybeRewrap is a no-op for a
// reference already in the primary's format.
func TestChainStore_MaybeRewrapNoopForBaoRef(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	oldRef := FormatBaoReference(scope)
	newRef, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
	if changed {
		t.Errorf("MaybeRewrap() changed = true, want false for a bao:// ref already in the primary's format")
	}
	if newRef != "" {
		t.Errorf("MaybeRewrap() newRef = %q, want \"\"", newRef)
	}
	if bao.totalCalls() != 0 || gcp.totalCalls() != 0 {
		t.Errorf("MaybeRewrap() no-op must not touch either backend; bao=%d gcp=%d", bao.totalCalls(), gcp.totalCalls())
	}
}

// TestChainStore_PutPrimaryGCP: with Primary=BackendGCP, Put returns a
// gsm:// reference and the Bao backend records ZERO calls. This is the
// configuration SHIPPING_SECRET_STORE=gcpsm actually constructs — the
// default every existing deployment takes the moment ChainStore ships —
// so it needs the same coverage as the Bao-primary path.
func TestChainStore_PutPrimaryGCP(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendGCP,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	ref, err := store.Put(context.Background(), scope, "top-secret")
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	want := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	if ref != want {
		t.Errorf("Put() = %q, want %q", ref, want)
	}
	if gcp.createCalls != 1 {
		t.Errorf("gcp.createCalls = %d, want 1", gcp.createCalls)
	}
	if bao.totalCalls() != 0 {
		t.Errorf("bao saw %d calls, want 0 — Put(primary=GCP) must never touch Bao", bao.totalCalls())
	}
}

// TestChainStore_PutDoesNotFallBackOnGCPError is the mirror of
// TestChainStore_PutDoesNotFallBackOnBaoError, on the branch that actually
// ships by default (SHIPPING_SECRET_STORE=gcpsm constructs
// Primary=BackendGCP). With GCP down, Put must fail — never silently fall
// back to writing Bao.
func TestChainStore_PutDoesNotFallBackOnGCPError(t *testing.T) {
	scope := testScope()
	gcpErr := errors.New("gcp sm: unavailable")
	bao := newRecordingClient()
	gcp := &erroringClient{err: gcpErr}
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendGCP,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	ref, err := store.Put(context.Background(), scope, "top-secret")
	if err == nil {
		t.Fatal("Put() error = nil, want error — GCP SM is down")
	}
	if !errors.Is(err, gcpErr) {
		t.Errorf("Put() error = %v, want it to wrap %v", err, gcpErr)
	}
	if ref != "" {
		t.Errorf("Put() ref = %q, want \"\" on error", ref)
	}
	if bao.totalCalls() != 0 {
		t.Fatalf("bao saw %d calls — Put() fell back to Bao after GCP failed, which must never happen", bao.totalCalls())
	}
}

// TestChainStore_GCPPrimaryDoesNotIncrementFallbackCounter: under
// GCP-primary, reading a gsm:// reference is reading from the CURRENT
// primary, not a fallback — it must not increment the fallback counter.
// That counter means "we read from the OLD backend while the NEW one is
// primary"; firing it here would inflate the only metric a later phase
// uses to decide GCP Secret Manager can be decommissioned.
func TestChainStore_GCPPrimaryDoesNotIncrementFallbackCounter(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	resource := SecretResource(testProjectID, testPrefix, scope)
	if err := gcp.CreateOrAddVersion(context.Background(), resource, []byte("current-secret")); err != nil {
		t.Fatalf("seed gcp: %v", err)
	}

	calls := 0
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendGCP,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
		Counter: func(string, int64) {
			calls++
		},
	})

	plaintext, err := store.Get(context.Background(), GSMRefPrefix+resource)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if plaintext != "current-secret" {
		t.Fatalf("Get() = %q, want %q", plaintext, "current-secret")
	}
	if calls != 0 {
		t.Errorf("counter called %d times, want 0 — a gsm:// read under GCP-primary is not a fallback", calls)
	}
}

// TestChainStore_MaybeRewrapUnderGCPPrimary pins the decision that
// MaybeRewrap is symmetric: under GCP-primary it migrates a bao:// reference
// back to gsm://. This is the rollback path — see the doc comment on
// MaybeRewrap. The GCP backend must be the only one written.
func TestChainStore_MaybeRewrapUnderGCPPrimary(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendGCP,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	oldRef := FormatBaoReference(scope)
	newRef, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
	if !changed {
		t.Fatal("MaybeRewrap() changed = false, want true for a bao:// ref when primary=GCP")
	}
	want := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	if newRef != want {
		t.Errorf("MaybeRewrap() newRef = %q, want %q", newRef, want)
	}
	if gcp.createCalls != 1 {
		t.Errorf("gcp.createCalls = %d, want 1", gcp.createCalls)
	}
	if bao.totalCalls() != 0 {
		t.Errorf("bao saw %d calls, want 0 — MaybeRewrap(primary=GCP) must never touch Bao", bao.totalCalls())
	}

	// The rewrap must actually be readable back from GCP.
	got, err := store.Get(context.Background(), newRef)
	if err != nil {
		t.Fatalf("Get(newRef) error = %v", err)
	}
	if got != "the-secret" {
		t.Errorf("Get(newRef) = %q, want %q", got, "the-secret")
	}
}

// TestChainStore_BaoRefStillResolvesUnderGCPPrimary pins the property the
// whole rollback story rests on (see pkg/config.ErrOpenBaoRoleRequired):
// after a deployment flips SHIPPING_SECRET_STORE from "bao" back to
// "gcpsm", any row that already migrated to bao:// must still resolve — Get
// routes by the reference's own prefix, never by which backend is
// "primary". If this ever regressed to routing by Primary instead, a
// rollback would silently break checkout/shipping/webhooks for every
// migrated tenant, which is exactly BLOCKER 1 from the whole-branch review.
func TestChainStore_BaoRefStillResolvesUnderGCPPrimary(t *testing.T) {
	scope := testScope()
	bao := newRecordingClient()
	gcp := newRecordingClient()
	// Primary=GCP models the rolled-back ("gcpsm") configuration — the
	// Bao client here still stands in for a live, correctly-configured
	// OpenBao client (i.e. OPENBAO_ROLE/OPENBAO_ADDR were kept set, as
	// pkg/config now requires even in gcpsm mode).
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendGCP,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	// Simulate a row that migrated to bao:// before the rollback, by
	// writing it directly through the Bao client (bypassing Put, which
	// under Primary=GCP would refuse to write there).
	path := BaoPath(scope)
	if err := bao.CreateOrAddVersion(context.Background(), path, []byte("still-live-credential")); err != nil {
		t.Fatalf("seed bao secret: %v", err)
	}
	bao.createCalls = 0 // reset so the assertion below only counts the Get

	got, err := store.Get(context.Background(), FormatBaoReference(scope))
	if err != nil {
		t.Fatalf("Get(bao://...) under Primary=GCP (rolled back) = %v, want nil — rollback must not break already-migrated reads", err)
	}
	if got != "still-live-credential" {
		t.Errorf("Get(bao://...) = %q, want %q", got, "still-live-credential")
	}
	if bao.accessCalls != 1 {
		t.Errorf("bao.accessCalls = %d, want 1 — a bao:// reference must route to Bao regardless of Primary", bao.accessCalls)
	}
	if gcp.totalCalls() != 0 {
		t.Errorf("gcp saw %d calls, want 0 — a bao:// reference must never fall back to GCP", gcp.totalCalls())
	}
}

// TestChainStore_MaybeRewrapFailureIsLoggedAndCounted: a failed rewrap
// write must never be a silent no-op. It must be both logged (so an
// operator can see it) and counted under RewrapFailedMetric (so it shows
// up in metrics), even for a plain transient error that is NOT
// bao.ErrForbidden.
func TestChainStore_MaybeRewrapFailureIsLoggedAndCounted(t *testing.T) {
	scope := testScope()
	transientErr := errors.New("openbao: timeout")
	baoClient := &erroringClient{err: transientErr}
	gcp := newRecordingClient()

	var logBuf bytes.Buffer
	var counts []string
	store := NewChainStore(ChainConfig{
		Bao:          baoClient,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
		Counter: func(label string, n int64) {
			if label == RewrapFailedMetric {
				counts = append(counts, label)
			}
		},
	})

	oldRef := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	newRef, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
	if changed {
		t.Fatalf("MaybeRewrap() changed = true, want false on a failed write")
	}
	if newRef != "" {
		t.Errorf("MaybeRewrap() newRef = %q, want \"\"", newRef)
	}
	if len(counts) != 1 {
		t.Errorf("RewrapFailedMetric fired %d times, want 1", len(counts))
	}
	if logBuf.Len() == 0 {
		t.Error("MaybeRewrap() logged nothing on a failed write, want a log line")
	}
	if !strings.Contains(logBuf.String(), "rewrap failed") {
		t.Errorf("log output = %q, want it to mention the rewrap failure", logBuf.String())
	}
	// A plain transient error must NOT latch — the next call should still
	// try the write.
	if store.rewrapDisabled.Load() {
		t.Error("rewrapDisabled = true after a non-forbidden error, want false")
	}
}

// TestChainStore_MaybeRewrapLatchesAfterForbidden: once OpenBao refuses a
// rewrap write with bao.ErrForbidden (the expected shape of every
// storefront-side MaybeRewrap call, since the storefront engine holds no
// write grant by design), every subsequent MaybeRewrap call on the same
// ChainStore must skip the write attempt entirely — no further calls reach
// the backend, and no further 403s land in OpenBao's audit log.
func TestChainStore_MaybeRewrapLatchesAfterForbidden(t *testing.T) {
	scope := testScope()
	baoClient := &countingErroringClient{err: bao.ErrForbidden}
	gcp := newRecordingClient()

	var warnCount int
	store := NewChainStore(ChainConfig{
		Bao:          baoClient,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Counter: func(label string, n int64) {
			if label == RewrapFailedMetric {
				warnCount++
			}
		},
	})

	oldRef := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)

	for i := 0; i < 3; i++ {
		_, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
		if changed {
			t.Fatalf("call %d: MaybeRewrap() changed = true, want false — backend always refuses", i)
		}
	}

	if !store.rewrapDisabled.Load() {
		t.Fatal("rewrapDisabled = false after a bao.ErrForbidden, want true")
	}
	if baoClient.createCalls != 1 {
		t.Errorf("bao.createCalls = %d, want 1 — the latch must stop every call after the first forbidden response from reaching the backend", baoClient.createCalls)
	}
	if warnCount != 1 {
		t.Errorf("RewrapFailedMetric fired %d times, want 1 — only the call that actually reached the backend counts", warnCount)
	}
}

// countingErroringClient is erroringClient plus a call counter, so a test
// can assert the latch actually stops calls from reaching the backend
// rather than merely ignoring their (still-erroring) result.
type countingErroringClient struct {
	err         error
	createCalls int
}

func (e *countingErroringClient) CreateOrAddVersion(context.Context, string, []byte) error {
	e.createCalls++
	return e.err
}

func (e *countingErroringClient) AccessLatest(context.Context, string) ([]byte, error) {
	return nil, e.err
}

func (e *countingErroringClient) DeleteSecret(context.Context, string) error {
	return e.err
}

// TestChainStore_GetUnknownPrefixErrorNeverLeaksValue is BLOCKER 3 from the
// whole-branch review: a pre-encryption plaintext row has neither "://" nor
// ":" in it, which is exactly a raw gateway key's shape. The old
// referencePrefix returned the WHOLE input in that case, and handlers wrap
// and log this error — so a raw credential would land verbatim in a log
// line. The fixed error must not contain the input value anywhere, in
// whole or in part.
func TestChainStore_GetUnknownPrefixErrorNeverLeaksValue(t *testing.T) {
	bao := newRecordingClient()
	gcp := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:          bao,
		GCP:          gcp,
		Primary:      BackendBao,
		GCPProjectID: testProjectID,
		GCPPrefix:    testPrefix,
	})

	// No "://" and no ":" — the dangerous shape: a raw plaintext credential.
	livePlaintextKey := "raw-credential-value-not-a-reference-0123456789"

	_, err := store.Get(context.Background(), livePlaintextKey)
	if err == nil {
		t.Fatal("Get() error = nil, want error for an unrecognised reference")
	}
	if strings.Contains(err.Error(), livePlaintextKey) {
		t.Fatalf("Get() error = %q, LEAKS the raw input value %q", err.Error(), livePlaintextKey)
	}
	// Also guard against a partial leak (any non-trivial substring of the
	// value appearing in the error), not just an exact match.
	if len(livePlaintextKey) > 8 && strings.Contains(err.Error(), livePlaintextKey[:8]) {
		t.Fatalf("Get() error = %q, leaks a PREFIX of the input value (%q)", err.Error(), livePlaintextKey[:8])
	}
}
