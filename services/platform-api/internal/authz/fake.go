package authz

import (
	"context"
	"sync"
)

// FakeClient is an in-memory Client for unit tests.
//
// Phase O: records one tuple per role per (user, tenant) pair. The
// derived `member`, `can_view_settings`, and `can_edit_settings`
// relations are resolved in Check() by walking the same role map the
// DSL unions over. This keeps the fake in lockstep with the real
// OpenFGA model without parsing DSL.
type FakeClient struct {
	mu sync.Mutex
	// roles[role][user|tenant] = true
	roles          map[Role]map[string]bool
	failNextWrites int
	failNextChecks int
	writeCallCount int
	checkCallCount int
}

// NewFake constructs an empty FakeClient.
func NewFake() *FakeClient {
	roles := make(map[Role]map[string]bool, len(allRoles))
	for _, r := range allRoles {
		roles[r] = make(map[string]bool)
	}
	return &FakeClient{roles: roles}
}

// FailNextWrites makes the next n Write{Membership,Ownership} calls return
// an error. Used to simulate FGA being down for outbox retry tests.
func (f *FakeClient) FailNextWrites(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNextWrites = n
}

// FailNextChecks makes the next n CheckMembership calls return an error.
func (f *FakeClient) FailNextChecks(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNextChecks = n
}

// WriteCallCount returns how many writes have been requested in total.
func (f *FakeClient) WriteCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writeCallCount
}

// CheckCallCount returns how many checks have been requested in total.
func (f *FakeClient) CheckCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkCallCount
}

// HasMembership returns true if the user has any role on the tenant.
// Kept for the existing onboarding integration test which asserts the
// owner write produced membership. Under the new DSL, any role tuple
// implies membership via the derived union.
func (f *FakeClient) HasMembership(userID, tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hasAnyRoleLocked(userID, tenantID)
}

// HasOwnership returns whether the in-memory store records ownership.
func (f *FakeClient) HasOwnership(userID, tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.roles[RoleOwner][userID+"|"+tenantID]
}

// HasRole returns whether a specific role tuple has been written.
func (f *FakeClient) HasRole(userID string, role Role, tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.roles[role]
	if !ok {
		return false
	}
	return m[userID+"|"+tenantID]
}

func (f *FakeClient) WriteOwnership(ctx context.Context, userID, tenantID string) error {
	return f.writeRole(userID, RoleOwner, tenantID)
}

func (f *FakeClient) WriteRole(ctx context.Context, userID string, role Role, tenantID string) error {
	if _, ok := rolePriority[role]; !ok {
		return fakeError("unknown role")
	}
	return f.writeRole(userID, role, tenantID)
}

func (f *FakeClient) writeRole(userID string, role Role, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCallCount++
	if f.failNextWrites > 0 {
		f.failNextWrites--
		return fakeError("simulated FGA write failure")
	}
	f.roles[role][userID+"|"+tenantID] = true
	return nil
}

func (f *FakeClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return f.Check(ctx, userID, "member", tenantID)
}

// Check resolves the same derived-union semantics the DSL defines:
//
//   - member / can_view_settings → any role
//   - can_edit_settings          → owner or admin
//   - owner / admin / staff / viewer → direct tuple match
//
// Keeping the fake's derivation rules hand-maintained is simpler than
// parsing the DSL, and the test suite in authz_test.go guards the
// fake and the real client against drift.
func (f *FakeClient) Check(ctx context.Context, userID, relation, tenantID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCallCount++
	if f.failNextChecks > 0 {
		f.failNextChecks--
		return false, fakeError("simulated FGA check failure")
	}
	key := userID + "|" + tenantID
	switch relation {
	case "member", "can_view_settings":
		return f.hasAnyRoleLocked(userID, tenantID), nil
	case "can_edit_settings", "can_invite_members":
		return f.roles[RoleOwner][key] || f.roles[RoleAdmin][key], nil
	case string(RoleOwner), string(RoleAdmin), string(RoleStaff), string(RoleViewer):
		return f.roles[Role(relation)][key], nil
	default:
		return false, nil
	}
}

func (f *FakeClient) GetRole(ctx context.Context, userID, tenantID string) (Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCallCount++
	if f.failNextChecks > 0 {
		f.failNextChecks--
		return "", fakeError("simulated FGA check failure")
	}
	key := userID + "|" + tenantID
	for _, r := range allRoles {
		if f.roles[r][key] {
			return r, nil
		}
	}
	return "", nil
}

func (f *FakeClient) ListMemberTenants(ctx context.Context, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCallCount++
	if f.failNextChecks > 0 {
		f.failNextChecks--
		return nil, fakeError("simulated FGA check failure")
	}
	// Collect the distinct tenant ids across all roles for this user.
	seen := map[string]struct{}{}
	out := []string{}
	prefix := userID + "|"
	for _, r := range allRoles {
		for key := range f.roles[r] {
			if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
				continue
			}
			tid := key[len(prefix):]
			if _, ok := seen[tid]; ok {
				continue
			}
			seen[tid] = struct{}{}
			out = append(out, tid)
		}
	}
	return out, nil
}

func (f *FakeClient) hasAnyRoleLocked(userID, tenantID string) bool {
	key := userID + "|" + tenantID
	for _, r := range allRoles {
		if f.roles[r][key] {
			return true
		}
	}
	return false
}

// fakeError is a sentinel error type for FakeClient failure injection.
type fakeError string

func (e fakeError) Error() string { return string(e) }
