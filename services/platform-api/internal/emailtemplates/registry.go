package emailtemplates

// registry.go — the RegisteredKeys/Fallback/Invalidate/Render surface the
// platform admin handler needs (mirroring marketplace-api's
// emailtemplates.Loader interface field for field), backed by
// internal/notification.Loader rather than a second cache.
//
// This is a deliberate divergence from marketplace-api's shape: there,
// ONE emailtemplates.Loader IS both the send-path cache and the admin
// registry, in the same package. Here, internal/notification.Loader is
// already that send-path cache — built once in cmd/server/main.go and
// handed to every real send call site (onboarding, verification, auth) —
// and duplicating it as a second Loader in this package would mean an
// admin write's Invalidate call evicts a cache nothing sends through,
// leaving the real send path serving stale copy for up to CacheTTL.
// Wrapping the SAME *notification.Loader instance instead means
// Invalidate here is Invalidate there: an operator's edit takes effect on
// the next real send, not just the next admin preview.

import (
	"context"
	"errors"

	"github.com/mark8ly/platform-api/internal/notification"
)

// EmbeddedFallback is what a key falls back to when no DB row exists.
// Mirrors marketplace-api's emailtemplates.EmbeddedFallback: no declared
// variable schema is carried here even though platform-api's embedded
// templates DO have one (notification.EmbeddedDefault returns it) — see
// Fallback below for why it is dropped at this boundary.
type EmbeddedFallback struct {
	Subject  string
	HTMLBody string
	TextBody string
}

// Rendered is a rendered subject/html/text triple, ready for a test send
// or a preview.
type Rendered struct {
	Subject  string
	HTMLBody string
	TextBody string
}

// Registry adapts *notification.Loader to the shape
// platformadmin.EmailTemplateRegistry expects.
type Registry struct {
	loader *notification.Loader
}

// NewRegistry wraps loader — the SAME instance cmd/server/main.go builds
// for the real send path. loader may be nil; every method then degrades
// (RegisteredKeys/Fallback still work off the fixed embedded set, which
// needs no Loader; Invalidate is a no-op; Render errors).
func NewRegistry(loader *notification.Loader) *Registry {
	return &Registry{loader: loader}
}

// RegisteredKeys returns platform-api's six embedded template keys,
// sorted. Needs no Loader — see notification.RegisteredKeys.
func (r *Registry) RegisteredKeys() []string {
	return notification.RegisteredKeys()
}

// Fallback returns the embedded default for key. The bool reports whether
// key is one of the six embedded templates at all.
//
// The declared variable schema notification.EmbeddedDefault also returns
// is deliberately dropped here, matching marketplace-api's
// EmailTemplatesHandler.Get: for an UNAUTHORED key it always answers with
// an EMPTY variables array regardless of what the embedded default might
// otherwise declare (marketplace-api's own EmbeddedFallback type has no
// Variables field at all), because "an embedded default declares no
// variable schema — the registry holds three template strings and
// nothing else" per that handler's doc comment. Keeping this type
// variable-schema-free, rather than adding the field and then discarding
// it at the call site, is what keeps that behaviour from silently
// reappearing later.
func (r *Registry) Fallback(key string) (EmbeddedFallback, bool) {
	subject, html, text, _, ok := notification.EmbeddedDefault(key)
	if !ok {
		return EmbeddedFallback{}, false
	}
	return EmbeddedFallback{Subject: subject, HTMLBody: html, TextBody: text}, true
}

// Invalidate clears the SEND PATH's cache for key — see the package doc
// comment for why this must reach the real *notification.Loader and not a
// second, admin-only cache.
func (r *Registry) Invalidate(key string) {
	if r == nil || r.loader == nil {
		return
	}
	r.loader.Invalidate(key)
}

// Render renders key against vars (typically a map[string]any decoded
// from the console's JSON body) through the SAME loader the send path
// uses — a published DB row if there is one, the embedded default
// otherwise — so a test send exercises exactly what is live for the key.
func (r *Registry) Render(ctx context.Context, key string, vars any) (Rendered, error) {
	if r == nil || r.loader == nil {
		return Rendered{}, errors.New("emailtemplates: registry has no template loader configured")
	}
	subject, html, text, err := r.loader.RenderPreview(ctx, key, vars)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{Subject: subject, HTMLBody: html, TextBody: text}, nil
}
