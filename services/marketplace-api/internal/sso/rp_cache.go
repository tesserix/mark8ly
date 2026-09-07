package sso

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

// rp_cache.go — per-tenant OIDC relying parties, built on demand (#820).
//
// # Why this is not a map built at startup
//
// SSOLoginHandler used to take `map[string]*OIDCRelyingParty`, and #820's own
// suggested scope was to "build OIDCRPs at startup". Both are wrong, for two
// reasons that only show up in production:
//
//  1. BuildOIDCRelyingParty performs OIDC DISCOVERY — an HTTP call to the
//     tenant's issuer. Building every tenant's party at boot makes every
//     customer's IdP a startup dependency of this service. One slow or
//     unreachable issuer delays the rollout; one that fails outright takes the
//     whole service down, for every tenant, over one tenant's misconfiguration.
//  2. A map built at boot cannot contain a tenant who configures SSO
//     afterwards. For a self-serve feature that means "works after the next
//     deploy", which is another way of saying it does not work.
//
// Building lazily inverts both: a broken issuer fails that tenant's login and
// nobody else's, and a config saved at 10:00 works at 10:00.

// OIDCSecretField is the key this cache reads out of the KV secret named by
// a config's client_secret_ref.
//
// "client_secret" rather than carriersecrets' "value" because these secrets
// are written by an operator against a documented contract, and a named field
// says what the blob is. A ref pointing at a secret without this field is a
// configuration error and is reported as one.
const OIDCSecretField = "client_secret"

// DefaultRelyingPartyTTL bounds how long a built relying party is reused.
//
// It is not about the client secret going stale — Invalidate covers a config
// change. It bounds the DISCOVERY document, which carries the IdP's signing
// keys and endpoints. An IdP that rotates keys or moves an endpoint is picked
// up within this window without an operator having to do anything.
const DefaultRelyingPartyTTL = 30 * time.Minute

// ErrNoClientSecret is returned when a config's client_secret_ref names a
// secret that does not exist, or one with no OIDCSecretField in it. Separate
// from a build failure so an operator is told which of the two happened —
// "your IdP is unreachable" and "your secret is missing" need different fixes.
var ErrNoClientSecret = errors.New("sso: client secret not found for config")

// SecretReader is the part of *bao.Client this cache needs.
type SecretReader interface {
	ReadSecret(ctx context.Context, path string) (map[string]string, int, error)
}

type rpEntry struct {
	rp      *OIDCRelyingParty
	builtAt time.Time
}

// RelyingPartyCache builds and reuses per-tenant OIDC relying parties.
//
// Safe for concurrent use. Concurrent misses for the SAME tenant are coalesced
// through singleflight, so a burst of logins after a config change performs one
// discovery call rather than one per request — which matters because the thing
// being coalesced is an outbound HTTP call to a customer's IdP.
type RelyingPartyCache struct {
	secrets SecretReader
	ttl     time.Duration
	now     func() time.Time

	mu      sync.Mutex
	entries map[uuid.UUID]rpEntry
	flight  singleflight.Group
}

// NewRelyingPartyCache constructs a cache. A zero ttl means
// DefaultRelyingPartyTTL; a nil secrets reader is allowed and makes every
// build fail with ErrNoClientSecret rather than panic.
func NewRelyingPartyCache(secrets SecretReader, ttl time.Duration) *RelyingPartyCache {
	if ttl <= 0 {
		ttl = DefaultRelyingPartyTTL
	}
	return &RelyingPartyCache{
		secrets: secrets,
		ttl:     ttl,
		now:     time.Now,
		entries: map[uuid.UUID]rpEntry{},
	}
}

// For returns the relying party for cfg's tenant, building it if there is no
// fresh one cached.
func (c *RelyingPartyCache) For(ctx context.Context, cfg *Config) (*OIDCRelyingParty, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: cfg is nil", ErrInvalidMetadata)
	}
	if rp, ok := c.cached(cfg.TenantID); ok {
		return rp, nil
	}

	built, err, _ := c.flight.Do(cfg.TenantID.String(), func() (any, error) {
		// Re-check inside the flight: the winner of a race has already
		// stored a party by the time the losers run, and rebuilding would
		// spend a second discovery call for nothing.
		if rp, ok := c.cached(cfg.TenantID); ok {
			return rp, nil
		}
		rp, err := c.build(ctx, cfg)
		if err != nil {
			return nil, err
		}
		c.store(cfg.TenantID, rp)
		return rp, nil
	})
	if err != nil {
		return nil, err
	}
	return built.(*OIDCRelyingParty), nil
}

// Invalidate drops any cached party for a tenant.
//
// Called when a config is written or deleted. Without it, a merchant
// correcting a wrong client secret would keep failing to log in for up to the
// TTL, with the console showing the corrected value — the kind of bug that
// gets diagnosed as "SSO is broken" rather than "the cache is warm".
func (c *RelyingPartyCache) Invalidate(tenantID uuid.UUID) {
	c.mu.Lock()
	delete(c.entries, tenantID)
	c.mu.Unlock()
}

func (c *RelyingPartyCache) cached(tenantID uuid.UUID) (*OIDCRelyingParty, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tenantID]
	if !ok || c.now().Sub(e.builtAt) > c.ttl {
		return nil, false
	}
	return e.rp, true
}

func (c *RelyingPartyCache) store(tenantID uuid.UUID, rp *OIDCRelyingParty) {
	c.mu.Lock()
	c.entries[tenantID] = rpEntry{rp: rp, builtAt: c.now()}
	c.mu.Unlock()
}

func (c *RelyingPartyCache) build(ctx context.Context, cfg *Config) (*OIDCRelyingParty, error) {
	secret, err := c.clientSecret(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return BuildOIDCRelyingParty(ctx, cfg, secret)
}

// clientSecret resolves cfg's client_secret_ref through the secret store.
//
// The plaintext never touches the database — metadata holds only the ref. The
// value is not cached separately either: it lives only inside the built
// relying party's oauth2.Config, for as long as that party is cached.
func (c *RelyingPartyCache) clientSecret(ctx context.Context, cfg *Config) (string, error) {
	ref, ok := nonEmpty(cfg.Metadata, OIDCKeyClientSecretRef)
	if !ok {
		return "", fmt.Errorf("%w: %s required", ErrInvalidMetadata, OIDCKeyClientSecretRef)
	}
	if c.secrets == nil {
		return "", fmt.Errorf("%w: no secret store configured", ErrNoClientSecret)
	}

	data, _, err := c.secrets.ReadSecret(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("%w: read %s: %w", ErrNoClientSecret, ref, err)
	}
	secret := data[OIDCSecretField]
	if secret == "" {
		return "", fmt.Errorf("%w: %s has no %q field", ErrNoClientSecret, ref, OIDCSecretField)
	}
	return secret, nil
}
