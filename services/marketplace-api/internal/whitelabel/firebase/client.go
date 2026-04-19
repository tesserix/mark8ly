// Package firebase wraps the Firebase Admin API for the two teardown
// operations the §13.5 lifecycle needs:
//
//   - Day 60: ArchiveProject — flip IAM to read-only; the Firebase
//     project stays provisioned so merchants can download export data,
//     but no new writes are accepted.
//   - Day 90: DeleteProject — hard-delete the Firebase project (GCP
//     project deletion is reversible within 30 days by platform
//     admins; beyond that, data is purged).
//
// The real Client uses the `firebase.google.com/go/v4` Admin SDK + the
// GCP resource-manager API for the delete path. Until that dep is
// added, Client returns ErrNotWired on every call. FakeClient is
// used in all unit tests so the lifecycle advancer is fully testable
// without the real Firebase plumbing.
package firebase

import (
	"context"
	"errors"
)

// ClientAPI is what lifecycle/advancer depends on; tests use FakeClient.
type ClientAPI interface {
	// ArchiveProject flips the project to read-only IAM. Idempotent.
	ArchiveProject(ctx context.Context, firebaseProjectID string) error

	// DeleteProject hard-deletes the project. Idempotent on 404.
	DeleteProject(ctx context.Context, firebaseProjectID string) error
}

// ErrNotWired is returned by the real Client until the Firebase Admin
// SDK integration is fleshed out. Lifecycle callers should log and
// continue.
var ErrNotWired = errors.New("firebase: admin sdk integration not yet wired")

// Client is the production Firebase Admin client. Shape frozen; real
// impl lands in a follow-up.
type Client struct{}

// New constructs a Client. No config required today — the real impl
// will pick up ADC (Workload Identity) at call time.
func New() *Client { return &Client{} }

func (c *Client) ArchiveProject(ctx context.Context, firebaseProjectID string) error {
	return ErrNotWired
}

func (c *Client) DeleteProject(ctx context.Context, firebaseProjectID string) error {
	return ErrNotWired
}
