package notification

// admin_registry.go — the read-only surface internal/emailtemplates (the
// platform admin authoring package, mark8ly#720 Task 5) needs from this
// package's Loader, without that package importing anything about HTTP,
// gin, or the console's wire shapes.
//
// Everything here is DERIVED from what Render/renderEmbedded already use
// (embeddedSeed's rows, the Loader's own DB cache) — it adds no new state
// and no new source of truth for what "the embedded default" is.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// TemplateVariable is one entry of a template's declared variable schema,
// decoded from the JSON embedded in embeddedSeed. Mirrors marketplace-api's
// emailtemplates.Variable field for field.
type TemplateVariable struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// RegisteredKeys returns every key this service embeds a fallback for,
// sorted. Unlike marketplace-api's Loader.RegisteredKeys (which reflects a
// dynamic Register() call from each sending package), this list is FIXED:
// platform-api's six auth/onboarding templates are all defined in this one
// package and never grow at runtime, so the embedded seed list IS the
// registered-keys list — there is nothing else that could register one.
func RegisteredKeys() []string {
	seed := embeddedSeed()
	keys := make([]string, 0, len(seed))
	for _, s := range seed {
		keys = append(keys, s.key)
	}
	sort.Strings(keys)
	return keys
}

// EmbeddedDefault returns the raw (unrendered) subject/html/text and
// declared variable schema for key, as compiled into the binary. The bool
// reports whether key is one of the six embedded templates at all — a
// caller must not treat the zero value as "an empty template".
func EmbeddedDefault(key string) (subject, html, text string, variables []TemplateVariable, ok bool) {
	s, found := embeddedTemplate(key)
	if !found {
		return "", "", "", nil, false
	}
	return s.subject, s.html, s.text, decodeSeedVariables(s.varsJSON), true
}

func embeddedTemplate(key string) (seedRow, bool) {
	for _, s := range embeddedSeed() {
		if s.key == key {
			return s, true
		}
	}
	return seedRow{}, false
}

func decodeSeedVariables(raw string) []TemplateVariable {
	if raw == "" {
		return []TemplateVariable{}
	}
	var out []TemplateVariable
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []TemplateVariable{}
	}
	return out
}

// RenderPreview renders a template GENERICALLY for the platform admin
// authoring surface: unlike Render (used by every real send call site),
// it takes untyped vars — a map[string]any built from a signed console
// request body — rather than one of this package's typed Vars structs,
// and it produces a subject/html/text triple rather than a dispatch-ready
// Email (there is no recipient or tenant to attach yet; the caller is
// building a preview or a test-send payload, not sending mail through
// this Loader).
//
// It follows the SAME db-then-embedded precedence Render uses — a
// published DB row wins, the embedded default otherwise — through the
// SAME cache (l.load), so a preview reflects exactly what a real send
// would use and an admin edit that calls Invalidate is visible here too.
func (l *Loader) RenderPreview(ctx context.Context, key string, vars any) (subject, html, text string, err error) {
	if row, ok, lerr := l.load(ctx, key); lerr == nil && ok {
		return renderPreviewTriple(key, row.Subject, row.HTMLBody, row.TextBody, vars)
	}

	seed, found := embeddedTemplate(key)
	if !found {
		return "", "", "", fmt.Errorf("notification: no embedded template for key %q", key)
	}
	return renderPreviewTriple(key, seed.subject, seed.html, seed.text, vars)
}

func renderPreviewTriple(key, subjectTpl, htmlTpl, textTpl string, vars any) (subject, html, text string, err error) {
	subject, err = renderInline("subject:"+key, subjectTpl, vars)
	if err != nil {
		return "", "", "", fmt.Errorf("notification: preview render %q: %w", key, err)
	}
	html, err = renderInline("html:"+key, htmlTpl, vars)
	if err != nil {
		return "", "", "", fmt.Errorf("notification: preview render %q: %w", key, err)
	}
	text, err = renderInline("text:"+key, textTpl, vars)
	if err != nil {
		return "", "", "", fmt.Errorf("notification: preview render %q: %w", key, err)
	}
	return subject, html, text, nil
}
