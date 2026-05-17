package gipkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/api/apikeys/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// Client maintains the merchant-domain allowlist on the GIP browser
// API key. Implementations must be idempotent — AddDomain on an
// already-listed pattern is a no-op, and RemoveDomain on an unknown
// pattern is a no-op.
type Client interface {
	AddDomain(ctx context.Context, domain string) error
	RemoveDomain(ctx context.Context, domain string) error
}

// Noop is a Client that does nothing. main.go uses it when the
// GIP_WEB_API_KEY_RESOURCE_NAME env is empty so the rest of the code
// can call the client without nil-checking.
type Noop struct{}

func (Noop) AddDomain(_ context.Context, _ string) error    { return nil }
func (Noop) RemoveDomain(_ context.Context, _ string) error { return nil }

// googleClient drives Google's API Keys v2 service. Reads the key,
// patches its browserKeyRestrictions.allowedReferrers, and retries on
// the optimistic-concurrency 409 a concurrent verify could provoke.
type googleClient struct {
	svc          *apikeys.Service
	keyResource  string // projects/<num>/locations/global/keys/<uid>
	maxRetry     int
	logger       *slog.Logger
}

// New constructs a Client that targets keyResource (the full
// projects/.../locations/global/keys/... name from `gcloud services
// api-keys list`). Uses application-default credentials — on GKE this
// is the workload-identity service account.
func New(ctx context.Context, keyResource string, logger *slog.Logger, opts ...option.ClientOption) (Client, error) {
	if strings.TrimSpace(keyResource) == "" {
		return nil, errors.New("gipkey: keyResource is empty")
	}
	svc, err := apikeys.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gipkey: new apikeys service: %w", err)
	}
	return &googleClient{
		svc:         svc,
		keyResource: keyResource,
		maxRetry:    3,
		logger:      logger,
	}, nil
}

// AddDomain appends the apex + wildcard referrer for domain when not
// already present.
func (c *googleClient) AddDomain(ctx context.Context, domain string) error {
	patterns := DeriveReferrers(domain)
	if len(patterns) == 0 {
		return fmt.Errorf("gipkey: domain %q produced no allowlist patterns", domain)
	}
	return c.mutate(ctx, func(existing []string) []string {
		return mergePatterns(existing, patterns)
	})
}

// RemoveDomain strips the apex + wildcard referrer for domain. Safe
// to call when the patterns aren't present.
func (c *googleClient) RemoveDomain(ctx context.Context, domain string) error {
	patterns := DeriveReferrers(domain)
	if len(patterns) == 0 {
		return fmt.Errorf("gipkey: domain %q produced no allowlist patterns", domain)
	}
	return c.mutate(ctx, func(existing []string) []string {
		return removePatterns(existing, patterns)
	})
}

// mutate does the read-modify-write dance with concurrency retry. It
// reads the current key (including its etag), computes the new
// allowlist via xform, and Patches the key with updateMask scoped to
// just the allowlist field so unrelated restrictions stay untouched.
func (c *googleClient) mutate(ctx context.Context, xform func([]string) []string) error {
	var lastErr error
	for attempt := 0; attempt < c.maxRetry; attempt++ {
		key, err := c.svc.Projects.Locations.Keys.Get(c.keyResource).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("gipkey: get key: %w", err)
		}
		// A key with no restrictions at all is a legitimate state — start
		// from an empty list and let the Patch establish the structure.
		var current []string
		if key.Restrictions != nil && key.Restrictions.BrowserKeyRestrictions != nil {
			current = key.Restrictions.BrowserKeyRestrictions.AllowedReferrers
		}
		next := xform(current)
		// Skip the write entirely when nothing changes — both faster
		// and audit-friendlier. Order-insensitive compare.
		if sameSet(current, next) {
			if c.logger != nil {
				c.logger.Info("gipkey: allowlist already in desired state",
					"key", c.keyResource, "size", len(next))
			}
			return nil
		}
		// Preserve other restriction blocks (apiTargets, server-key, etc.)
		// by mutating the existing struct rather than building a fresh one.
		restrictions := key.Restrictions
		if restrictions == nil {
			restrictions = &apikeys.V2Restrictions{}
		}
		restrictions.BrowserKeyRestrictions = &apikeys.V2BrowserKeyRestrictions{
			AllowedReferrers: next,
		}
		patch := &apikeys.V2Key{
			Etag:         key.Etag,
			Restrictions: restrictions,
		}
		_, err = c.svc.Projects.Locations.Keys.Patch(c.keyResource, patch).
			UpdateMask("restrictions.browser_key_restrictions.allowed_referrers").
			Context(ctx).Do()
		if err == nil {
			if c.logger != nil {
				c.logger.Info("gipkey: allowlist patched",
					"key", c.keyResource, "size", len(next))
			}
			return nil
		}
		// 409 ABORTED / 412 PRECONDITION_FAILED → another writer beat us.
		// Re-read and try again. Anything else is fatal.
		var gErr *googleapi.Error
		if errors.As(err, &gErr) && (gErr.Code == 409 || gErr.Code == 412) {
			lastErr = err
			if c.logger != nil {
				c.logger.Warn("gipkey: patch conflict, retrying",
					"key", c.keyResource, "attempt", attempt+1, "err", err)
			}
			continue
		}
		return fmt.Errorf("gipkey: patch key: %w", err)
	}
	return fmt.Errorf("gipkey: patch key: exhausted retries: %w", lastErr)
}

// mergePatterns returns existing with want appended, deduped, and
// preserving the original order of existing entries — so the human-
// curated ordering in the console isn't reshuffled by every verify.
func mergePatterns(existing, want []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(want))
	out := make([]string, 0, len(existing)+len(want))
	for _, p := range existing {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range want {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// removePatterns returns existing without any entry that exact-matches
// one of want. Preserves order of the remaining entries.
func removePatterns(existing, want []string) []string {
	drop := make(map[string]struct{}, len(want))
	for _, p := range want {
		drop[p] = struct{}{}
	}
	out := make([]string, 0, len(existing))
	for _, p := range existing {
		if _, skip := drop[p]; skip {
			continue
		}
		out = append(out, p)
	}
	return out
}

// sameSet treats the inputs as sets and returns true when every member
// of a is also in b and vice versa.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, p := range a {
		set[p] = struct{}{}
	}
	for _, p := range b {
		if _, ok := set[p]; !ok {
			return false
		}
	}
	return true
}
