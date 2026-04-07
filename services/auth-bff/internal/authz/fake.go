package authz

import (
	"context"
	"sync"
)

// FakeClient is an in-memory Client for unit tests.
//
// SetMembership records a "user X is a member of tenant Y" entry. Tests
// can also use FailNextChecks to simulate transient OpenFGA failures.
type FakeClient struct {
	mu             sync.Mutex
	memberships    map[string]bool
	failNextChecks int
	checkCalls     int
}

// NewFake constructs an empty FakeClient.
func NewFake() *FakeClient {
	return &FakeClient{memberships: map[string]bool{}}
}

// SetMembership records a tuple. Idempotent.
func (f *FakeClient) SetMembership(userID, tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.memberships[userID+"|"+tenantID] = true
}

// FailNextChecks makes the next n CheckMembership calls return an error.
func (f *FakeClient) FailNextChecks(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNextChecks = n
}

// CheckCallCount returns how many checks the autologin retry loop made.
func (f *FakeClient) CheckCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkCalls
}

// CheckMembership returns whether the in-memory store has the membership.
func (f *FakeClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCalls++

	if f.failNextChecks > 0 {
		f.failNextChecks--
		return false, fakeError("simulated openfga failure")
	}
	return f.memberships[userID+"|"+tenantID], nil
}

type fakeError string

func (e fakeError) Error() string { return string(e) }
