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
