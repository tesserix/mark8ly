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
// assert not just "bao saw the call" but "bao saw exactly N calls" —
// proving a read/write happened exactly once and never more.
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
		seed    func(bao *recordingClient)
		enc     crypto.Encryptor
		ref     string
		want    string
		wantBao int
	}{
		{
			name: "bao reference reaches bao",
			seed: func(bao *recordingClient) {
				_ = bao.CreateOrAddVersion(context.Background(), BaoPath(scope), []byte("bao-secret"))
			},
			ref:     FormatBaoReference(scope),
			want:    "bao-secret",
			wantBao: 1,
		},
		{
			name: "noop inline reference reaches bao zero times",
			enc:  enc,
			ref:  noopCipher,
			want: "noop-secret",
		},
		{
			name: "aes inline reference reaches bao zero times",
			enc:  aesEnc,
			ref:  aesCipher,
			want: "aes-secret",
		},
		{
			name: "empty reference reaches bao zero times",
			ref:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baoClient := newRecordingClient()
			if tt.seed != nil {
				tt.seed(baoClient)
			}

			store := NewChainStore(ChainConfig{
				Bao:       baoClient,
				Encryptor: tt.enc,
				Primary:   BackendBao,
			})

			got, err := store.Get(context.Background(), tt.ref)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Get() = %q, want %q", got, tt.want)
			}
			if baoClient.accessCalls != tt.wantBao {
				t.Errorf("bao.accessCalls = %d, want %d", baoClient.accessCalls, tt.wantBao)
			}
			if tt.wantBao == 0 && baoClient.totalCalls() != 0 {
				t.Errorf("bao saw %d calls, want 0 (backend not addressed by this reference)", baoClient.totalCalls())
			}
		})
	}
}

// TestChainStore_GSMReferenceFailsExplicitlyAndNeverTouchesBao pins the
// mark8ly#621 contract: GCP Secret Manager was retired from ChainStore, so
// a gsm:// reference — a row the backfill missed — must fail with a
// distinct, self-explaining error rather than falling through to the
// generic "unrecognised reference" branch (whose message deliberately
// names nothing). The failure must never touch the Bao backend, and the
// fallback counter still fires once per attempt so an operator can see the
// row exists.
func TestChainStore_GSMReferenceFailsExplicitlyAndNeverTouchesBao(t *testing.T) {
	scope := testScope()
	baoClient := newRecordingClient()

	var gotLabel string
	var gotIncrement int64
	calls := 0
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
		Counter: func(label string, increment int64) {
			calls++
			gotLabel = label
			gotIncrement = increment
		},
	})

	gsmRef := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	_, err := store.Get(context.Background(), gsmRef)
	if err == nil {
		t.Fatal("Get(gsm://...) error = nil, want the explicit GCP-retired error")
	}
	if !strings.Contains(err.Error(), "GCP Secret Manager") || !strings.Contains(err.Error(), "621") {
		t.Errorf("Get(gsm://...) error = %q, want it to name GCP Secret Manager and mark8ly#621", err.Error())
	}
	if strings.Contains(err.Error(), gsmRef) {
		t.Errorf("Get(gsm://...) error = %q, must not contain any part of the reference %q", err.Error(), gsmRef)
	}
	if baoClient.totalCalls() != 0 {
		t.Errorf("bao saw %d calls, want 0 — a gsm:// reference must never reach OpenBao", baoClient.totalCalls())
	}
	if calls != 1 {
		t.Fatalf("counter called %d times, want 1 — an unmigrated gsm:// row must still be visible", calls)
	}
	if gotLabel != FallbackReadMetric {
		t.Errorf("counter label = %q, want %q", gotLabel, FallbackReadMetric)
	}
	if gotIncrement != 1 {
		t.Errorf("counter increment = %d, want 1", gotIncrement)
	}
}

// TestChainStore_DestroyGSMReferenceFailsExplicitly: Destroy must not
// silently report success for a gsm:// reference — there is no GCP client
// left to route the delete to, and the underlying GCP secret has NOT
// actually been removed (that is a later, human-operator step). Claiming
// success here would be exactly the kind of silent, misleading behaviour
// the package must never produce.
func TestChainStore_DestroyGSMReferenceFailsExplicitly(t *testing.T) {
	scope := testScope()
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
	})

	gsmRef := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	err := store.Destroy(context.Background(), gsmRef)
	if err == nil {
		t.Fatal("Destroy(gsm://...) error = nil, want the explicit GCP-retired error")
	}
	if strings.Contains(err.Error(), gsmRef) {
		t.Errorf("Destroy(gsm://...) error = %q, must not contain any part of the reference %q", err.Error(), gsmRef)
	}
	if baoClient.totalCalls() != 0 {
		t.Errorf("bao saw %d calls, want 0 — a gsm:// reference must never reach OpenBao", baoClient.totalCalls())
	}
}

func TestChainStore_GetUnknownPrefixErrors(t *testing.T) {
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
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
	if baoClient.totalCalls() != 0 {
		t.Errorf("unknown-prefix Get() must not touch bao; saw %d calls", baoClient.totalCalls())
	}
}

func TestChainStore_GetEmptyReference(t *testing.T) {
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
	})

	got, err := store.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("Get(\"\") error = %v", err)
	}
	if got != "" {
		t.Errorf("Get(\"\") = %q, want \"\"", got)
	}
	if baoClient.totalCalls() != 0 {
		t.Errorf("empty reference must not touch bao; saw %d calls", baoClient.totalCalls())
	}
}

func TestChainStore_PutPrimaryBao(t *testing.T) {
	scope := testScope()
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
	})

	ref, err := store.Put(context.Background(), scope, "top-secret")
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	want := FormatBaoReference(scope)
	if ref != want {
		t.Errorf("Put() = %q, want %q", ref, want)
	}
	if baoClient.createCalls != 1 {
		t.Errorf("bao.createCalls = %d, want 1", baoClient.createCalls)
	}
}

// TestChainStore_PutDoesNotFallBackOnBaoError is THE critical test: when
// OpenBao is down, Put must fail — never silently succeed against another
// backend. GCP Secret Manager is gone, so there IS no other backend to
// fall back to any more, but the no-fallback guarantee itself (Put returns
// the error, not a substitute reference) still needs pinning.
func TestChainStore_PutDoesNotFallBackOnBaoError(t *testing.T) {
	scope := testScope()
	baoErr := errors.New("openbao: connection refused")
	baoClient := &erroringClient{err: baoErr}
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
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
}

// TestChainStore_BaoReadDoesNotIncrementFallbackCounter: reading a bao://
// row must NOT increment the fallback counter — only a gsm:// hit does.
func TestChainStore_BaoReadDoesNotIncrementFallbackCounter(t *testing.T) {
	scope := testScope()
	baoClient := newRecordingClient()
	if err := baoClient.CreateOrAddVersion(context.Background(), BaoPath(scope), []byte("current-secret")); err != nil {
		t.Fatalf("seed bao: %v", err)
	}

	calls := 0
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
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

// TestChainStore_MaybeRewrapUpgradesGSM: MaybeRewrap still migrates a
// gsm:// reference to bao:// given already-decoded plaintext (the shape
// this can be reached in now that Get itself refuses every gsm:// read —
// see chain.go's doc comment on MaybeRewrap).
func TestChainStore_MaybeRewrapUpgradesGSM(t *testing.T) {
	scope := testScope()
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
	})

	oldRef := GSMRefPrefix + SecretResource(testProjectID, testPrefix, scope)
	newRef, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
	if !changed {
		t.Fatal("MaybeRewrap() changed = false, want true for a gsm:// ref")
	}
	want := FormatBaoReference(scope)
	if newRef != want {
		t.Errorf("MaybeRewrap() newRef = %q, want %q", newRef, want)
	}
	if baoClient.createCalls != 1 {
		t.Errorf("bao.createCalls = %d, want 1", baoClient.createCalls)
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
// reference already in the primary's (bao://) format.
func TestChainStore_MaybeRewrapNoopForBaoRef(t *testing.T) {
	scope := testScope()
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
	})

	oldRef := FormatBaoReference(scope)
	newRef, changed := store.MaybeRewrap(context.Background(), oldRef, scope, "the-secret")
	if changed {
		t.Errorf("MaybeRewrap() changed = true, want false for a bao:// ref already in the primary's format")
	}
	if newRef != "" {
		t.Errorf("MaybeRewrap() newRef = %q, want \"\"", newRef)
	}
	if baoClient.totalCalls() != 0 {
		t.Errorf("MaybeRewrap() no-op must not touch bao; saw %d calls", baoClient.totalCalls())
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

	var logBuf bytes.Buffer
	var counts []string
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
		Logger:  slog.New(slog.NewTextHandler(&logBuf, nil)),
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

	var warnCount int
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
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

// TestChainStore_GetUnknownPrefixErrorNeverLeaksValue is BLOCKER 3 from an
// earlier whole-branch review: a pre-encryption plaintext row has neither
// "://" nor ":" in it, which is exactly a raw gateway key's shape. The old
// referencePrefix returned the WHOLE input in that case, and handlers wrap
// and log this error — so a raw credential would land verbatim in a log
// line. The fixed error must not contain the input value anywhere, in
// whole or in part.
func TestChainStore_GetUnknownPrefixErrorNeverLeaksValue(t *testing.T) {
	baoClient := newRecordingClient()
	store := NewChainStore(ChainConfig{
		Bao:     baoClient,
		Primary: BackendBao,
	})

	// No "://" and no ":" — the dangerous shape: a raw plaintext credential
	// from a pre-encryption row. Deliberately NOT shaped like a real
	// provider key: GitHub push protection blocks a literal matching a live
	// Stripe/Razorpay pattern, and a fixture that looks like a real
	// credential is a liability in its own right. The property under test is
	// the SHAPE (no scheme, no colon), not the issuer.
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

// TestNewChainStore_PanicsOnNonBaoprimary pins the config-error-at-boot
// contract: Primary must be BackendBao — GCP Secret Manager (BackendGCP)
// was retired in mark8ly#621, and NewChainStore must reject anything else
// loudly at construction time rather than let a zero-value Primary (or a
// stray legacy value) silently misroute every Put.
func TestNewChainStore_PanicsOnNonBaoPrimary(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewChainStore did not panic on an invalid Primary")
		}
	}()
	NewChainStore(ChainConfig{Bao: NewFakeClient(), Primary: Backend("gcp")})
}
