package auth

import "context"

// NamedVerifier pairs a TokenVerifier with the issuer it verifies for, so
// a successful verification can be attributed to one issuer.
type NamedVerifier struct {
	// Issuer is the label recorded when this verifier accepts a token.
	// It is a metric label, so it must stay low-cardinality and stable —
	// "zitadel" / "gip", never a URL or a tenant.
	Issuer   string
	Verifier TokenVerifier
}

// CompositeVerifier accepts a token issued by ANY of several issuers,
// trying them in order and returning the first success.
//
// # Why this exists
//
// Mobile admin auth is mid-migration from GIP to Zitadel (#686). The
// incumbent wiring selects exactly ONE verifier from ZITADEL_ENABLED, so
// the day that flag flips every already-installed app stops working at
// once: its GIP tokens are no longer accepted and there is no version of
// the client in the field that holds a Zitadel token. That makes the
// mobile cutover a flag day, which for a store-app release is
// unshippable — old installs cannot be forced to update.
//
// Accepting both issuers for the duration of the drain turns that flag
// day into a rollout: new app versions authenticate against Zitadel,
// existing installs keep working on GIP, and the two coexist until the
// old versions age out.
//
// # The observability is the point, not a side benefit
//
// Retiring GIP (#708) is blocked on a question nobody can currently
// answer: is anything still authenticating against it? This type is the
// only place that knows which issuer a given token came from, so it is
// where that question becomes measurable. onVerified fires with the
// winning issuer on every successful verification; wired to a counter,
// "no gip in N days" is what makes the deletion decision evidence rather
// than a guess.
//
// # Ordering
//
// A token verifies against at most one issuer — they have different
// signing keys — so order changes latency, never the outcome. Put the
// issuer expected to carry most traffic first; every miss ahead of the
// winner is a wasted (cached-JWKS, but still non-zero) verification on
// the request path.
type CompositeVerifier struct {
	verifiers  []NamedVerifier
	onVerified func(issuer string)
}

// NewCompositeVerifier returns a verifier that tries each entry in order.
// Entries with a nil Verifier are skipped, so callers may build the list
// conditionally (Zitadel may be unconfigured) without filtering first.
// onVerified may be nil.
func NewCompositeVerifier(verifiers []NamedVerifier, onVerified func(issuer string)) *CompositeVerifier {
	return &CompositeVerifier{verifiers: verifiers, onVerified: onVerified}
}

// Verify returns the claims from the first issuer that accepts the token.
//
// When every issuer rejects it, the FIRST error is returned rather than
// the last: the first entry is the primary issuer, so its error is the
// one that describes the expected failure, and surfacing the last would
// report a legacy issuer's complaint about a token never meant for it.
// The caller (GIPBearerAuth) turns any error into an identical 401
// regardless, so this only affects what an operator reads in a log.
//
// A failed verification records NO issuer: attributing a rejected token
// to an issuer would inflate that issuer's traffic with tokens it never
// minted, and this counter's whole job is to say when GIP has stopped
// being used.
func (c *CompositeVerifier) Verify(ctx context.Context, idToken string) (*TokenClaims, error) {
	var firstErr error
	for _, nv := range c.verifiers {
		if nv.Verifier == nil {
			continue
		}
		claims, err := nv.Verifier.Verify(ctx, idToken)
		if err == nil {
			if c.onVerified != nil {
				c.onVerified(nv.Issuer)
			}
			return claims, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		// No usable verifier was configured at all. Fail closed: an empty
		// composite must never read as "the token is fine".
		return nil, ErrInvalidToken
	}
	return nil, firstErr
}
