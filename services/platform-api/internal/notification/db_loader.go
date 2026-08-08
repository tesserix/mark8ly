package notification

// db_loader.go — DB-backed template loader with embedded fallback.
//
// Templates are stored in `email_templates` (see migration 0013). The
// loader tries DB first via a per-process TTL cache; on miss / DB error
// it falls back to the embedded template compiled into the binary
// (templates.go's `templatesFS`). Operators edit templates via
// tesserix-home, which UPSERTs into this table over the cross-DB grant
// and then pings POST /internal/templates/refresh to evict the cache so
// the change is live within seconds rather than waiting for TTL.
//
// Subjects are themselves Go templates (welcome / invitation interpolate
// the tenant name) so the same render path applies to all three slots.

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
// where the refresh ping is missed (network glitch, replicas not yet
// joined, etc.).
const CacheTTL = 5 * time.Minute

// dbTemplate is the row shape of email_templates. Kept package-private
// because callers consume Email values, not template rows.
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
	missingDB bool // true when the row was confirmed absent — still cached
	// briefly to avoid hot-looping on a missing key, but with a short
	// TTL so seeding flips us to the DB row quickly.
}

// Loader is the package-level entry point for loading templates. It
// wraps a *gorm.DB and a per-key cache; callers should construct one at
// service start and reuse it for the lifetime of the process.
type Loader struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]cacheEntry
	// negativeCacheTTL is shorter than CacheTTL so a "missing in DB"
	// cache entry doesn't outlive the seed bootstrap by much.
	negativeCacheTTL time.Duration
}

// NewLoader returns a Loader bound to the given GORM connection.
// db may be nil — in which case the loader always falls back to embedded
// templates and SeedFromEmbedded becomes a no-op. This keeps unit tests
// that don't want a real DB simple, and lets the service start cleanly
// when the DB connection is unavailable at boot time.
func NewLoader(db *gorm.DB) *Loader {
	return &Loader{
		db:               db,
		cache:            make(map[string]cacheEntry),
		negativeCacheTTL: 30 * time.Second,
	}
}

// Invalidate clears the cache entry for a given key so the next Render
// call re-queries the DB. Called by POST /internal/templates/refresh
// after tesserix-home authors a change.
func (l *Loader) Invalidate(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cache, key)
}

// InvalidateAll clears every cached template. Useful in tests and when
// the operator wants to force a full reload.
func (l *Loader) InvalidateAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = make(map[string]cacheEntry)
}

// load returns a dbTemplate for `key`, or (zero, false, err) when it
// cannot be served from DB (caller should fall back to embedded).
//
// "Cannot be served" means: the loader has no DB, the row is missing,
// the row has status='draft' (drafts must not affect live sends), or
// the DB query errored. In all of those cases we want the embedded
// fallback path to kick in — emails must keep flowing.
func (l *Loader) load(ctx context.Context, key string) (dbTemplate, bool, error) {
	if l == nil || l.db == nil {
		return dbTemplate{}, false, nil
	}
	now := time.Now()

	// Fast path: cached + fresh.
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

	// Slow path: query DB.
	var row dbTemplate
	err := l.db.WithContext(ctx).
		Raw(`SELECT key, subject, html_body, text_body, status, version, updated_at
		     FROM email_templates
		     WHERE key = ? AND status = 'published'`, key).
		Scan(&row).Error
	if err != nil {
		// Don't poison the cache with an error — try DB again next call.
		return dbTemplate{}, false, fmt.Errorf("notification: db load %q: %w", key, err)
	}
	if row.Key == "" {
		// Missing row → cache the absence briefly and fall back.
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

// Render builds an Email from a template key and a Vars value. It tries
// the DB-loaded template first and falls back to the embedded version
// on miss / DB error. The returned Email is ready to hand to a Sender.
//
// from is the SMTP From address. to is the recipient. tenantID is
// optional and is forwarded to the SendGrid client for engagement
// attribution (custom_args).
func (l *Loader) Render(ctx context.Context, key, to, from, tenantID string, vars any) (Email, error) {
	if to == "" {
		return Email{}, ErrNoRecipient
	}

	// 1. Try DB.
	row, ok, err := l.load(ctx, key)
	if err == nil && ok {
		subject, sErr := renderInline("subject:"+key, row.Subject, vars)
		htmlBody, hErr := renderInline("html:"+key, row.HTMLBody, vars)
		textBody, tErr := renderInline("text:"+key, row.TextBody, vars)
		if sErr == nil && hErr == nil && tErr == nil {
			return Email{
				To:       to,
				From:     from,
				Subject:  subject,
				HTMLBody: htmlBody,
				TextBody: textBody,
				TenantID: tenantID,
			}, nil
		}
		// DB row parsed but failed to render — almost certainly a
		// template syntax error introduced by an operator edit. Don't
		// silently send the embedded version with possibly-stale copy;
		// surface the error so the call site can log + retry. The
		// admin UI validates templates at write time so this should be
		// rare in practice.
		return Email{}, fmt.Errorf("notification: db render %q: subject=%v html=%v text=%v", key, sErr, hErr, tErr)
	}

	// 2. Fall back to embedded.
	return renderEmbedded(key, to, from, tenantID, vars)
}

// renderInline parses + executes a Go template against vars. Uses
// html/template for HTML keys (auto-escapes) and text/template for
// subject + plain-text bodies. The `name` prefix encodes which slot:
// "html:" → HTML, anything else → text.
func renderInline(name, body string, vars any) (string, error) {
	var buf bytes.Buffer
	if isHTMLKey(name) {
		t, err := htmltpl.New(name).Parse(body)
		if err != nil {
			return "", err
		}
		if err := t.Execute(&buf, vars); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	t, err := texttpl.New(name).Parse(body)
	if err != nil {
		return "", err
	}
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func isHTMLKey(name string) bool {
	const prefix = "html:"
	return len(name) >= len(prefix) && name[:len(prefix)] == prefix
}

// renderEmbedded mirrors renderHTML/renderText from templates.go but is
// keyed by template `key` rather than path. We don't import the
// existing renderHTML/renderText helpers because they take filesystem
// paths; the embedded fallback per-key dispatch lives here so the
// loader API surface is uniform.
func renderEmbedded(key, to, from, tenantID string, vars any) (Email, error) {
	switch key {
	case "welcome":
		v, ok := vars.(WelcomeVars)
		if !ok {
			return Email{}, fmt.Errorf("notification: welcome render expected WelcomeVars, got %T", vars)
		}
		msg, err := RenderWelcome(to, from, v)
		if err != nil {
			return Email{}, err
		}
		msg.TenantID = tenantID
		return msg, nil
	case "email_verification":
		v, ok := vars.(EmailVerificationVars)
		if !ok {
			return Email{}, fmt.Errorf("notification: email_verification render expected EmailVerificationVars, got %T", vars)
		}
		msg, err := RenderEmailVerification(to, from, v)
		if err != nil {
			return Email{}, err
		}
		msg.TenantID = tenantID
		return msg, nil
	case "invitation":
		v, ok := vars.(InvitationVars)
		if !ok {
			return Email{}, fmt.Errorf("notification: invitation render expected InvitationVars, got %T", vars)
		}
		msg, err := RenderInvitation(to, from, v)
		if err != nil {
			return Email{}, err
		}
		msg.TenantID = tenantID
		return msg, nil
	case "password_reset":
		v, ok := vars.(PasswordResetVars)
		if !ok {
			return Email{}, fmt.Errorf("notification: password_reset render expected PasswordResetVars, got %T", vars)
		}
		msg, err := RenderPasswordReset(to, from, v)
		if err != nil {
			return Email{}, err
		}
		msg.TenantID = tenantID
		return msg, nil
	case "login_otp":
		v, ok := vars.(LoginOTPVars)
		if !ok {
			return Email{}, fmt.Errorf("notification: login_otp render expected LoginOTPVars, got %T", vars)
		}
		msg, err := RenderLoginOTP(to, from, v)
		if err != nil {
			return Email{}, err
		}
		msg.TenantID = tenantID
		return msg, nil
	case "new_device_login":
		v, ok := vars.(NewDeviceLoginVars)
		if !ok {
			return Email{}, fmt.Errorf("notification: new_device_login render expected NewDeviceLoginVars, got %T", vars)
		}
		msg, err := RenderNewDeviceLogin(to, from, v)
		if err != nil {
			return Email{}, err
		}
		msg.TenantID = tenantID
		return msg, nil
	}
	return Email{}, fmt.Errorf("notification: no embedded template for key %q", key)
}

// SeedFromEmbedded inserts the embedded templates into email_templates
// for any key that doesn't already have a row. Idempotent — safe to
// run on every boot. Used at service start so that a freshly-migrated
// DB is immediately byte-identical to the embedded path.
//
// Returns nil when db is nil so unit tests without a DB don't fail.
func (l *Loader) SeedFromEmbedded(ctx context.Context) error {
	if l == nil || l.db == nil {
		return nil
	}
	for _, s := range embeddedSeed() {
		err := l.db.WithContext(ctx).Exec(`
			INSERT INTO email_templates
				(key, subject, html_body, text_body, variables, status, version, updated_by)
			VALUES (?, ?, ?, ?, ?::jsonb, 'published', 1, 'embedded-seed')
			ON CONFLICT (key) DO NOTHING
		`, s.key, s.subject, s.html, s.text, s.varsJSON).Error
		if err != nil {
			return fmt.Errorf("notification: seed %q: %w", s.key, err)
		}
	}
	return nil
}

type seedRow struct {
	key      string
	subject  string
	html     string
	text     string
	varsJSON string
}

func embeddedSeed() []seedRow {
	read := func(path string) string {
		raw, err := templatesFS.ReadFile(path)
		if err != nil {
			// embedded files are guaranteed by the //go:embed directive
			// and cannot be missing at runtime.
			panic(err)
		}
		return string(raw)
	}
	return []seedRow{
		{
			key:      "welcome",
			subject:  "Welcome to Mark8ly, {{.BusinessName}}",
			html:     read("templates/welcome.html"),
			text:     read("templates/welcome.txt"),
			varsJSON: `[{"name":"BusinessName","type":"string","required":true},{"name":"OwnerName","type":"string","required":false},{"name":"AdminURL","type":"string","required":true},{"name":"StorefrontURL","type":"string","required":true},{"name":"SupportEmail","type":"string","required":true}]`,
		},
		{
			key:      "email_verification",
			subject:  "Verify your email — Mark8ly",
			html:     read("templates/email_verification.html"),
			text:     read("templates/email_verification.txt"),
			varsJSON: `[{"name":"BusinessName","type":"string","required":false},{"name":"VerifyURL","type":"string","required":true},{"name":"ExpiresIn","type":"string","required":true},{"name":"SupportEmail","type":"string","required":true}]`,
		},
		{
			key:      "invitation",
			subject:  "You've been invited to join {{.TenantName}} on Mark8ly",
			html:     read("templates/invitation.html"),
			text:     read("templates/invitation.txt"),
			varsJSON: `[{"name":"TenantName","type":"string","required":true},{"name":"Role","type":"string","required":true},{"name":"Inviter","type":"string","required":false},{"name":"AcceptURL","type":"string","required":true},{"name":"ExpiresIn","type":"string","required":true},{"name":"SupportEmail","type":"string","required":true}]`,
		},
		{
			key:      "password_reset",
			subject:  "Reset your Mark8ly password",
			html:     read("templates/password_reset.html"),
			text:     read("templates/password_reset.txt"),
			varsJSON: `[{"name":"ResetURL","type":"string","required":true},{"name":"ExpiresIn","type":"string","required":true},{"name":"SupportEmail","type":"string","required":true}]`,
		},
		{
			key:      "login_otp",
			subject:  "{{.Code}} is your Mark8ly sign-in code",
			html:     read("templates/login_otp.html"),
			text:     read("templates/login_otp.txt"),
			varsJSON: `[{"name":"Code","type":"string","required":true},{"name":"ExpiresIn","type":"string","required":true},{"name":"SupportEmail","type":"string","required":true}]`,
		},
		{
			key:      "new_device_login",
			subject:  "New sign-in to your Mark8ly account",
			html:     read("templates/new_device_login.html"),
			text:     read("templates/new_device_login.txt"),
			varsJSON: `[{"name":"Device","type":"string","required":true},{"name":"CountryName","type":"string","required":true},{"name":"IPAddress","type":"string","required":false},{"name":"At","type":"string","required":true},{"name":"SecureURL","type":"string","required":true},{"name":"SupportEmail","type":"string","required":true}]`,
		},
	}
}

// errLoaderClosed is reserved for a future shutdown signal. Declared
// here so callers can errors.Is against it once the loader gets a
// Close() method.
var errLoaderClosed = errors.New("notification: loader closed")
var _ = errLoaderClosed
