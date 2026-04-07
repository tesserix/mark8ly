package authz

import (
	"context"
	"sync"
)

// FakeClient is an in-memory Client for unit tests.
//
// It records every Write call so tests can assert "this user-tenant pair
// was written" without standing up a real OpenFGA. Returns canned errors
// from FailNextWrite to simulate transient FGA failures (used by the
// outbox retry tests).
type FakeClient struct {
	mu              sync.Mutex
	memberships     map[string]bool // key = userID + "|" + tenantID
	ownerships      map[string]bool
	failNextWrites  int
	failNextChecks  int
	writeCallCount  int
	checkCallCount  int
}

// NewFake constructs an empty FakeClient.
func NewFake() *FakeClient {
	return &FakeClient{
		memberships: make(map[string]bool),
		ownerships:  make(map[string]bool),
	}
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

// HasMembership returns whether the in-memory store records a membership.
// Used by tests to assert side effects.
func (f *FakeClient) HasMembership(userID, tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.memberships[userID+"|"+tenantID]
}

// HasOwnership returns whether the in-memory store records ownership.
func (f *FakeClient) HasOwnership(userID, tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ownerships[userID+"|"+tenantID]
}

func (f *FakeClient) WriteMembership(ctx context.Context, userID, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCallCount++
	if f.failNextWrites > 0 {
		f.failNextWrites--
		return fakeError("simulated FGA write failure")
	}
	f.memberships[userID+"|"+tenantID] = true
	return nil
}

func (f *FakeClient) WriteOwnership(ctx context.Context, userID, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCallCount++
	if f.failNextWrites > 0 {
		f.failNextWrites--
		return fakeError("simulated FGA write failure")
	}
	f.ownerships[userID+"|"+tenantID] = true
	return nil
}

func (f *FakeClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkCallCount++
	if f.failNextChecks > 0 {
		f.failNextChecks--
		return false, fakeError("simulated FGA check failure")
	}
	return f.memberships[userID+"|"+tenantID], nil
}

// fakeError is a sentinel error type for FakeClient failure injection.
type fakeError string

func (e fakeError) Error() string { return string(e) }
