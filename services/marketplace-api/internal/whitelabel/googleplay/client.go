// Package googleplay wraps the Google Play Android Publisher API for the
// §13.5 white-label app teardown sequence.
//
// ONE OF THE TWO STEPS IS POSSIBLE. Verified against Google's Android
// Publisher v3 reference on 2026-09-09:
//
//   - Day 30: BlockDownloads — IMPLEMENTED. The production track's
//     serving releases are set to `status: "halted"`, documented as "the
//     release's APKs will no longer be served to users. Users who already
//     have these APKs are unaffected." That is exactly the day-30 intent:
//     no new installs, existing installs keep working.
//
//   - Day 60: PullApp — NOT IMPLEMENTABLE, and not faked. Unpublishing a
//     Play listing has no Android Publisher API; it is a Play Console
//     action. PullApp therefore returns ErrUnpublishNotSupported on every
//     call. See the note on that error for the evidence, and for the
//     mechanisms that were considered and rejected.
//
// An earlier version of this comment claimed day 60 worked via
// `edits.tracks.update` "flipping state to unpublished". There is no such
// status — TrackRelease.status is one of `draft`, `inProgress`, `halted`,
// `completed` — and `edits.tracks.update` only manages releases within a
// track. The `applications` resource has exactly one method, `dataSafety`.
// The false mechanism is why nobody noticed day 60 was unbuildable.
//
// Auth is service-account OAuth2 (RS256 JWT → token exchange) via
// golang.org/x/oauth2/google, scoped to androidpublisher. Credentials are
// fetched per call and never stored on the Client (spec §18.9) so a key
// rotated or revoked between calls is picked up without a restart. Both
// `google.golang.org/api` and `golang.org/x/oauth2` are already direct
// dependencies; nothing was added for this.
//
// Tests use FakeClient (no network, no credentials). Main wires the real
// Client through lifecycle.NewGoogleTeardownFactory.
package googleplay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// ClientAPI is what lifecycle/advancer depends on; tests use FakeClient.
type ClientAPI interface {
	// BlockDownloads halts new installs of packageName. Idempotent.
	BlockDownloads(ctx context.Context, packageName string) error

	// PullApp removes packageName's public listing. Idempotent.
	//
	// The real Client CANNOT do this — see ErrUnpublishNotSupported. The
	// method stays on the interface because the lifecycle advancer's day-60
	// step is shaped around it and must record a refusal rather than
	// silently skip the step; removing it would make the gap invisible.
	PullApp(ctx context.Context, packageName string) error
}

// ErrUnauthorized reports that Google rejected the service-account
// credentials — a 401/403 from the API, a failed OAuth2 token exchange (a
// deleted or disabled key, or a key without the androidpublisher scope),
// or service-account JSON that will not parse.
//
// It is deliberately distinct from a transport failure, for the same
// reason apple.ErrUnauthorized is: "the merchant's key no longer works"
// needs an operator to re-onboard credentials, while "the request did not
// reach Google" resolves itself on the next tick. A caller that conflates
// them retries forever on the first and alarms on the second.
var ErrUnauthorized = errors.New("googleplay: Google rejected the service-account credentials")

// ErrUnpublishNotSupported reports that the day-60 "pull the listing"
// step cannot be performed through any API.
//
// EVIDENCE (Android Publisher v3 reference, checked 2026-09-09):
//
//   - `applications` has exactly one method: `dataSafety`. There is no
//     unpublish, delete, or delist.
//   - `edits.tracks` (create/get/list/patch/update) manages releases
//     WITHIN a track. TrackRelease.status is one of `draft`, `inProgress`,
//     `halted`, `completed` — there is no `unpublished`.
//   - Unpublishing an app is a Play Console action with no REST surface.
//
// Considered and rejected:
//
//   - `edits.listings.delete`/`deleteall` removes the localized store
//     LISTING TEXT from an edit. The app stays published; committing that
//     would break the listing's copy without delisting anything.
//   - `releases[].countryTargeting` is documented as settable only for
//     inProgress production releases and requires at least one country, so
//     it cannot express "nowhere" — and it would not remove the listing.
//   - Reusing BlockDownloads would be worse than failing: it would let the
//     row claim `pulled` for work that is only the day-30 halt.
//
// A cancelled merchant's listing staying visible is a fact the platform
// must state, not paper over. The lifecycle advancer records this reason
// beside the status rather than letting the status assert a pull.
var ErrUnpublishNotSupported = errors.New(
	"googleplay: unpublishing a Play listing has no Android Publisher API; it requires a Play Console action")

// ErrNotWired is retained only so callers written against the previous
// stub keep compiling. Nothing returns it any more: BlockDownloads is
// implemented, and PullApp returns ErrUnpublishNotSupported, which names a
// permanent constraint rather than pending work.
//
// Deprecated: match on ErrUnauthorized or ErrUnpublishNotSupported.
var ErrNotWired = errors.New("googleplay: android publisher integration not yet wired")

// Release statuses from TrackRelease.status. Only these four are valid;
// `unpublished` does not exist (see the package doc).
const (
	releaseStatusDraft      = "draft"
	releaseStatusInProgress = "inProgress"
	releaseStatusHalted     = "halted"
	releaseStatusCompleted  = "completed"
)

// productionTrack is the only track day-30 touches. Halting `internal`,
// `alpha` or `beta` would stop tester distribution without affecting the
// public listing, which is not what day 30 means.
const productionTrack = "production"

// Credentials is the service-account JSON bundle read from appcreds at
// call time — never stored on the Client. See spec §18.9.
type Credentials struct {
	ServiceAccountJSON []byte
}

// Client is the production Android Publisher client, backed by
// credentials fetched per-call from appcreds.Service. The CredsFetcher
// closure injects whatever lookup strategy the caller wants
// (tenant → appcreds.Load).
type Client struct {
	endpoint      string
	tokenURL      string
	http          *http.Client
	credsFetcher  func(ctx context.Context) (Credentials, error)
	tokenLifetime time.Duration
}

// Config groups Client construction params.
type Config struct {
	CredsFetcher func(ctx context.Context) (Credentials, error) // required

	// HTTP is the transport for both the token exchange and the API
	// calls. Default: http.Client{Timeout: 30s}.
	HTTP *http.Client

	// TokenLifetime is the assertion lifetime of the signed JWT. Google
	// caps service-account assertions at 1 hour. Default: 15 minutes —
	// short enough that a revoked key stops working promptly, long enough
	// that one teardown never re-signs mid-flight.
	TokenLifetime time.Duration

	// Endpoint overrides the Android Publisher base URL. TEST ONLY;
	// production leaves it empty to use googleapis.com.
	Endpoint string

	// TokenURL overrides the OAuth2 token endpoint. TEST ONLY, and
	// meaningful only alongside Endpoint: a test server that serves the
	// API must also serve the token exchange, or the client would sign a
	// real assertion and post it to Google.
	TokenURL string
}

// New constructs a production Client. CredsFetcher is required; tests
// should use FakeClient rather than a real Client with a stub fetcher,
// except where the point of the test is this file's own HTTP behaviour.
func New(cfg Config) (*Client, error) {
	if cfg.CredsFetcher == nil {
		return nil, errors.New("googleplay: Config.CredsFetcher is required")
	}
	h := cfg.HTTP
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	ttl := cfg.TokenLifetime
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	endpoint := cfg.Endpoint
	if endpoint != "" && !strings.HasSuffix(endpoint, "/") {
		// googleapi.ResolveRelative joins relative to the base path, so a
		// base without a trailing slash silently drops its last segment.
		endpoint += "/"
	}
	return &Client{
		endpoint:      endpoint,
		tokenURL:      cfg.TokenURL,
		http:          h,
		credsFetcher:  cfg.CredsFetcher,
		tokenLifetime: ttl,
	}, nil
}

// BlockDownloads halts new installs of packageName by setting every
// serving release on the production track to `halted`.
//
// The Android Publisher write model is an EDIT: insert an edit, mutate
// inside it, commit. Nothing is visible on Play until the commit.
//
//	edits.insert → edits.tracks.get(production) → edits.tracks.update
//	→ edits.commit
//
// Every return path after edits.insert abandons the edit with
// edits.delete, including the success-without-changes path. A leaked edit
// is a real side effect on the merchant's account: Play allows one open
// edit at a time, so an abandoned one blocks the merchant's own releases
// until it expires. A delete that itself fails is joined onto the returned
// error rather than dropped — that leak needs an operator, and this
// package has no logger to whisper it into.
//
// IDEMPOTENT, as the interface promises. If no release is in a serving
// state — already halted, only drafts, or no releases at all — the edit is
// abandoned and nil is returned. Committing an unchanged edit would be a
// pointless write against the merchant's account, and erroring would make
// the second tick of a retried teardown fail.
func (c *Client) BlockDownloads(ctx context.Context, packageName string) (err error) {
	if packageName == "" {
		return errors.New("googleplay: packageName is required")
	}
	svc, err := c.service(ctx)
	if err != nil {
		return err
	}
	edits := svc.Edits

	edit, err := edits.Insert(packageName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return classify("edits.insert", packageName, err)
	}

	committed := false
	defer func() {
		if committed {
			// The commit consumed the edit; deleting it now would 404.
			return
		}
		// Detached from ctx so a cancellation between insert and here does
		// not turn a recoverable failure into a leaked edit. The bound is
		// c.http's own Timeout.
		cleanupCtx := context.WithoutCancel(ctx)
		if delErr := edits.Delete(packageName, edit.Id).Context(cleanupCtx).Do(); delErr != nil {
			err = errors.Join(err, classify("edits.delete (abandoning edit "+edit.Id+")", packageName, delErr))
		}
	}()

	track, err := edits.Tracks.Get(packageName, edit.Id, productionTrack).Context(ctx).Do()
	if err != nil {
		return classify("edits.tracks.get("+productionTrack+")", packageName, err)
	}

	halted, changed := haltServingReleases(track)
	if !changed {
		return nil
	}

	if _, err := edits.Tracks.Update(packageName, edit.Id, productionTrack, halted).Context(ctx).Do(); err != nil {
		return classify("edits.tracks.update("+productionTrack+")", packageName, err)
	}
	if _, err := edits.Commit(packageName, edit.Id).Context(ctx).Do(); err != nil {
		return classify("edits.commit", packageName, err)
	}
	committed = true
	return nil
}

// PullApp always fails. Unpublishing a Play listing has no Android
// Publisher API — see ErrUnpublishNotSupported for the evidence and for
// the alternatives that were considered.
//
// It deliberately does NOT fetch credentials first: there is no request to
// authenticate, so a credential read here would add an audit entry
// implying an attempt was made. And it deliberately does not return nil:
// the caller's day-60 record must say the listing is still up.
func (c *Client) PullApp(_ context.Context, packageName string) error {
	return fmt.Errorf("%w (package %s)", ErrUnpublishNotSupported, packageName)
}

// haltServingReleases returns a COPY of track with every serving release
// switched to `halted`, plus whether anything changed. The fetched track
// is not mutated — the caller still needs the original to compare against,
// and an in-place edit would make "did anything change" unanswerable.
//
// `inProgress` (a staged rollout) and `completed` (fully rolled out) are
// the two states whose APKs are being served. `draft` is not served and
// `halted` is already the target, so both are copied through untouched:
// re-halting is what makes this method idempotent, and un-drafting a
// release the merchant staged but never shipped is not day 30's business.
func haltServingReleases(track *androidpublisher.Track) (*androidpublisher.Track, bool) {
	out := &androidpublisher.Track{Track: track.Track}
	changed := false
	for _, rel := range track.Releases {
		next := *rel
		switch rel.Status {
		case releaseStatusInProgress, releaseStatusCompleted:
			next.Status = releaseStatusHalted
			changed = true
		case releaseStatusDraft, releaseStatusHalted:
			// Already not serving.
		}
		out.Releases = append(out.Releases, &next)
	}
	return out, changed
}

// service builds an authenticated Android Publisher client for one call.
//
// Constructed per call, not once on the Client, because the credentials
// are: the CredsFetcher is the appcreds audit + metrics choke point, and a
// key revoked since the last teardown must fail this call rather than keep
// working off a cached token source.
func (c *Client) service(ctx context.Context) (*androidpublisher.Service, error) {
	creds, err := c.credsFetcher(ctx)
	if err != nil {
		return nil, fmt.Errorf("googleplay: fetch creds: %w", err)
	}
	if len(creds.ServiceAccountJSON) == 0 {
		return nil, fmt.Errorf("%w: service-account JSON is empty", ErrUnauthorized)
	}

	jwtCfg, err := google.JWTConfigFromJSON(creds.ServiceAccountJSON, androidpublisher.AndroidpublisherScope)
	if err != nil {
		// Unparseable or non-service-account JSON is a credential problem,
		// not a transport one — the same operator action fixes it as a
		// revoked key, so it carries the same sentinel.
		return nil, fmt.Errorf("%w: parse service-account JSON: %v", ErrUnauthorized, err)
	}
	jwtCfg.Expires = c.tokenLifetime
	if c.tokenURL != "" {
		jwtCfg.TokenURL = c.tokenURL
	}

	// oauth2.HTTPClient is how x/oauth2 is told which transport to use for
	// the token exchange; without it the exchange would bypass c.http and
	// its timeout.
	tokenCtx := context.WithValue(ctx, oauth2.HTTPClient, c.http)
	authed := &http.Client{
		Timeout: c.http.Timeout,
		Transport: &oauth2.Transport{
			Base:   c.http.Transport,
			Source: oauth2.ReuseTokenSource(nil, jwtCfg.TokenSource(tokenCtx)),
		},
	}

	opts := []option.ClientOption{option.WithHTTPClient(authed)}
	if c.endpoint != "" {
		opts = append(opts, option.WithEndpoint(c.endpoint))
	}
	svc, err := androidpublisher.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("googleplay: build android publisher service: %w", err)
	}
	return svc, nil
}

// classify turns an Android Publisher error into either ErrUnauthorized or
// a plain wrapped error, so callers can tell "these credentials are dead"
// from "this request did not get through".
func classify(op, packageName string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code == http.StatusUnauthorized || apiErr.Code == http.StatusForbidden {
			return fmt.Errorf("%w: %s(%s): status %d", ErrUnauthorized, op, packageName, apiErr.Code)
		}
		return fmt.Errorf("googleplay: %s(%s): status %d: %w", op, packageName, apiErr.Code, err)
	}

	// The token exchange rejected the assertion. x/oauth2 surfaces that as
	// *oauth2.RetrieveError, wrapped in *url.Error by http.Client — hence
	// errors.As rather than a type switch. A revoked, deleted or
	// wrong-scope key lands here, never as a googleapi.Error, because the
	// API request is never sent.
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return fmt.Errorf("%w: %s(%s): oauth2 token exchange: %v", ErrUnauthorized, op, packageName, retrieveErr)
	}

	return fmt.Errorf("googleplay: %s(%s): %w", op, packageName, err)
}
