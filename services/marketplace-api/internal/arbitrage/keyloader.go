package arbitrage

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// KeyVersion is one Secret Manager version: name, raw payload bytes, and the
// time the version was created (used for rotation policy decisions).
type KeyVersion struct {
	Name      string // projects/.../secrets/arbitrage-ip-hmac-key/versions/N
	Payload   []byte
	CreatedAt time.Time
}

// VersionsSource is the injection point for listing enabled key versions.
// Production is backed by SecretManagerSource; tests inject a stub.
type VersionsSource interface {
	ListEnabled(ctx context.Context) ([]KeyVersion, error)
}

// KeyLoader caches Secret Manager versions in-process with a configurable TTL.
// It never blocks on startup — the first call on a cold cache is the one that
// fetches. Thread-safe via a read/write mutex with double-checked locking.
type KeyLoader struct {
	src VersionsSource
	ttl time.Duration

	mu       sync.RWMutex
	cached   []KeyVersion
	cachedAt time.Time
}

// NewKeyLoader constructs a KeyLoader backed by src with the given TTL.
// For production pass 5*time.Minute; tests may pass 1*time.Nanosecond.
func NewKeyLoader(src VersionsSource, ttl time.Duration) *KeyLoader {
	return &KeyLoader{src: src, ttl: ttl}
}

// Latest returns the newest enabled version's payload and name (resource path).
// Used by Hasher for every write — always hashes with the current key.
func (l *KeyLoader) Latest(ctx context.Context) ([]byte, string, error) {
	vs, err := l.get(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(vs) == 0 {
		return nil, "", errors.New("arbitrage: no enabled key versions in Secret Manager")
	}
	return vs[0].Payload, vs[0].Name, nil
}

// All returns every enabled version newest-first. Used by correlation queries
// that need to re-hash a candidate IP under each retained key.
func (l *KeyLoader) All(ctx context.Context) ([]KeyVersion, error) {
	return l.get(ctx)
}

func (l *KeyLoader) get(ctx context.Context) ([]KeyVersion, error) {
	l.mu.RLock()
	if len(l.cached) > 0 && time.Since(l.cachedAt) < l.ttl {
		out := l.cached
		l.mu.RUnlock()
		return out, nil
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	// Double-check under write lock (another goroutine may have refreshed).
	if len(l.cached) > 0 && time.Since(l.cachedAt) < l.ttl {
		return l.cached, nil
	}

	vs, err := l.src.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	// Sort newest-first so Latest() is always index 0.
	sort.Slice(vs, func(i, j int) bool { return vs[i].CreatedAt.After(vs[j].CreatedAt) })
	l.cached = vs
	l.cachedAt = time.Now()
	return vs, nil
}
