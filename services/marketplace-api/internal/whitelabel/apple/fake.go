package apple

import (
	"context"
	"sync"
)

// FakeClient is an in-memory ClientAPI for unit tests. Records the
// sequence of calls so tests can assert order + idempotency.
type FakeClient struct {
	mu                      sync.Mutex
	BlockDownloadsCallCount int
	PullAppCallCount        int
	BlockedAppIDs           []string
	PulledAppIDs            []string
	// BlockDownloadsErr / PullAppErr make subsequent calls return the
	// named error — used to exercise the advancer's error path.
	BlockDownloadsErr error
	PullAppErr        error

	// Apps is what ListApps returns; ListAppsErr overrides it. Zero,
	// one and many are all expressible so callers can be tested against
	// each — an empty account and a rejected key are different facts.
	Apps              []App
	ListAppsErr       error
	ListAppsCallCount int
}

// NewFakeClient is a convenience zero-value constructor.
func NewFakeClient() *FakeClient { return &FakeClient{} }

func (f *FakeClient) BlockDownloads(_ context.Context, appleAppID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BlockDownloadsCallCount++
	f.BlockedAppIDs = append(f.BlockedAppIDs, appleAppID)
	return f.BlockDownloadsErr
}

func (f *FakeClient) PullApp(_ context.Context, appleAppID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.PullAppCallCount++
	f.PulledAppIDs = append(f.PulledAppIDs, appleAppID)
	return f.PullAppErr
}

func (f *FakeClient) ListApps(context.Context) ([]App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ListAppsCallCount++
	if f.ListAppsErr != nil {
		return nil, f.ListAppsErr
	}
	// Copy: ListApps is a read, and a caller must not be able to mutate
	// the fake's state through the slice it gets back.
	out := make([]App, len(f.Apps))
	copy(out, f.Apps)
	return out, nil
}
