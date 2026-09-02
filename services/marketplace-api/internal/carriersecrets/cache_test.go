package carriersecrets

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingStore records how many times each method is called, and lets
// tests inject a failure for the next N Get calls. It is a plain fake in
// the style of chain_test.go's recordingClient, but implements Store
// (not SecretClient) since CachingStore wraps a Store, not a SecretClient.
type countingStore struct {
	mu sync.Mutex

	getCalls     int
	putCalls     int
	destroyCalls int

	values  map[string]string
	failGet map[string]error
}

func newCountingStore() *countingStore {
	return &countingStore{
		values:  make(map[string]string),
		failGet: make(map[string]error),
	}
}

func (c *countingStore) Put(ctx context.Context, scope Scope, plaintext string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putCalls++
	ref := FormatBaoReference(scope)
	c.values[ref] = plaintext
	return ref, nil
}

func (c *countingStore) Get(ctx context.Context, reference string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if err, ok := c.failGet[reference]; ok {
		return "", err
	}
	return c.values[reference], nil
}

func (c *countingStore) Destroy(ctx context.Context, reference string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.destroyCalls++
	delete(c.values, reference)
	return nil
}

func (c *countingStore) setValue(reference, plaintext string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[reference] = plaintext
}

func (c *countingStore) setFailGet(reference string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failGet[reference] = err
}

func (c *countingStore) clearFailGet(reference string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.failGet, reference)
}

func (c *countingStore) getCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getCalls
}

// fakeClock lets tests move time forward deterministically instead of
// sleeping, matching the brief's clock-injection requirement.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func recordingCounter() (CounterFn, func() (label string, total int64)) {
	var mu sync.Mutex
	var lastLabel string
	var total int64
	fn := func(label string, increment int64) {
		mu.Lock()
		defer mu.Unlock()
		lastLabel = label
		total += increment
	}
	get := func() (string, int64) {
		mu.Lock()
		defer mu.Unlock()
		return lastLabel, total
	}
	return fn, get
}

// TestCachingStore_HitWithinTTL: a second Get inside the TTL does not
// reach the inner store.
func TestCachingStore_HitWithinTTL(t *testing.T) {
	inner := newCountingStore()
	inner.setValue("bao://ref-1", "secret-value")
	clock := newFakeClock(time.Unix(0, 0))
	counter, _ := recordingCounter()

	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	got1, err := cache.Get(context.Background(), "bao://ref-1")
	if err != nil {
		t.Fatalf("Get() #1 error = %v", err)
	}
	if got1 != "secret-value" {
		t.Fatalf("Get() #1 = %q, want %q", got1, "secret-value")
	}

	clock.Advance(30 * time.Second)

	got2, err := cache.Get(context.Background(), "bao://ref-1")
	if err != nil {
		t.Fatalf("Get() #2 error = %v", err)
	}
	if got2 != "secret-value" {
		t.Fatalf("Get() #2 = %q, want %q", got2, "secret-value")
	}

	if inner.getCallCount() != 1 {
		t.Errorf("inner.getCalls = %d, want 1 — second Get should be served from cache", inner.getCallCount())
	}
}

// TestCachingStore_ExpiresAfterTTL: after the TTL, the inner store is
// consulted again.
func TestCachingStore_ExpiresAfterTTL(t *testing.T) {
	inner := newCountingStore()
	inner.setValue("bao://ref-1", "secret-value")
	clock := newFakeClock(time.Unix(0, 0))
	counter, _ := recordingCounter()

	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	if _, err := cache.Get(context.Background(), "bao://ref-1"); err != nil {
		t.Fatalf("Get() #1 error = %v", err)
	}

	clock.Advance(61 * time.Second)

	if _, err := cache.Get(context.Background(), "bao://ref-1"); err != nil {
		t.Fatalf("Get() #2 error = %v", err)
	}

	if inner.getCallCount() != 2 {
		t.Errorf("inner.getCalls = %d, want 2 — the entry should have expired", inner.getCallCount())
	}
}

// TestCachingStore_ServesStaleOnError: the inner store fails, a stale
// entry exists, so the stale value is served and the stale counter
// increments.
func TestCachingStore_ServesStaleOnError(t *testing.T) {
	inner := newCountingStore()
	inner.setValue("bao://ref-1", "secret-value")
	clock := newFakeClock(time.Unix(0, 0))
	counter, getCounter := recordingCounter()

	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	if _, err := cache.Get(context.Background(), "bao://ref-1"); err != nil {
		t.Fatalf("Get() #1 error = %v", err)
	}

	clock.Advance(61 * time.Second)
	inner.setFailGet("bao://ref-1", errors.New("openbao: connection refused"))

	got, err := cache.Get(context.Background(), "bao://ref-1")
	if err != nil {
		t.Fatalf("Get() #2 error = %v, want stale value served instead of error", err)
	}
	if got != "secret-value" {
		t.Errorf("Get() #2 = %q, want stale value %q", got, "secret-value")
	}

	label, total := getCounter()
	if label != StaleReadMetric {
		t.Errorf("counter label = %q, want %q", label, StaleReadMetric)
	}
	if total != 1 {
		t.Errorf("counter total = %d, want 1", total)
	}
}

// TestCachingStore_ErrorPropagatesWithoutStale: with no cached entry, an
// inner error propagates. Stale-on-error must not become "swallow all
// errors".
func TestCachingStore_ErrorPropagatesWithoutStale(t *testing.T) {
	inner := newCountingStore()
	wantErr := errors.New("openbao: connection refused")
	inner.setFailGet("bao://ref-missing", wantErr)
	clock := newFakeClock(time.Unix(0, 0))
	counter, getCounter := recordingCounter()

	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	_, err := cache.Get(context.Background(), "bao://ref-missing")
	if err == nil {
		t.Fatal("Get() error = nil, want error to propagate — no cached entry exists")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Get() error = %v, want it to wrap %v", err, wantErr)
	}

	_, total := getCounter()
	if total != 0 {
		t.Errorf("stale counter total = %d, want 0 — no stale value was served", total)
	}
}

// TestCachingStore_PutInvalidates: Put invalidates that reference's
// cache entry so a rotation is visible immediately to the writer.
func TestCachingStore_PutInvalidates(t *testing.T) {
	inner := newCountingStore()
	clock := newFakeClock(time.Unix(0, 0))
	counter, _ := recordingCounter()
	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	scope := testScope()
	ref, err := cache.Put(context.Background(), scope, "v1")
	if err != nil {
		t.Fatalf("Put() #1 error = %v", err)
	}

	got, err := cache.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() #1 error = %v", err)
	}
	if got != "v1" {
		t.Fatalf("Get() #1 = %q, want %q", got, "v1")
	}
	if inner.getCallCount() != 1 {
		t.Fatalf("inner.getCalls = %d, want 1 before rotation", inner.getCallCount())
	}

	// Rotate: Put a new value under the same reference (simulated directly
	// on inner, since countingStore.Put always derives ref from scope —
	// the point under test is that CachingStore.Put invalidates ref).
	inner.setValue(ref, "v2")
	if _, err := cache.Put(context.Background(), scope, "v2"); err != nil {
		t.Fatalf("Put() #2 error = %v", err)
	}

	got2, err := cache.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() #2 error = %v", err)
	}
	if got2 != "v2" {
		t.Errorf("Get() #2 = %q, want %q — Put must invalidate the cached entry", got2, "v2")
	}
	if inner.getCallCount() != 2 {
		t.Errorf("inner.getCalls = %d, want 2 — Get after Put must not be served stale from cache", inner.getCallCount())
	}
}

// TestCachingStore_DestroyInvalidates: Destroy invalidates the cache
// entry — a destroyed credential must not keep resolving from cache.
func TestCachingStore_DestroyInvalidates(t *testing.T) {
	inner := newCountingStore()
	inner.setValue("bao://ref-1", "secret-value")
	clock := newFakeClock(time.Unix(0, 0))
	counter, _ := recordingCounter()
	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	if _, err := cache.Get(context.Background(), "bao://ref-1"); err != nil {
		t.Fatalf("Get() #1 error = %v", err)
	}
	if inner.getCallCount() != 1 {
		t.Fatalf("inner.getCalls = %d, want 1", inner.getCallCount())
	}

	if err := cache.Destroy(context.Background(), "bao://ref-1"); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	// After Destroy, the underlying value is gone too (countingStore.Destroy
	// deletes it), so if the cache still had a stale entry it would still
	// serve the value with zero further inner calls — instead it must miss
	// and go to inner, which now returns "".
	got, err := cache.Get(context.Background(), "bao://ref-1")
	if err != nil {
		t.Fatalf("Get() after Destroy error = %v", err)
	}
	if got != "" {
		t.Errorf("Get() after Destroy = %q, want \"\" — Destroy must invalidate the cache entry", got)
	}
	if inner.getCallCount() != 2 {
		t.Errorf("inner.getCalls = %d, want 2 — Get after Destroy must not be served from cache", inner.getCallCount())
	}
}

// TestCachingStore_ConcurrentGets: concurrent Gets on the same key are
// safe. Run with -race. Uses many real goroutines racing on the same
// key concurrently, not sequential calls wrapped in a goroutine.
func TestCachingStore_ConcurrentGets(t *testing.T) {
	inner := newCountingStore()
	inner.setValue("bao://ref-1", "secret-value")
	clock := newFakeClock(time.Unix(0, 0))
	counter, _ := recordingCounter()
	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	const goroutines = 50
	var wg sync.WaitGroup
	var errCount int64
	wg.Add(goroutines)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			got, err := cache.Get(context.Background(), "bao://ref-1")
			if err != nil || got != "secret-value" {
				atomic.AddInt64(&errCount, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if errCount != 0 {
		t.Errorf("%d of %d concurrent Get() calls failed or returned wrong value", errCount, goroutines)
	}
}

// TestCachingStore_ForwardsRewrapper: CachingStore must forward Rewrapper
// when the inner store implements it, so lazy migration keeps working
// through the cache decorator.
type rewrapRecorder struct {
	*countingStore
	rewrapCalls int
}

func (r *rewrapRecorder) MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (string, bool) {
	r.rewrapCalls++
	return "new-ref", true
}

func TestCachingStore_ForwardsRewrapper(t *testing.T) {
	inner := &rewrapRecorder{countingStore: newCountingStore()}
	clock := newFakeClock(time.Unix(0, 0))
	counter, _ := recordingCounter()
	cache := NewCachingStore(inner, 60*time.Second, clock.Now, counter)

	rw, ok := any(cache).(Rewrapper)
	if !ok {
		t.Fatal("CachingStore does not implement Rewrapper")
	}
	newRef, changed := rw.MaybeRewrap(context.Background(), "old-ref", testScope(), "plaintext")
	if !changed || newRef != "new-ref" {
		t.Errorf("MaybeRewrap() = (%q, %v), want (%q, true)", newRef, changed, "new-ref")
	}
	if inner.rewrapCalls != 1 {
		t.Errorf("inner.rewrapCalls = %d, want 1", inner.rewrapCalls)
	}
}
