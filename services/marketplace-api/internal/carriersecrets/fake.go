package carriersecrets

import (
	"context"
	"sync"

	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// FakeClient is an in-memory SecretClient for unit tests. Thread-safe
// so parallel subtests don't race on the shared map.
type FakeClient struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewFakeClient returns an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{data: map[string][]byte{}}
}

// CreateOrAddVersion stores payload at name, overwriting any prior
// value. Matches the production adapter's contract that every call
// produces a readable /versions/latest.
func (f *FakeClient) CreateOrAddVersion(_ context.Context, name string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	f.data[name] = cp
	return nil
}

// AccessLatest returns the payload stored at name, or
// ErrSecretNotFound when the secret was never written.
func (f *FakeClient) AccessLatest(_ context.Context, name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[name]
	if !ok {
		return nil, ErrSecretNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

// DeleteSecret removes name. Idempotent — missing keys are success.
func (f *FakeClient) DeleteSecret(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, name)
	return nil
}

// Has reports whether the fake currently holds a value at name.
// Useful in tests for asserting "Destroy actually deleted the
// secret" without an additional AccessLatest round-trip.
func (f *FakeClient) Has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[name]
	return ok
}

// NewFakeStore builds a ready-to-use HybridStore around a FakeClient.
// Exposed to tests and wiring helpers that want the full Store
// surface (including MaybeRewrap) without a real GCP client.
func NewFakeStore(projectID, prefix string, enc crypto.Encryptor) *HybridStore {
	return NewHybridStore(HybridConfig{
		Client:    NewFakeClient(),
		Encryptor: enc,
		ProjectID: projectID,
		Prefix:    prefix,
	})
}
