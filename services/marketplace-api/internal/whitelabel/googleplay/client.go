// Package googleplay wraps the Google Play Android Publisher API for
// the two teardown operations the §13.5 lifecycle needs:
//
//   - Day 30: BlockDownloads — update the default track's rollout so no
//     new downloads from the production channel (equivalent to halting
//     distribution; existing installs keep working).
//   - Day 60: PullApp — remove the app listing via Android Publisher's
//     `edits.tracks.update` flipping state to unpublished.
//
// The real Client uses Google service-account OAuth2 (RS256 JWT →
// token exchange) plus the Android Publisher v3 REST API. Tests use
// FakeClient (no network, no SDK dep). Main wires the real Client
// behind a feature flag; until the full oauth2 + publisher plumbing
// lands, production calls return ErrNotWired — the lifecycle advancer
// treats this as a non-fatal "skip for this run" so day-30 can be
// partially applied (Apple ok, Google deferred) without stalling.
//
// Adding the `google.golang.org/api/androidpublisher` dep + full oauth2
// JWT exchange is a follow-up; flagging here keeps the advancer API
// frozen so that addition is internal-only.
package googleplay

import (
	"context"
	"errors"
)

// ClientAPI is what lifecycle/advancer depends on; tests use FakeClient.
type ClientAPI interface {
	// BlockDownloads halts new installs of packageName. Idempotent.
	BlockDownloads(ctx context.Context, packageName string) error

	// PullApp removes packageName's public listing. Idempotent.
	PullApp(ctx context.Context, packageName string) error
}

// ErrNotWired is returned by the real Client stub until the Android
// Publisher integration is fleshed out. Lifecycle callers should log
// and continue rather than fail the whole advance cycle on this.
var ErrNotWired = errors.New("googleplay: android publisher integration not yet wired")

// Credentials is the service-account JSON bundle read from appcreds
// at call time — never stored on the Client. See spec §18.9.
type Credentials struct {
	ServiceAccountJSON []byte
}

// Client is the production Android Publisher client. Constructing it
// is allowed but every method returns ErrNotWired until the real
// oauth2 + publisher plumbing lands. The shape is frozen now so the
// advancer + main wiring don't change when the implementation fills in.
type Client struct {
	credsFetcher func(ctx context.Context) (Credentials, error)
}

// Config groups Client construction params.
type Config struct {
	CredsFetcher func(ctx context.Context) (Credentials, error)
}

// New constructs a Client. CredsFetcher is required.
func New(cfg Config) (*Client, error) {
	if cfg.CredsFetcher == nil {
		return nil, errors.New("googleplay: Config.CredsFetcher is required")
	}
	return &Client{credsFetcher: cfg.CredsFetcher}, nil
}

// BlockDownloads is not yet wired — returns ErrNotWired. See package doc.
func (c *Client) BlockDownloads(ctx context.Context, packageName string) error {
	// Fetch creds to surface any appcreds-level failures early — real
	// impl will use them to sign the oauth2 JWT.
	if _, err := c.credsFetcher(ctx); err != nil {
		return err
	}
	return ErrNotWired
}

// PullApp is not yet wired — returns ErrNotWired. See package doc.
func (c *Client) PullApp(ctx context.Context, packageName string) error {
	if _, err := c.credsFetcher(ctx); err != nil {
		return err
	}
	return ErrNotWired
}
