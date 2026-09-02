package carriersecrets

import (
	"context"
	"errors"
	"strings"
	"testing"

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

	_, err := store.Get(context.Background(), "s3://some-bucket/some-key")
	if err == nil {
		t.Fatal("Get() error = nil, want error naming the unknown prefix")
	}
	if !strings.Contains(err.Error(), "s3://") {
		t.Errorf("Get() error = %q, want it to name the prefix %q", err.Error(), "s3://")
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
