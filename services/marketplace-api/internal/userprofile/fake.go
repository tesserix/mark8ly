package userprofile

import (
	"context"
	"sync"
)

// FakeStore is an in-memory Store for tests. It lives in the production
// package (not a _test.go file) so handler tests in other packages can
// use it — the same pattern auth.FakeVerifier follows.
type FakeStore struct {
	mu sync.Mutex

	// Rows is the backing map, keyed by user id. Exported so tests can
	// seed and assert against it.
	Rows map[string]Profile

	// GetErr / CreateErr / DisplayNameErr, when set, are returned by the
	// matching method instead of doing the in-memory work. Used to cover
	// the storage-failure branches.
	GetErr         error
	CreateErr      error
	DisplayNameErr error
}

var _ Store = (*FakeStore)(nil)

// NewFakeStore returns an empty FakeStore.
func NewFakeStore() *FakeStore { return &FakeStore{Rows: map[string]Profile{}} }

func (f *FakeStore) Get(_ context.Context, userID string) (Profile, error) {
	if f.GetErr != nil {
		return Profile{}, f.GetErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.Rows[userID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return p, nil
}

func (f *FakeStore) Create(_ context.Context, p Profile) error {
	if f.CreateErr != nil {
		return f.CreateErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Rows == nil {
		f.Rows = map[string]Profile{}
	}
	f.Rows[p.UserID] = p
	return nil
}

func (f *FakeStore) Update(_ context.Context, userID string, fields map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.Rows[userID]
	if !ok {
		return ErrNotFound
	}
	if v, ok := fields["email"].(string); ok {
		p.Email = v
	}
	if v, ok := fields["display_name"].(string); ok {
		p.DisplayName = v
	}
	if v, ok := fields["phone"].(string); ok {
		p.Phone = v
	}
	if v, ok := fields["avatar_url"].(string); ok {
		p.AvatarURL = v
	}
	f.Rows[userID] = p
	return nil
}

func (f *FakeStore) Delete(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Rows, userID)
	return nil
}

func (f *FakeStore) DisplayName(_ context.Context, userID string) (string, error) {
	if f.DisplayNameErr != nil {
		return "", f.DisplayNameErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.Rows[userID]
	if !ok {
		return "", ErrNotFound
	}
	return p.DisplayName, nil
}
