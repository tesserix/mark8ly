// Package emailtemplates is the marketplace-api side of the runtime
// templates registry. Mirrors the platform-api notification loader but
// lives in its own package because it's shared by orderdoc, giftcard,
// and campaign — none of which are a natural home for the loader.
//
// Templates are stored in the email_templates table (migration 000085).
// Authored from tesserix-home over the cross-DB grant on
// mark8ly_platform_admin. Read on every send via a per-process TTL
// cache; on miss / DB error we fall back to the embedded template
// compiled into the binary so emails keep flowing during outages or
// before the seed migration has run.
//
// Subject + heading + lede + CTA copy now live in the template files
// themselves (with {{if}} blocks for state-driven variations) rather
// than being computed in Go, so an operator editing a template can
// change the wording without a code change. The Go side just hands
// over a typed Vars struct with the business state (RefundAmount,
// OrderNumber, IsFullRefund, etc.).

package emailtemplates

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmltpl "html/template"
	"sync"
	texttpl "text/template"
	"time"

	"gorm.io/gorm"
)

// CacheTTL is the maximum age of a cached template before re-querying
// the DB. The /internal/templates/refresh endpoint evicts immediately
// for an authored change; this TTL is the upper bound for the case
// where the refresh ping is missed.
const CacheTTL = 5 * time.Minute

// Rendered is the result of rendering a template — subject + both
// body shapes. Callers compose the SendGrid envelope from this.
type Rendered struct {
	Subject  string
	HTMLBody string
	TextBody string
}

// EmbeddedFallback is what the loader falls back to when the DB row
// is missing or invalid. Every package that sends via this loader
// registers its embedded templates at boot.
type EmbeddedFallback struct {
	Subject  string // raw template string (Go text/template syntax)
	HTMLBody string // raw template string (Go html/template syntax)
	TextBody string // raw template string (Go text/template syntax)
}

type dbTemplate struct {
	Key       string
	Subject   string
	HTMLBody  string
	TextBody  string
	Status    string
	Version   int
	UpdatedAt time.Time
}

type cacheEntry struct {
	tpl       dbTemplate
	storedAt  time.Time
	missingDB bool
}

// Loader serves rendered templates with embedded fallback. One per
// process; safe for concurrent use.
type Loader struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]cacheEntry
	// fallbacks is populated by Register before Render is called.
	// Reads are sync.RWMutex-guarded so concurrent registers + renders
	// (e.g. wave of sends during a hot start) don't race.
	fallbackMu sync.RWMutex
	fallbacks  map[string]EmbeddedFallback
	// negativeCacheTTL is shorter than CacheTTL so a "missing in DB"
	// cache entry doesn't outlive the seed bootstrap by much.
	negativeCacheTTL time.Duration
}

// NewLoader returns a Loader bound to a GORM connection. db may be
// nil — every Render call will fall back to embedded.
func NewLoader(db *gorm.DB) *Loader {
	return &Loader{
		db:               db,
		cache:            make(map[string]cacheEntry),
		fallbacks:        make(map[string]EmbeddedFallback),
		negativeCacheTTL: 30 * time.Second,
	}
}

// Register binds an embedded fallback for `key`. Call from each
// caller's init or from a registration helper before serving traffic.
// Subsequent calls for the same key overwrite (last writer wins) so
// hot-reload during tests works.
func (l *Loader) Register(key string, fb EmbeddedFallback) {
	l.fallbackMu.Lock()
	defer l.fallbackMu.Unlock()
	l.fallbacks[key] = fb
}

// Invalidate clears the cache entry for a given key so the next
// Render call re-queries the DB. Called by /internal/templates/refresh.
func (l *Loader) Invalidate(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cache, key)
}

// InvalidateAll clears every cached template.
func (l *Loader) InvalidateAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = make(map[string]cacheEntry)
}

// load returns a dbTemplate for key, or (zero, false, err) if the
// template cannot be served from DB (caller should fall back).
func (l *Loader) load(ctx context.Context, key string) (dbTemplate, bool, error) {
	if l == nil || l.db == nil {
		return dbTemplate{}, false, nil
	}
	now := time.Now()

	l.mu.RLock()
	entry, ok := l.cache[key]
	l.mu.RUnlock()
	if ok {
		ttl := CacheTTL
		if entry.missingDB {
			ttl = l.negativeCacheTTL
		}
		if now.Sub(entry.storedAt) < ttl {
			if entry.missingDB {
				return dbTemplate{}, false, nil
			}
			return entry.tpl, true, nil
		}
	}

	var row dbTemplate
	err := l.db.WithContext(ctx).
		Raw(`SELECT key, subject, html_body, text_body, status, version, updated_at
		     FROM email_templates
		     WHERE key = ? AND status = 'published'`, key).
		Scan(&row).Error
	if err != nil {
		return dbTemplate{}, false, fmt.Errorf("emailtemplates: db load %q: %w", key, err)
	}
	if row.Key == "" {
		l.mu.Lock()
		l.cache[key] = cacheEntry{storedAt: now, missingDB: true}
		l.mu.Unlock()
		return dbTemplate{}, false, nil
	}
	l.mu.Lock()
	l.cache[key] = cacheEntry{tpl: row, storedAt: now}
	l.mu.Unlock()
	return row, true, nil
}

// Render returns the subject + html + text bodies for a key, with
// vars interpolated. Tries the DB first; on miss / DB error falls
// back to the embedded fallback registered under the same key.
//
// Returns ErrUnknownKey if neither the DB nor the embedded fallback
// has the key — defensive: never silently send a blank email.
func (l *Loader) Render(ctx context.Context, key string, vars any) (Rendered, error) {
	row, ok, _ := l.load(ctx, key)
	if ok {
		r, err := renderTriple(key, row.Subject, row.HTMLBody, row.TextBody, vars)
		if err == nil {
			return r, nil
		}
		// DB row exists but failed to render — surface the error rather
		// than falling back silently. Operators set status='draft' for
		// in-progress edits; published rows are expected to render.
		return Rendered{}, fmt.Errorf("emailtemplates: db render %q: %w", key, err)
	}

	l.fallbackMu.RLock()
	fb, ok := l.fallbacks[key]
	l.fallbackMu.RUnlock()
	if !ok {
		return Rendered{}, fmt.Errorf("emailtemplates: %w: %s", ErrUnknownKey, key)
	}
	return renderTriple(key, fb.Subject, fb.HTMLBody, fb.TextBody, vars)
}

// ErrUnknownKey is returned when neither DB nor embedded fallback has
// the requested template key. Wrapped, so callers can errors.Is.
var ErrUnknownKey = errors.New("unknown template key")

func renderTriple(key, subjectTpl, htmlTpl, textTpl string, vars any) (Rendered, error) {
	subject, err := renderText("subject:"+key, subjectTpl, vars)
	if err != nil {
		return Rendered{}, fmt.Errorf("subject: %w", err)
	}
	html, err := renderHTML("html:"+key, htmlTpl, vars)
	if err != nil {
		return Rendered{}, fmt.Errorf("html: %w", err)
	}
	text, err := renderText("text:"+key, textTpl, vars)
	if err != nil {
		return Rendered{}, fmt.Errorf("text: %w", err)
	}
	return Rendered{Subject: subject, HTMLBody: html, TextBody: text}, nil
}

func renderHTML(name, body string, vars any) (string, error) {
	t, err := htmltpl.New(name).Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderText(name, body string, vars any) (string, error) {
	t, err := texttpl.New(name).Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// SeedFromEmbedded inserts every registered fallback into the
// email_templates table for any key without a row. Idempotent —
// safe to run on every boot. Returns nil when db is nil.
func (l *Loader) SeedFromEmbedded(ctx context.Context) error {
	if l == nil || l.db == nil {
		return nil
	}
	l.fallbackMu.RLock()
	keys := make([]string, 0, len(l.fallbacks))
	for k := range l.fallbacks {
		keys = append(keys, k)
	}
	l.fallbackMu.RUnlock()

	for _, k := range keys {
		l.fallbackMu.RLock()
		fb := l.fallbacks[k]
		l.fallbackMu.RUnlock()
		err := l.db.WithContext(ctx).Exec(`
			INSERT INTO email_templates
				(key, subject, html_body, text_body, variables, status, version, updated_by)
			VALUES (?, ?, ?, ?, '[]'::jsonb, 'published', 1, 'embedded-seed')
			ON CONFLICT (key) DO NOTHING
		`, k, fb.Subject, fb.HTMLBody, fb.TextBody).Error
		if err != nil {
			return fmt.Errorf("emailtemplates: seed %q: %w", k, err)
		}
	}
	return nil
}
