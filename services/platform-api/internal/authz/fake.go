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
	roles map[Role]map[string]bool
	// storeParents[storeID] = tenantID — Phase Q store->tenant
	// parent tuples.
	storeParents map[string]string
	// storeRoles[relation][user|storeID] = true — Phase R direct
	// store-level role grants (manager/staff/viewer on a store).
	storeRoles     map[string]map[string]bool
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
	return &FakeClient{
		roles:        roles,
		storeParents: map[string]string{},
		storeRoles:   map[string]map[string]bool{},
	}
}

// WriteRoleObject records a direct role tuple on an arbitrary
// object type. Phase R uses this for store-level grants —
// e.g. WriteRoleObject(ctx, uid, "manager", "store", sid).
func (f *FakeClient) WriteRoleObject(ctx context.Context, userID, relation, objectType, objectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCallCount++
	if f.failNextWrites > 0 {
		f.failNextWrites--
		return fakeError("simulated FGA write failure")
	}
	if objectType != "store" {
		// Only store-level grants are modelled today; tenant-level
		// grants go through WriteRole. Silently accept other types
		// so future call sites don't explode.
		return nil
	}
	if _, ok := f.storeRoles[relation]; !ok {
		f.storeRoles[relation] = map[string]bool{}
	}
	f.storeRoles[relation][userID+"|"+objectID] = true
	return nil
}

// HasStoreRole is a test helper to assert a store-level role grant
// was written.
func (f *FakeClient) HasStoreRole(userID, relation, storeID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.storeRoles[relation]
	if !ok {
		return false
	}
	return m[userID+"|"+storeID]
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

// WriteStoreParent records a store→tenant parent tuple. Idempotent:
// writing the same pair twice is a no-op.
func (f *FakeClient) WriteStoreParent(ctx context.Context, storeID, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCallCount++
	if f.failNextWrites > 0 {
		f.failNextWrites--
		return fakeError("simulated FGA write failure")
	}
	f.storeParents[storeID] = tenantID
	return nil
}

// HasStoreParent is a test helper to assert a parent tuple was written.
func (f *FakeClient) HasStoreParent(storeID, tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.storeParents[storeID] == tenantID
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
	return f.CheckObject(ctx, userID, relation, "tenant", tenantID)
}

// CheckObject resolves the Phase O tenant relations AND the Phase Q
// store relations. Store-type lookups use the storeParents map to
// walk up to the tenant, mirroring the DSL's `from parent`
// inheritance rules. No store-direct role tuples exist in Phase Q
// (stores only inherit); Phase R will teach this method about
// store-level owner/manager/staff/viewer tuples when it lands.
func (f *FakeClient) CheckObject(ctx context.Context, userID, relation, objectType, objectID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCallCount++
	if f.failNextChecks > 0 {
		f.failNextChecks--
		return false, fakeError("simulated FGA check failure")
	}

	switch objectType {
	case "tenant":
		return f.checkTenantRelationLocked(userID, relation, objectID), nil
	case "store":
		// Direct store-level role grant (Phase R). Check first
		// before walking up to the parent tenant so a store-only
		// invitee short-circuits without needing tenant access.
		storeKey := userID + "|" + objectID
		direct := func(rel string) bool {
			m, ok := f.storeRoles[rel]
			return ok && m[storeKey]
		}
		// Relation-keyed lookups for direct role strings first.
		if m, ok := f.storeRoles[relation]; ok && m[storeKey] {
			return true, nil
		}

		tenantID, hasParent := f.storeParents[objectID]
		tenantKey := userID + "|" + tenantID
		tenantHasRole := func(r Role) bool {
			return hasParent && f.roles[r][tenantKey]
		}

		switch relation {
		case "can_view_store", "member":
			return direct("owner") || direct("manager") ||
				direct("staff") || direct("viewer") ||
				(hasParent && f.hasAnyRoleLocked(userID, tenantID)), nil
		case "can_edit_store_settings":
			return direct("owner") || direct("manager") ||
				tenantHasRole(RoleOwner) || tenantHasRole(RoleAdmin), nil
		case "can_manage_catalog":
			return direct("owner") || direct("manager") || direct("staff") ||
				tenantHasRole(RoleOwner) || tenantHasRole(RoleAdmin), nil
		}
	}
	return false, nil
}

func (f *FakeClient) checkTenantRelationLocked(userID, relation, tenantID string) bool {
	key := userID + "|" + tenantID
	switch relation {
	case "member", "can_view_settings":
		return f.hasAnyRoleLocked(userID, tenantID)
	case "can_edit_settings", "can_invite_members", "can_manage_stores":
		return f.roles[RoleOwner][key] || f.roles[RoleAdmin][key]
	case string(RoleOwner), string(RoleAdmin), string(RoleStaff), string(RoleViewer):
		return f.roles[Role(relation)][key]
	}
	return false
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

// ListTenantMembers returns every user holding a direct role tuple on
// the tenant, mirroring the real client's Read-based enumeration. No
// pagination to simulate here — the fake holds everything in memory.
func (f *FakeClient) ListTenantMembers(ctx context.Context, tenantID string) ([]Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCallCount++
	if f.failNextChecks > 0 {
		f.failNextChecks--
		return nil, fakeError("simulated FGA check failure")
	}
	var out []Member
	suffix := "|" + tenantID
	for _, r := range allRoles {
		for key := range f.roles[r] {
			if len(key) <= len(suffix) || key[len(key)-len(suffix):] != suffix {
				continue
			}
			userID := key[:len(key)-len(suffix)]
			out = append(out, Member{UserID: userID, Relation: string(r)})
		}
	}
	return out, nil
}

// DeleteTuple removes the user:<userID> <relation> tenant:<tenantID>
// tuple if present. Idempotent: deleting an already-absent tuple (or
// an unrecognized relation) is a no-op that returns nil, mirroring
// the real client's tolerance of OpenFGA's "tuple not found" error.
func (f *FakeClient) DeleteTuple(ctx context.Context, userID, relation, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.roles[Role(relation)]; ok {
		delete(m, userID+"|"+tenantID)
	}
	return nil
}

// DeleteStoreParent removes the store→tenant parent tuple if it
// matches. Idempotent: deleting an already-absent (or mismatched)
// pair is a no-op that returns nil.
func (f *FakeClient) DeleteStoreParent(ctx context.Context, storeID, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.storeParents[storeID] == tenantID {
		delete(f.storeParents, storeID)
	}
	return nil
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
