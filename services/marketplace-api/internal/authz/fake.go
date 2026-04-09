package authz

import (
	"context"
	"sync"
)

// FakeClient is an in-memory Client used by unit tests. It mirrors the
// derived-relation semantics of the real OpenFGA model: granting `owner`
// implies `admin` implies `staff` implies `viewer` implies `member`.
//
// FakeClient is safe for concurrent use.
type FakeClient struct {
	mu sync.RWMutex
	// granted[userID][tenantID] = highest direct role (or "" if none)
	granted map[string]map[string]Role
}

// NewFakeClient returns an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{granted: map[string]map[string]Role{}}
}

// Grant assigns a role to a user on a tenant. If the user already has a
// higher role, the call is a no-op (matching real-world "promote up
// only" semantics).
func (f *FakeClient) Grant(userID string, role Role, tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.granted[userID]; !ok {
		f.granted[userID] = map[string]Role{}
	}
	existing := f.granted[userID][tenantID]
	if role.HigherOrEqual(existing) {
		f.granted[userID][tenantID] = role
	}
}

// Revoke removes any role the user holds on the tenant.
func (f *FakeClient) Revoke(userID, tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.granted[userID]; ok {
		delete(m, tenantID)
	}
}

func (f *FakeClient) Check(_ context.Context, userID, relation, tenantID string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	role, ok := f.granted[userID][tenantID]
	if !ok || role == "" {
		return false, nil
	}
	switch relation {
	case "owner":
		return role == RoleOwner, nil
	case "admin":
		return role == RoleOwner || role == RoleAdmin, nil
	case "staff":
		return role == RoleOwner || role == RoleAdmin || role == RoleStaff, nil
	case "viewer":
		return role == RoleOwner || role == RoleAdmin || role == RoleStaff || role == RoleViewer, nil
	case "member":
		return true, nil
	}
	return false, nil
}

func (f *FakeClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return f.Check(ctx, userID, "member", tenantID)
}

func (f *FakeClient) GetRole(_ context.Context, userID, tenantID string) (Role, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	role, ok := f.granted[userID][tenantID]
	if !ok {
		return "", nil
	}
	return role, nil
}
