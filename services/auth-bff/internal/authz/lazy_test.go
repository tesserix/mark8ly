package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testStoreID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

// fakeFGAServer serves the two OpenFGA endpoints the lazy client touches.
// While `down` is set every request fails, simulating a pod that has not
// finished starting.
func fakeFGAServer(t *testing.T, down *atomic.Bool, listCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/stores", func(w http.ResponseWriter, r *http.Request) {
		listCalls.Add(1)
		if down.Load() {
			http.Error(w, "starting up", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stores":[{"id":"` + testStoreID +
			`","name":"` + FGAStoreName +
			`","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}],` +
			`"continuation_token":""}`))
	})

	mux.HandleFunc("/stores/"+testStoreID+"/check", func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "starting up", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":true}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Regression: startup-time discovery meant a boot that raced openfga left
// authorization dead for the life of the pod. Resolution must be retried
// per call so the first request after recovery succeeds.
func TestLazyClient_RecoversAfterOpenFGAStartsLate(t *testing.T) {
	var down atomic.Bool
	var listCalls atomic.Int32
	down.Store(true)

	srv := fakeFGAServer(t, &down, &listCalls)
	c := NewLazy(LazyConfig{APIURL: srv.URL})

	// openfga is not serving yet: the call must error, not panic.
	if _, err := c.CheckMembership(context.Background(), "user-1", "tenant-1"); err == nil {
		t.Fatal("expected an error while openfga is down")
	}

	// openfga comes up; the very next call must resolve and succeed.
	down.Store(false)
	ok, err := c.CheckMembership(context.Background(), "user-1", "tenant-1")
	if err != nil {
		t.Fatalf("CheckMembership after recovery: %v", err)
	}
	if !ok {
		t.Error("CheckMembership = false, want true")
	}
}

// Once resolved, the store is cached — discovery must not run per call.
func TestLazyClient_CachesResolvedStore(t *testing.T) {
	var down atomic.Bool
	var listCalls atomic.Int32

	srv := fakeFGAServer(t, &down, &listCalls)
	c := NewLazy(LazyConfig{APIURL: srv.URL})

	for i := range 3 {
		if _, err := c.CheckMembership(context.Background(), "user-1", "tenant-1"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := listCalls.Load(); got != 1 {
		t.Errorf("ListStores called %d times, want 1", got)
	}
}

// An explicit store ID skips discovery entirely.
func TestLazyClient_ExplicitStoreIDSkipsDiscovery(t *testing.T) {
	var down atomic.Bool
	var listCalls atomic.Int32

	srv := fakeFGAServer(t, &down, &listCalls)
	c := NewLazy(LazyConfig{APIURL: srv.URL, StoreID: testStoreID})

	if _, err := c.CheckMembership(context.Background(), "user-1", "tenant-1"); err != nil {
		t.Fatalf("CheckMembership: %v", err)
	}
	if got := listCalls.Load(); got != 0 {
		t.Errorf("ListStores called %d times, want 0", got)
	}
}

// A reachable openfga with no matching store is a configuration error, and
// must say so rather than surfacing as a generic failure.
func TestLazyClient_MissingStoreIsNamed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stores":[],"continuation_token":""}`))
	}))
	t.Cleanup(srv.Close)

	c := NewLazy(LazyConfig{APIURL: srv.URL})
	_, err := c.CheckMembership(context.Background(), "user-1", "tenant-1")
	if err == nil {
		t.Fatal("expected an error when no store matches")
	}
	if !strings.Contains(err.Error(), FGAStoreName) {
		t.Errorf("err = %v, want it to name the missing store", err)
	}
}
