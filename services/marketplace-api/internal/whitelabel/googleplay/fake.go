package googleplay

import (
	"context"
	"sync"
)

// FakeClient is an in-memory ClientAPI for unit tests.
type FakeClient struct {
	mu                      sync.Mutex
	BlockDownloadsCallCount int
	PullAppCallCount        int
	BlockedPackages         []string
	PulledPackages          []string
	BlockDownloadsErr       error
	PullAppErr              error
}

func NewFakeClient() *FakeClient { return &FakeClient{} }

func (f *FakeClient) BlockDownloads(_ context.Context, packageName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BlockDownloadsCallCount++
	f.BlockedPackages = append(f.BlockedPackages, packageName)
	return f.BlockDownloadsErr
}

func (f *FakeClient) PullApp(_ context.Context, packageName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.PullAppCallCount++
	f.PulledPackages = append(f.PulledPackages, packageName)
	return f.PullAppErr
}
