package firebase

import (
	"context"
	"sync"
)

// FakeClient is an in-memory ClientAPI for unit tests.
type FakeClient struct {
	mu                     sync.Mutex
	ArchiveProjectCalls    int
	DeleteProjectCallCount int
	ArchivedProjectIDs     []string
	DeletedProjectIDs      []string
	ArchiveErr             error
	DeleteErr              error
}

func NewFakeClient() *FakeClient { return &FakeClient{} }

func (f *FakeClient) ArchiveProject(_ context.Context, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ArchiveProjectCalls++
	f.ArchivedProjectIDs = append(f.ArchivedProjectIDs, projectID)
	return f.ArchiveErr
}

func (f *FakeClient) DeleteProject(_ context.Context, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DeleteProjectCallCount++
	f.DeletedProjectIDs = append(f.DeletedProjectIDs, projectID)
	return f.DeleteErr
}
