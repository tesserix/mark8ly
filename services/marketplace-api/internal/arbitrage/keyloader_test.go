package arbitrage

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubSource is the test double for VersionsSource.
type stubSource struct {
	versions []KeyVersion
	err      error
	calls    int
}

func (s *stubSource) ListEnabled(_ context.Context) ([]KeyVersion, error) {
	s.calls++
	return s.versions, s.err
}

func TestKeyLoader_LatestReturnsNewest(t *testing.T) {
	now := time.Now()
	older := now.Add(-30 * 24 * time.Hour)
	src := &stubSource{
		versions: []KeyVersion{
			{Name: "v1", Payload: []byte("old-key"), CreatedAt: older},
			{Name: "v2", Payload: []byte("new-key"), CreatedAt: now},
		},
	}
	loader := NewKeyLoader(src, 5*time.Minute)
	payload, name, err := loader.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() error: %v", err)
	}
	if string(payload) != "new-key" {
		t.Errorf("payload=%q, want new-key", payload)
	}
	if name != "v2" {
		t.Errorf("name=%q, want v2", name)
	}
}

func TestKeyLoader_AllReturnsBothForCorrelation(t *testing.T) {
	now := time.Now()
	src := &stubSource{
		versions: []KeyVersion{
			{Name: "v1", Payload: []byte("old"), CreatedAt: now.Add(-30 * 24 * time.Hour)},
			{Name: "v2", Payload: []byte("new"), CreatedAt: now},
		},
	}
	loader := NewKeyLoader(src, 5*time.Minute)
	all, err := loader.All(context.Background())
	if err != nil {
		t.Fatalf("All() error: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("len(All())=%d, want 2", len(all))
	}
	// Newest-first ordering.
	if all[0].Name != "v2" {
		t.Errorf("all[0].Name=%q, want v2 (newest first)", all[0].Name)
	}
}

func TestKeyLoader_CachesBetweenCalls(t *testing.T) {
	src := &stubSource{
		versions: []KeyVersion{{Name: "v1", Payload: []byte("k"), CreatedAt: time.Now()}},
	}
	loader := NewKeyLoader(src, 5*time.Minute)
	_, _, _ = loader.Latest(context.Background())
	_, _, _ = loader.Latest(context.Background())
	if src.calls != 1 {
		t.Errorf("src.calls=%d, want 1 (second call served from cache)", src.calls)
	}
}

func TestKeyLoader_TTLExpiryRefetches(t *testing.T) {
	src := &stubSource{
		versions: []KeyVersion{{Name: "v1", Payload: []byte("k"), CreatedAt: time.Now()}},
	}
	loader := NewKeyLoader(src, 1*time.Nanosecond)
	_, _, _ = loader.Latest(context.Background())
	time.Sleep(2 * time.Nanosecond)
	_, _, _ = loader.Latest(context.Background())
	if src.calls < 2 {
		t.Errorf("src.calls=%d, want >=2 after TTL expiry", src.calls)
	}
}

func TestKeyLoader_ErrorPropagates(t *testing.T) {
	src := &stubSource{err: errors.New("secret manager down")}
	loader := NewKeyLoader(src, time.Minute)
	_, _, err := loader.Latest(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestKeyLoader_EmptyVersionsError(t *testing.T) {
	src := &stubSource{versions: nil}
	loader := NewKeyLoader(src, time.Minute)
	_, _, err := loader.Latest(context.Background())
	if err == nil {
		t.Error("expected error for empty versions, got nil")
	}
}
