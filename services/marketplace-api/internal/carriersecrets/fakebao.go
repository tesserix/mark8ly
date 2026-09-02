package carriersecrets

import (
	"context"
	"sync"
)

// FakeBao is an in-memory SecretClient for unit tests of OpenBao-backed
// stores. Thread-safe so parallel subtests don't race on the shared map.
type FakeBao struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewFakeBao returns an empty FakeBao.
func NewFakeBao() *FakeBao {
	return &FakeBao{data: map[string][]byte{}}
}

// CreateOrAddVersion stores payload at name, overwriting any prior
// value. Matches the production adapter's contract that every call
// produces a readable secret at name.
func (f *FakeBao) CreateOrAddVersion(_ context.Context, name string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	f.data[name] = cp
	return nil
}

// AccessLatest returns the payload stored at name, or
// ErrSecretNotFound when the secret was never written.
func (f *FakeBao) AccessLatest(_ context.Context, name string) ([]byte, error) {
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
func (f *FakeBao) DeleteSecret(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, name)
	return nil
}

// Has reports whether the fake currently holds a value at name.
// Useful in tests for asserting "Destroy actually deleted the
// secret" without an additional AccessLatest round-trip.
func (f *FakeBao) Has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[name]
	return ok
}

// Compile-time assertion that *FakeBao satisfies SecretClient.
var _ SecretClient = (*FakeBao)(nil)
