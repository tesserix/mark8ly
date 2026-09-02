# Outbound Webhooks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a merchant register an HTTPS endpoint and receive signed, retried notifications when order, return, product and category events happen in their store.

**Architecture:** Two in-process loops beside the existing `outbox.Publisher`. A dispatcher reads `outbox_events` against its own cursor and fans each event out into one `webhook_deliveries` row per matching subscription. A delivery worker drains that table with `FOR UPDATE SKIP LOCKED`, POSTs to the merchant, and records the outcome. The existing outbox publisher is not touched.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, golang-migrate, testify. Admin UI is Next.js 16 / React 19 with `@tesserix/web` primitives.

**Spec:** `docs/superpowers/specs/2026-09-02-outbound-webhooks-design.md`

## Global Constraints

- **Migration number:** the next free is `000126`. Re-check `services/marketplace-api/migrations/` before writing — another PR may land first.
- **`ExpectedSchemaVersion`** in `services/marketplace-api/migrations.go` is currently `125` and must be bumped to match the migration you add.
- **Payloads are notify-and-fetch.** The delivery body carries event name, aggregate id, and timestamp. Never the entity. Never customer PII.
- **Delivery retention is 30 days on every plan** — deliberately NOT tied to `FeatureAuditRetentionDays`.
- **Available on every plan.** Do not add a `plangate` check for the webhook feature itself.
- **Never log a webhook URL's response body, the signing secret, or a subscriber email.**
- **Integration tests need `TEST_DATABASE_URL` and skip silently without it**, still printing `ok`. Always confirm tests actually ran.
- **Run the FULL integration suite before opening a PR**, not just touched packages: `go test -tags=integration -p 1 ./...`. `-p 1` matters — parallel packages exhaust the connection limit. The last two features here each tripped a guard in an untouched package that a narrow run missed.
- **Use a throwaway database**, never the shared one: `docker run -d --name m8-wh -e POSTGRES_PASSWORD=test -e POSTGRES_DB=marketplace_test -p 55440:5432 postgres:15`

## Prerequisite, not part of this plan

Spec decision 5 — widening the read-only API to Starter — is a **plan-entitlement change** and needs its own issue. Notify-and-fetch is incoherent for Starter merchants without it. This plan can be built and merged first; it must not be *announced* to merchants until that lands.

## File structure

| File | Responsibility |
|---|---|
| `internal/webhook/ssrfguard/guard.go` | Resolve a URL and reject non-public destinations. No DB, no HTTP. |
| `internal/webhook/models.go` | `Subscription`, `Delivery` GORM models + status constants |
| `internal/webhook/subscription_repo.go` | Subscription CRUD, failure counter, auto-disable |
| `internal/webhook/delivery_repo.go` | Fan-out insert, claim-batch, outcome recording, prune |
| `internal/webhook/cursor.go` | Dispatcher's own watermark over `outbox_events` |
| `internal/webhook/signing.go` | HMAC signature over timestamp + body |
| `internal/webhook/sender.go` | One HTTP attempt, guard applied immediately before dial |
| `internal/webhook/dispatcher.go` | outbox → deliveries fan-out loop |
| `internal/webhook/worker.go` | deliveries → merchant endpoint loop, backoff, auto-disable |
| `internal/handlers/admin/webhooks.go` | Admin CRUD + test-send + delivery log + replay |
| `apps/admin/app/(dashboard)/settings/webhooks/` | Admin UI |

---

### Task 1: SSRF guard

The security-critical unit, built first and standalone so it can be tested without a database or a network.

**Files:**
- Create: `services/marketplace-api/internal/webhook/ssrfguard/guard.go`
- Test: `services/marketplace-api/internal/webhook/ssrfguard/guard_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type Resolver func(host string) ([]net.IP, error)`
  - `type Guard struct{ resolve Resolver }`
  - `func New(r Resolver) *Guard` — pass `nil` for `net.LookupIP`
  - `func (g *Guard) Check(raw string) (*url.URL, error)` — parses, enforces https, resolves, rejects non-public IPs
  - `var ErrNotHTTPS, ErrPrivateAddress, ErrUnresolvable error`

- [ ] **Step 1: Write the failing test**

```go
package ssrfguard

import (
	"net"
	"strings"
	"testing"
)

func fixed(ips ...string) Resolver {
	return func(string) ([]net.IP, error) {
		out := make([]net.IP, 0, len(ips))
		for _, s := range ips {
			out = append(out, net.ParseIP(s))
		}
		return out, nil
	}
}

func TestCheck_AllowsPublicHTTPS(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	u, err := g.Check("https://hooks.example.com/mark8ly")
	if err != nil {
		t.Fatalf("expected public https URL to pass, got %v", err)
	}
	if u.Host != "hooks.example.com" {
		t.Fatalf("host = %q", u.Host)
	}
}

func TestCheck_RejectsPlainHTTP(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	if _, err := g.Check("http://hooks.example.com/x"); err != ErrNotHTTPS {
		t.Fatalf("want ErrNotHTTPS, got %v", err)
	}
}

// Each of these is a documented SSRF target. A merchant-supplied URL that
// resolves to any of them must never be dialled.
func TestCheck_RejectsNonPublicDestinations(t *testing.T) {
	for name, ip := range map[string]string{
		"loopback":         "127.0.0.1",
		"private 10/8":     "10.0.0.5",
		"private 172.16/12": "172.16.0.5",
		"private 192.168":  "192.168.1.5",
		"link-local":       "169.254.1.1",
		"GCP metadata":     "169.254.169.254",
		"IPv6 loopback":    "::1",
		"IPv6 ULA":         "fd00::1",
		"unspecified":      "0.0.0.0",
	} {
		t.Run(name, func(t *testing.T) {
			g := New(fixed(ip))
			if _, err := g.Check("https://evil.example.com/x"); err != ErrPrivateAddress {
				t.Fatalf("want ErrPrivateAddress for %s, got %v", ip, err)
			}
		})
	}
}

// A hostname that resolves to a mix must be rejected — an attacker only
// needs one private answer to be picked at dial time.
func TestCheck_RejectsWhenAnyResolvedAddressIsPrivate(t *testing.T) {
	g := New(fixed("93.184.216.34", "127.0.0.1"))
	if _, err := g.Check("https://mixed.example.com/x"); err != ErrPrivateAddress {
		t.Fatalf("want ErrPrivateAddress, got %v", err)
	}
}

func TestCheck_RejectsUnresolvable(t *testing.T) {
	g := New(func(string) ([]net.IP, error) { return nil, net.UnknownNetworkError("nope") })
	if _, err := g.Check("https://nx.example.com/x"); err != ErrUnresolvable {
		t.Fatalf("want ErrUnresolvable, got %v", err)
	}
}

func TestCheck_RejectsGarbage(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	for _, raw := range []string{"", "not a url", "https://", "ftp://x.example.com"} {
		if _, err := g.Check(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestCheck_RejectsOverlongURL(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	long := "https://hooks.example.com/" + strings.Repeat("a", 3000)
	if _, err := g.Check(long); err == nil {
		t.Fatal("expected an overlong URL to be rejected")
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `cd services/marketplace-api && go test ./internal/webhook/ssrfguard/...`
Expected: FAIL — package does not compile, `New` undefined.

- [ ] **Step 3: Implement the guard**

```go
// Package ssrfguard decides whether a merchant-supplied URL is safe for the
// cluster to dial.
//
// Webhooks are the first feature where merchant input becomes an egress
// target: every other outbound integration here (payment gateways, carriers,
// email providers) talks to fixed, configured endpoints. A merchant who can
// make us POST to an arbitrary URL can otherwise reach the GCP metadata
// server or any in-cluster service and read the response back out of the
// delivery log.
//
// Check is called at registration AND again immediately before every
// delivery. Registration-only validation is the usual shortcut and it is
// defeated by DNS rebinding — a hostname that answers public when saved and
// private when dialled.
package ssrfguard

import (
	"errors"
	"net"
	"net/url"
)

var (
	ErrNotHTTPS       = errors.New("webhook url must use https")
	ErrPrivateAddress = errors.New("webhook url resolves to a non-public address")
	ErrUnresolvable   = errors.New("webhook url host could not be resolved")
	ErrMalformed      = errors.New("webhook url is malformed")
	ErrTooLong        = errors.New("webhook url is too long")
)

// maxURLLen bounds what we will store and log.
const maxURLLen = 2048

// Resolver looks a hostname up. Injected so tests need no DNS.
type Resolver func(host string) ([]net.IP, error)

type Guard struct{ resolve Resolver }

// New builds a Guard. Pass nil to use real DNS.
func New(r Resolver) *Guard {
	if r == nil {
		r = net.LookupIP
	}
	return &Guard{resolve: r}
}

// Check parses raw, requires https, resolves the host, and rejects the URL
// if ANY resolved address is non-public — an attacker needs only one private
// answer to be selected at dial time.
func (g *Guard) Check(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, ErrMalformed
	}
	if len(raw) > maxURLLen {
		return nil, ErrTooLong
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, ErrMalformed
	}
	if u.Scheme != "https" {
		return nil, ErrNotHTTPS
	}
	if u.Hostname() == "" {
		return nil, ErrMalformed
	}

	ips, err := g.resolve(u.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, ErrUnresolvable
	}
	for _, ip := range ips {
		if !isPublic(ip) {
			return nil, ErrPrivateAddress
		}
	}
	return u, nil
}

// isPublic reports whether ip is globally routable. Everything else —
// loopback, private, link-local (which covers 169.254.169.254, the cloud
// metadata endpoint), multicast, unspecified — is refused.
func isPublic(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	return true
}
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: `cd services/marketplace-api && go test ./internal/webhook/ssrfguard/... -v`
Expected: PASS, all cases including every entry in the non-public table.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/webhook/ssrfguard/
git commit -m "feat(webhooks): SSRF guard rejecting non-public webhook destinations"
```

---

### Task 2: Schema, models, subscription repository

**Files:**
- Create: `services/marketplace-api/migrations/000126_webhook_subscriptions.up.sql` / `.down.sql`
- Create: `services/marketplace-api/internal/webhook/models.go`
- Create: `services/marketplace-api/internal/webhook/subscription_repo.go`
- Modify: `services/marketplace-api/migrations.go` (`ExpectedSchemaVersion` 125 → 126)
- Test: `services/marketplace-api/internal/webhook/subscription_repo_integration_test.go`

**Interfaces:**
- Consumes: Task 1's `ssrfguard` (validation happens in the handler, not the repo)
- Produces:
  - `webhook.Subscription` with fields `ID, TenantID, StoreID uuid.UUID`, `URL string`, `EventTypes pq.StringArray`, `Secret string`, `Enabled bool`, `DisabledReason *string`, `DisabledAt *time.Time`, `ConsecutiveFailures int`, `CreatedAt, UpdatedAt time.Time`
  - `webhook.Delivery` (table created here, used from Task 3)
  - `func NewSubscriptionRepo(db *gorm.DB) *SubscriptionRepo`
  - `(*SubscriptionRepo) Create(ctx, *Subscription) error`
  - `(*SubscriptionRepo) ListForStore(ctx, tenantID, storeID uuid.UUID) ([]Subscription, error)`
  - `(*SubscriptionRepo) MatchingEvent(ctx, tenantID uuid.UUID, eventType string) ([]Subscription, error)` — enabled only
  - `(*SubscriptionRepo) RecordFailure(ctx, id uuid.UUID, threshold int) (disabled bool, err error)`
  - `(*SubscriptionRepo) RecordSuccess(ctx, id uuid.UUID) error`
  - `const MaxEventTypes = 32`

- [ ] **Step 1: Write the migration**

```sql
-- Migration 126: outbound webhooks (#562).
--
-- Consumes the existing transactional outbox rather than instrumenting new
-- events: outbox_events already carries 18 domain events written in the same
-- transaction as the mutation that produced them.
--
-- Two tables, deliberately separate from outbox_events. A merchant's dead
-- endpoint must never stall the outbox watermark publisher, whose recovery
-- semantics are documented in internal/outbox/models.go (#336).
CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID         NOT NULL,
    store_id             UUID         NOT NULL,
    url                  VARCHAR(2048) NOT NULL,
    -- Event types this subscription wants, e.g. {order.placed,order.refunded}.
    -- Values come from internal/outbox's Event* constants.
    event_types          TEXT[]       NOT NULL,
    -- HMAC signing secret. Shown to the merchant once at creation.
    secret               VARCHAR(128) NOT NULL,
    enabled              BOOLEAN      NOT NULL DEFAULT true,
    -- Set when the platform auto-disables after sustained failure, so the
    -- merchant is told WHY rather than finding a silently dead webhook.
    disabled_reason      TEXT,
    disabled_at          TIMESTAMPTZ,
    consecutive_failures INTEGER      NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- The dispatcher's hot path: "enabled subscriptions for this tenant".
CREATE INDEX IF NOT EXISTS idx_webhook_subs_tenant_enabled
    ON webhook_subscriptions (tenant_id) WHERE enabled;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id   UUID         NOT NULL REFERENCES webhook_subscriptions(id) ON DELETE CASCADE,
    outbox_event_id   UUID         NOT NULL,
    event_type        VARCHAR(64)  NOT NULL,
    aggregate_id      UUID         NOT NULL,
    status            VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts          INTEGER      NOT NULL DEFAULT 0,
    next_attempt_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_status_code  INTEGER,
    -- Truncated by the worker before insert. Surfaced to the merchant so a
    -- failing endpoint is debuggable; never logged server-side.
    last_error        TEXT,
    delivered_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- This is what makes fan-out idempotent. The dispatcher inserts with
-- ON CONFLICT DO NOTHING against it, so re-running over the same outbox
-- rows cannot double-deliver — which is how we get exactly-once fan-out
-- without coupling to the outbox publisher's transaction.
CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_deliveries_event_sub
    ON webhook_deliveries (outbox_event_id, subscription_id);

-- The worker's claim query.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_due
    ON webhook_deliveries (next_attempt_at) WHERE status = 'pending';

-- Prune scan (30-day retention on every plan, see the design doc).
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created
    ON webhook_deliveries (created_at);

-- The dispatcher's own cursor over outbox_events. Deliberately NOT the
-- publisher's watermark: the two consumers advance independently, so a
-- stalled webhook dispatch cannot hold back outbox publishing.
CREATE TABLE IF NOT EXISTS webhook_dispatch_cursor (
    id                  BOOLEAN     PRIMARY KEY DEFAULT true CHECK (id),
    last_event_created  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_event_id       UUID
);
INSERT INTO webhook_dispatch_cursor (id) VALUES (true) ON CONFLICT DO NOTHING;
```

Down migration:

```sql
DROP TABLE IF EXISTS webhook_dispatch_cursor;
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_subscriptions;
```

- [ ] **Step 2: Write the failing integration test**

```go
//go:build integration

package webhook_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newSub(t *testing.T, repo *webhook.SubscriptionRepo, tenant uuid.UUID, events []string) *webhook.Subscription {
	t.Helper()
	s := &webhook.Subscription{
		TenantID:   tenant,
		StoreID:    uuid.New(),
		URL:        "https://hooks.example.com/x",
		EventTypes: events,
		Secret:     "s3cret-value-for-test",
		Enabled:    true,
	}
	require.NoError(t, repo.Create(context.Background(), s))
	return s
}

func TestMatchingEvent_ReturnsOnlyEnabledSubscriptionsWantingThatType(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	tenant := uuid.New()
	ctx := context.Background()

	want := newSub(t, repo, tenant, []string{"order.placed", "order.refunded"})
	newSub(t, repo, tenant, []string{"product.created"})           // wrong type
	newSub(t, repo, uuid.New(), []string{"order.placed"})          // wrong tenant
	disabled := newSub(t, repo, tenant, []string{"order.placed"})
	_, err := repo.RecordFailure(ctx, disabled.ID, 1)              // threshold 1 → disabled
	require.NoError(t, err)

	got, err := repo.MatchingEvent(ctx, tenant, "order.placed")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, want.ID, got[0].ID)
}

func TestRecordFailure_DisablesAtThresholdAndRecordsWhy(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	ctx := context.Background()
	s := newSub(t, repo, uuid.New(), []string{"order.placed"})

	disabled, err := repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.False(t, disabled, "one failure must not disable a subscription")

	_, err = repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	disabled, err = repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.True(t, disabled, "third consecutive failure should disable")

	all, err := repo.ListForStore(ctx, s.TenantID, s.StoreID)
	require.NoError(t, err)
	require.False(t, all[0].Enabled)
	require.NotNil(t, all[0].DisabledReason, "merchant must be told why")
	require.NotNil(t, all[0].DisabledAt)
}

// A working delivery after a failure must clear the counter, or an endpoint
// that fails intermittently over weeks would eventually be disabled despite
// mostly working.
func TestRecordSuccess_ResetsTheFailureCounter(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	ctx := context.Background()
	s := newSub(t, repo, uuid.New(), []string{"order.placed"})

	_, err := repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.NoError(t, repo.RecordSuccess(ctx, s.ID))

	// Two more failures must not disable — the counter restarted.
	_, err = repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	disabled, err := repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.False(t, disabled)
}
```

- [ ] **Step 3: Run it and watch it fail**

```bash
docker run -d --name m8-wh -e POSTGRES_PASSWORD=test -e POSTGRES_DB=marketplace_test -p 55440:5432 postgres:15
cd services/marketplace-api
export DSN='postgres://postgres:test@localhost:55440/marketplace_test?sslmode=disable'
DATABASE_URL="$DSN" go run ./cmd/migrate up
TEST_DATABASE_URL="$DSN" go test -tags=integration ./internal/webhook/... -run TestMatchingEvent -v
```

Expected: FAIL — `webhook.NewSubscriptionRepo` undefined.

- [ ] **Step 4: Write models.go**

```go
// Package webhook implements merchant-facing outbound webhooks (#562).
//
// It is a CONSUMER of internal/outbox, not a producer: outbox_events already
// records every domain event transactionally. See
// docs/superpowers/specs/2026-09-02-outbound-webhooks-design.md.
package webhook

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MaxEventTypes bounds how many event types one subscription may select.
// There are 18 today; the cap exists so a malformed request cannot store an
// unbounded array.
const MaxEventTypes = 32

// Delivery statuses.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

type Subscription struct {
	ID                  uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID            uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"                      json:"-"`
	StoreID             uuid.UUID      `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	URL                 string         `gorm:"column:url;not null"                                      json:"url"`
	EventTypes          pq.StringArray `gorm:"column:event_types;type:text[];not null"                   json:"event_types"`
	// Secret never leaves the server after creation — the handler returns it
	// once in its own response field and this tag keeps it out of every
	// subsequent read.
	Secret              string         `gorm:"column:secret;not null"                                   json:"-"`
	Enabled             bool           `gorm:"column:enabled;not null;default:true"                     json:"enabled"`
	DisabledReason      *string        `gorm:"column:disabled_reason"                                   json:"disabled_reason,omitempty"`
	DisabledAt          *time.Time     `gorm:"column:disabled_at"                                       json:"disabled_at,omitempty"`
	ConsecutiveFailures int            `gorm:"column:consecutive_failures;not null;default:0"           json:"-"`
	CreatedAt           time.Time      `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

func (Subscription) TableName() string { return "webhook_subscriptions" }

type Delivery struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubscriptionID uuid.UUID  `gorm:"column:subscription_id;type:uuid;not null"                json:"subscription_id"`
	OutboxEventID  uuid.UUID  `gorm:"column:outbox_event_id;type:uuid;not null"                json:"-"`
	EventType      string     `gorm:"column:event_type;not null"                               json:"event_type"`
	AggregateID    uuid.UUID  `gorm:"column:aggregate_id;type:uuid;not null"                   json:"aggregate_id"`
	Status         string     `gorm:"column:status;not null;default:pending"                   json:"status"`
	Attempts       int        `gorm:"column:attempts;not null;default:0"                       json:"attempts"`
	NextAttemptAt  time.Time  `gorm:"column:next_attempt_at;not null;default:now()"            json:"next_attempt_at"`
	LastStatusCode *int       `gorm:"column:last_status_code"                                  json:"last_status_code,omitempty"`
	LastError      *string    `gorm:"column:last_error"                                        json:"last_error,omitempty"`
	DeliveredAt    *time.Time `gorm:"column:delivered_at"                                      json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (Delivery) TableName() string { return "webhook_deliveries" }
```

- [ ] **Step 5: Write subscription_repo.go**

```go
package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionRepo struct{ db *gorm.DB }

func NewSubscriptionRepo(db *gorm.DB) *SubscriptionRepo { return &SubscriptionRepo{db: db} }

func (r *SubscriptionRepo) Create(ctx context.Context, s *Subscription) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("webhook: create subscription: %w", err)
	}
	return nil
}

func (r *SubscriptionRepo) ListForStore(ctx context.Context, tenantID, storeID uuid.UUID) ([]Subscription, error) {
	var out []Subscription
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		Order("created_at DESC").Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: list subscriptions: %w", err)
	}
	return out, nil
}

// MatchingEvent returns the ENABLED subscriptions for tenantID that selected
// eventType. `event_types @> ARRAY[?]` uses the array containment operator so
// the match happens in Postgres rather than by loading every subscription.
func (r *SubscriptionRepo) MatchingEvent(ctx context.Context, tenantID uuid.UUID, eventType string) ([]Subscription, error) {
	var out []Subscription
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND enabled AND event_types @> ARRAY[?]::text[]", tenantID, eventType).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: match subscriptions: %w", err)
	}
	return out, nil
}

// RecordFailure increments the consecutive-failure counter and disables the
// subscription once it reaches threshold, reporting whether it did.
//
// The increment and the disable happen in ONE statement so two delivery
// workers failing concurrently cannot interleave a read-modify-write and
// lose a count.
func (r *SubscriptionRepo) RecordFailure(ctx context.Context, id uuid.UUID, threshold int) (bool, error) {
	var enabled bool
	err := r.db.WithContext(ctx).Raw(`
		UPDATE webhook_subscriptions
		   SET consecutive_failures = consecutive_failures + 1,
		       enabled = CASE WHEN consecutive_failures + 1 >= ? THEN false ELSE enabled END,
		       disabled_reason = CASE WHEN consecutive_failures + 1 >= ?
		            THEN 'Disabled automatically after ' || (consecutive_failures + 1) ||
		                 ' consecutive delivery failures. Fix the endpoint and re-enable.'
		            ELSE disabled_reason END,
		       disabled_at = CASE WHEN consecutive_failures + 1 >= ? THEN now() ELSE disabled_at END,
		       updated_at = now()
		 WHERE id = ?
		RETURNING enabled`, threshold, threshold, threshold, id).Scan(&enabled).Error
	if err != nil {
		return false, fmt.Errorf("webhook: record failure: %w", err)
	}
	return !enabled, nil
}

// RecordSuccess clears the counter. Without this an endpoint that fails
// occasionally over weeks would eventually be disabled despite working.
func (r *SubscriptionRepo) RecordSuccess(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE webhook_subscriptions
		   SET consecutive_failures = 0, updated_at = now()
		 WHERE id = ? AND consecutive_failures <> 0`, id).Error
	if err != nil {
		return fmt.Errorf("webhook: record success: %w", err)
	}
	return nil
}

var _ = time.Now
```

- [ ] **Step 6: Bump `ExpectedSchemaVersion`**

In `services/marketplace-api/migrations.go`, change `const ExpectedSchemaVersion uint = 125` to `126`.

- [ ] **Step 7: Run the tests and watch them pass**

```bash
TEST_DATABASE_URL="$DSN" go test -tags=integration ./internal/webhook/... -v
```
Expected: PASS — all three tests. Confirm they actually ran; a silent skip also prints `ok`.

- [ ] **Step 8: Verify the down migration**

```bash
DATABASE_URL="$DSN" go run ./cmd/migrate down 1
DATABASE_URL="$DSN" go run ./cmd/migrate up
```
Expected: both succeed; tables dropped then recreated.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/migrations/000126_* services/marketplace-api/migrations.go services/marketplace-api/internal/webhook/
git commit -m "feat(webhooks): subscription and delivery schema with auto-disable counter"
```

---

### Task 3: Signing

Small, pure, and worth its own task because merchants write verification code against it and it cannot change later without breaking them.

**Files:**
- Create: `services/marketplace-api/internal/webhook/signing.go`
- Test: `services/marketplace-api/internal/webhook/signing_test.go`

**Interfaces:**
- Produces:
  - `func Sign(secret string, ts time.Time, body []byte) string` — returns `"t=<unix>,v1=<hex>"`
  - `func GenerateSecret() (string, error)` — 32 random bytes, hex
  - `const SignatureHeader = "X-Mark8ly-Signature"`

- [ ] **Step 1: Write the failing test**

```go
package webhook

import (
	"strings"
	"testing"
	"time"
)

func TestSign_IsStableForTheSameInputs(t *testing.T) {
	ts := time.Unix(1756800000, 0)
	a := Sign("shh", ts, []byte(`{"event":"order.placed"}`))
	b := Sign("shh", ts, []byte(`{"event":"order.placed"}`))
	if a != b {
		t.Fatalf("signature not deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "t=1756800000,v1=") {
		t.Fatalf("unexpected format: %q", a)
	}
}

// The timestamp must be INSIDE the signed material. If it were only a
// separate header, a captured delivery could be replayed later with a fresh
// timestamp and still verify.
func TestSign_ChangesWithTimestamp(t *testing.T) {
	body := []byte(`{"event":"order.placed"}`)
	a := Sign("shh", time.Unix(1756800000, 0), body)
	b := Sign("shh", time.Unix(1756800001, 0), body)
	if a == b {
		t.Fatal("signature must cover the timestamp")
	}
}

func TestSign_ChangesWithBodyAndSecret(t *testing.T) {
	ts := time.Unix(1756800000, 0)
	base := Sign("shh", ts, []byte(`{"a":1}`))
	if Sign("shh", ts, []byte(`{"a":2}`)) == base {
		t.Fatal("signature must cover the body")
	}
	if Sign("other", ts, []byte(`{"a":1}`)) == base {
		t.Fatal("signature must depend on the secret")
	}
}

func TestGenerateSecret_IsRandomAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s, err := GenerateSecret()
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 64 {
			t.Fatalf("want 64 hex chars, got %d", len(s))
		}
		if seen[s] {
			t.Fatal("duplicate secret generated")
		}
		seen[s] = true
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd services/marketplace-api && go test ./internal/webhook/ -run 'TestSign|TestGenerateSecret' -v`
Expected: FAIL — `Sign` undefined.

- [ ] **Step 3: Implement**

```go
package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// SignatureHeader carries the signature on every delivery.
const SignatureHeader = "X-Mark8ly-Signature"

// Sign returns "t=<unix>,v1=<hex>" over "<unix>.<body>" using secret.
//
// The timestamp is part of the SIGNED material, not merely a sibling header:
// that is what lets a merchant reject a replayed delivery by checking the
// timestamp is recent, knowing an attacker cannot rewrite it without
// invalidating v1. The format mirrors Stripe's so the verification recipe is
// already familiar to most integrators.
func Sign(secret string, ts time.Time, body []byte) string {
	unix := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unix))
	mac.Write([]byte("."))
	mac.Write(body)
	return fmt.Sprintf("t=%s,v1=%s", unix, hex.EncodeToString(mac.Sum(nil)))
}

// GenerateSecret returns 32 random bytes hex-encoded. Shown to the merchant
// once at creation and never returned again.
func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("webhook: generate secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
```

- [ ] **Step 4: Run and watch them pass**

Run: `go test ./internal/webhook/ -run 'TestSign|TestGenerateSecret' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/webhook/signing.go services/marketplace-api/internal/webhook/signing_test.go
git commit -m "feat(webhooks): HMAC signing with the timestamp inside the signed material"
```

---

### Task 4: Fan-out — delivery repository and dispatcher

**Files:**
- Create: `services/marketplace-api/internal/webhook/delivery_repo.go`
- Create: `services/marketplace-api/internal/webhook/dispatcher.go`
- Test: `services/marketplace-api/internal/webhook/dispatcher_integration_test.go`

**Interfaces:**
- Consumes: `NewSubscriptionRepo`, `Subscription`, `Delivery`, `StatusPending` (Task 2)
- Produces:
  - `func NewDeliveryRepo(db *gorm.DB) *DeliveryRepo`
  - `(*DeliveryRepo) FanOut(ctx, rows []Delivery) (int, error)` — `ON CONFLICT DO NOTHING`
  - `func NewDispatcher(db *gorm.DB, subs *SubscriptionRepo, deliveries *DeliveryRepo, log *slog.Logger, batch int) *Dispatcher`
  - `(*Dispatcher) Tick(ctx) (int, error)` — returns deliveries created
  - `(*Dispatcher) Start(ctx, interval time.Duration) <-chan struct{}`

- [ ] **Step 1: Write the failing integration test**

```go
//go:build integration

package webhook_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
	"gorm.io/gorm"
)

func enqueueOutbox(t *testing.T, db *gorm.DB, tenant uuid.UUID, eventType string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	payload, _ := json.Marshal(map[string]any{"store_id": uuid.New().String()})
	require.NoError(t, db.Exec(`
		INSERT INTO outbox_events (id, tenant_id, aggregate, aggregate_id, event_type, payload)
		VALUES (?, ?, 'order', ?, ?, ?)`,
		id, tenant, uuid.New(), eventType, payload).Error)
	return id
}

func TestDispatcher_CreatesOneDeliveryPerMatchingSubscription(t *testing.T) {
	db := testdb.NewDB(t)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	ctx := context.Background()
	tenant := uuid.New()

	newSub(t, subs, tenant, []string{"order.placed"})
	newSub(t, subs, tenant, []string{"order.placed", "order.refunded"})
	newSub(t, subs, tenant, []string{"product.created"}) // must not match
	enqueueOutbox(t, db, tenant, "order.placed")

	n, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, n, "one delivery per subscription that selected the type")
}

// The property that replaces the exactly-once guarantee we gave up by NOT
// coupling to the outbox publisher's transaction.
func TestDispatcher_IsIdempotentAcrossRuns(t *testing.T) {
	db := testdb.NewDB(t)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()
	tenant := uuid.New()
	newSub(t, subs, tenant, []string{"order.placed"})
	enqueueOutbox(t, db, tenant, "order.placed")

	first := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	n1, err := first.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n1)

	// A dispatcher starting from a fresh cursor re-reads the same rows —
	// the unique index must stop it double-delivering.
	require.NoError(t, db.Exec(`UPDATE webhook_dispatch_cursor SET last_event_created = 'epoch'`).Error)
	second := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	n2, err := second.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n2, "re-dispatching the same outbox rows must create nothing")

	var count int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// The failure this whole architecture exists to prevent.
func TestDispatcher_DoesNotTouchOutboxPublishedState(t *testing.T) {
	db := testdb.NewDB(t)
	subs := webhook.NewSubscriptionRepo(db)
	d := webhook.NewDispatcher(db, subs, webhook.NewDeliveryRepo(db), slog.Default(), 100)
	tenant := uuid.New()
	newSub(t, subs, tenant, []string{"order.placed"})
	id := enqueueOutbox(t, db, tenant, "order.placed")

	_, err := d.Tick(context.Background())
	require.NoError(t, err)

	var row outbox.OutboxEvent
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	require.Nil(t, row.PublishedAt, "webhook dispatch must not mark outbox rows published")
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `TEST_DATABASE_URL="$DSN" go test -tags=integration ./internal/webhook/ -run TestDispatcher -v`
Expected: FAIL — `NewDeliveryRepo` / `NewDispatcher` undefined.

- [ ] **Step 3: Write delivery_repo.go**

```go
package webhook

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeliveryRepo struct{ db *gorm.DB }

func NewDeliveryRepo(db *gorm.DB) *DeliveryRepo { return &DeliveryRepo{db: db} }

// FanOut inserts delivery rows, ignoring any that already exist.
//
// ON CONFLICT DO NOTHING against idx_webhook_deliveries_event_sub is what
// makes dispatch idempotent, and therefore what lets the dispatcher run
// OUTSIDE the outbox publisher's transaction without risking duplicate
// deliveries. Re-reading the same outbox rows is harmless.
func (r *DeliveryRepo) FanOut(ctx context.Context, rows []Delivery) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "outbox_event_id"}, {Name: "subscription_id"}},
		DoNothing: true,
	}).Create(&rows)
	if res.Error != nil {
		return 0, fmt.Errorf("webhook: fan out deliveries: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}
```

- [ ] **Step 4: Write dispatcher.go**

```go
package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Dispatcher turns outbox_events into webhook_deliveries.
//
// It keeps its OWN cursor (webhook_dispatch_cursor) rather than reading the
// outbox publisher's watermark, so the two consumers advance independently.
// A stalled webhook dispatch cannot hold back outbox publishing, and vice
// versa. It never writes to outbox_events.
type Dispatcher struct {
	db         *gorm.DB
	subs       *SubscriptionRepo
	deliveries *DeliveryRepo
	logger     *slog.Logger
	batch      int
}

func NewDispatcher(db *gorm.DB, subs *SubscriptionRepo, deliveries *DeliveryRepo, logger *slog.Logger, batch int) *Dispatcher {
	if batch <= 0 {
		batch = 100
	}
	return &Dispatcher{db: db, subs: subs, deliveries: deliveries, logger: logger, batch: batch}
}

type outboxRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	CreatedAt   time.Time
}

// Tick reads one batch of outbox events past the cursor, fans them out, and
// advances the cursor. Returns how many delivery rows were created.
func (d *Dispatcher) Tick(ctx context.Context) (int, error) {
	var cursor time.Time
	if err := d.db.WithContext(ctx).
		Raw(`SELECT last_event_created FROM webhook_dispatch_cursor WHERE id`).
		Scan(&cursor).Error; err != nil {
		return 0, fmt.Errorf("webhook: read cursor: %w", err)
	}

	var rows []outboxRow
	if err := d.db.WithContext(ctx).Raw(`
		SELECT id, tenant_id, aggregate_id, event_type, created_at
		  FROM outbox_events
		 WHERE created_at > ?
		 ORDER BY created_at ASC
		 LIMIT ?`, cursor, d.batch).Scan(&rows).Error; err != nil {
		return 0, fmt.Errorf("webhook: read outbox: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	created := 0
	for _, row := range rows {
		matches, err := d.subs.MatchingEvent(ctx, row.TenantID, row.EventType)
		if err != nil {
			return created, err
		}
		pending := make([]Delivery, 0, len(matches))
		for _, s := range matches {
			pending = append(pending, Delivery{
				SubscriptionID: s.ID,
				OutboxEventID:  row.ID,
				EventType:      row.EventType,
				AggregateID:    row.AggregateID,
				Status:         StatusPending,
				NextAttemptAt:  time.Now(),
			})
		}
		n, err := d.deliveries.FanOut(ctx, pending)
		if err != nil {
			return created, err
		}
		created += n
	}

	last := rows[len(rows)-1]
	if err := d.db.WithContext(ctx).Exec(`
		UPDATE webhook_dispatch_cursor
		   SET last_event_created = ?, last_event_id = ?
		 WHERE id`, last.CreatedAt, last.ID).Error; err != nil {
		return created, fmt.Errorf("webhook: advance cursor: %w", err)
	}
	return created, nil
}

// Start runs Tick on an interval until ctx is cancelled. Mirrors
// outbox.Publisher.Start so the two loops behave the same way on shutdown.
func (d *Dispatcher) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := d.Tick(ctx); err != nil && d.logger != nil {
					d.logger.Error("webhook dispatcher tick failed", "err", err)
				}
			}
		}
	}()
	return done
}
```

- [ ] **Step 5: Run and watch them pass**

Run: `TEST_DATABASE_URL="$DSN" go test -tags=integration ./internal/webhook/ -run TestDispatcher -v`
Expected: PASS — all three, including the outbox-isolation test.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/webhook/delivery_repo.go services/marketplace-api/internal/webhook/dispatcher.go services/marketplace-api/internal/webhook/dispatcher_integration_test.go
git commit -m "feat(webhooks): idempotent fan-out from outbox events to per-subscription deliveries"
```

---

### Task 5: Sender and delivery worker

**Files:**
- Create: `services/marketplace-api/internal/webhook/sender.go`
- Create: `services/marketplace-api/internal/webhook/worker.go`
- Modify: `services/marketplace-api/internal/webhook/delivery_repo.go` (add `ClaimDue`, `RecordOutcome`)
- Test: `services/marketplace-api/internal/webhook/sender_test.go`, `worker_integration_test.go`

**Interfaces:**
- Consumes: `ssrfguard.Guard.Check` (Task 1), `Sign`, `SignatureHeader` (Task 3), `DeliveryRepo`, `SubscriptionRepo` (Tasks 2, 4)
- Produces:
  - `func NewSender(guard *ssrfguard.Guard, client *http.Client) *Sender`
  - `(*Sender) Send(ctx, sub Subscription, d Delivery) (statusCode int, err error)`
  - `(*DeliveryRepo) ClaimDue(ctx, limit int) ([]Delivery, error)` — `FOR UPDATE SKIP LOCKED`
  - `(*DeliveryRepo) RecordOutcome(ctx, id uuid.UUID, status string, code *int, errMsg *string, next time.Time) error`
  - `(*DeliveryRepo) Prune(ctx, olderThan time.Duration) (int64, error)` — consumed by Task 8
  - `(*SubscriptionRepo) ByID(ctx, id uuid.UUID) (*Subscription, error)` — added to Task 2's file in this task; returns `(nil, nil)` when the row is gone
  - `const RetentionWindow = 30 * 24 * time.Hour` — consumed by Task 8
  - `func NewWorker(...) *Worker`, `(*Worker) Tick(ctx) (int, error)`, `(*Worker) Start(ctx, interval) <-chan struct{}`
  - `const MaxAttempts = 6`, `const FailureThreshold = 10`, `const RequestTimeout = 5 * time.Second`
  - `func backoff(attempt int) time.Duration`

- [ ] **Step 1: Write the failing sender test**

```go
package webhook

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
)

// allowAll resolves every host to a public address so httptest servers on
// 127.0.0.1 can be exercised. Production passes nil, which uses real DNS.
func allowAll() *ssrfguard.Guard {
	return ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
}

func TestSend_PostsSignedNotifyAndFetchBody(t *testing.T) {
	var gotBody []byte
	var gotSig string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(SignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSender(allowAll(), srv.Client())
	sub := Subscription{ID: uuid.New(), URL: srv.URL, Secret: "shh"}
	d := Delivery{EventType: "order.placed", AggregateID: uuid.New(), CreatedAt: time.Now()}

	code, err := s.Send(context.Background(), sub, d)
	if err != nil || code != 200 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if gotSig == "" {
		t.Fatal("delivery was not signed")
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"event", "id", "occurred_at"} {
		if _, ok := payload[k]; !ok {
			t.Fatalf("payload missing %q: %s", k, gotBody)
		}
	}
	// Notify-and-fetch: the body must NOT carry the entity.
	if len(payload) != 3 {
		t.Fatalf("payload should carry exactly event/id/occurred_at, got %v", payload)
	}
}

// The guard runs immediately before dialling, not only at registration.
// This is the DNS-rebinding case: the row was saved when the host was
// public, and now resolves private.
func TestSend_RefusesWhenTheHostNowResolvesPrivate(t *testing.T) {
	rebind := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	})
	s := NewSender(rebind, http.DefaultClient)
	sub := Subscription{ID: uuid.New(), URL: "https://hooks.example.com/x", Secret: "shh"}

	if _, err := s.Send(context.Background(), sub, Delivery{EventType: "order.placed"}); err == nil {
		t.Fatal("expected delivery to a rebound private address to be refused")
	}
}

func TestSend_TreatsNon2xxAsFailureButReturnsTheCode(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewSender(allowAll(), srv.Client())
	code, err := s.Send(context.Background(), Subscription{URL: srv.URL, Secret: "x"}, Delivery{EventType: "order.placed"})
	if err == nil {
		t.Fatal("non-2xx must be an error")
	}
	if code != 500 {
		t.Fatalf("want the status code surfaced for the merchant log, got %d", code)
	}
}

func TestBackoff_GrowsAndIsBounded(t *testing.T) {
	prev := time.Duration(0)
	for a := 1; a <= MaxAttempts; a++ {
		d := backoff(a)
		if d <= prev {
			t.Fatalf("backoff must increase: attempt %d gave %v after %v", a, d, prev)
		}
		if d > 4*time.Hour {
			t.Fatalf("backoff unbounded at attempt %d: %v", a, d)
		}
		prev = d
	}
}
```

Add `"io"` to the import block.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/webhook/ -run 'TestSend|TestBackoff' -v`
Expected: FAIL — `NewSender` undefined.

- [ ] **Step 3: Write sender.go**

```go
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
)

// RequestTimeout bounds one delivery attempt. Short on purpose: these loops
// run in-process alongside admin API request handling, so a slow endpoint
// must not hold a goroutine for long.
const RequestTimeout = 5 * time.Second

// maxErrorLen bounds what we store from a failing endpoint's response. The
// body is surfaced to the merchant to make a broken endpoint debuggable; it
// is never logged server-side, since it is arbitrary remote content.
const maxErrorLen = 500

type Sender struct {
	guard  *ssrfguard.Guard
	client *http.Client
}

func NewSender(guard *ssrfguard.Guard, client *http.Client) *Sender {
	if client == nil {
		client = &http.Client{
			Timeout: RequestTimeout,
			// Never follow redirects: a 302 to an internal address would
			// walk straight around the guard, which only checked the
			// original host.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Sender{guard: guard, client: client}
}

// notification is the notify-and-fetch body. Identifiers only — the merchant
// calls the REST API for detail, so no customer data reaches a
// merchant-supplied URL and a retry cannot deliver a stale entity.
type notification struct {
	Event      string    `json:"event"`
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Send makes one attempt. It re-checks the URL through the guard FIRST:
// registration-time validation alone is defeated by DNS rebinding.
func (s *Sender) Send(ctx context.Context, sub Subscription, d Delivery) (int, error) {
	if _, err := s.guard.Check(sub.URL); err != nil {
		return 0, fmt.Errorf("webhook: destination refused: %w", err)
	}

	occurred := d.CreatedAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	body, err := json.Marshal(notification{
		Event:      d.EventType,
		ID:         d.AggregateID.String(),
		OccurredAt: occurred.UTC(),
	})
	if err != nil {
		return 0, fmt.Errorf("webhook: marshal notification: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("webhook: build request: %w", err)
	}
	now := time.Now()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mark8ly-Webhooks/1")
	req.Header.Set(SignatureHeader, Sign(sub.Secret, now, body))

	res, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("webhook: request failed: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.CopyN(io.Discard, res.Body, 1<<16)

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, fmt.Errorf("webhook: endpoint returned %d", res.StatusCode)
	}
	return res.StatusCode, nil
}

// backoff returns the delay before attempt n (1-based): roughly 30s, 2m,
// 8m, 32m, 2h, capped. Spread over hours so a merchant restarting a server
// has time to recover before attempts are exhausted.
func backoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 4
		if d > 4*time.Hour {
			return 4 * time.Hour
		}
	}
	return d
}
```

- [ ] **Step 4: Add ClaimDue and RecordOutcome to delivery_repo.go**

```go
// ClaimDue locks up to limit pending, due deliveries. FOR UPDATE SKIP LOCKED
// is what makes it safe for several replicas to run this loop at once — each
// takes a disjoint set instead of contending.
func (r *DeliveryRepo) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	var out []Delivery
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM webhook_deliveries
		 WHERE status = ? AND next_attempt_at <= now()
		 ORDER BY next_attempt_at ASC
		 LIMIT ?
		 FOR UPDATE SKIP LOCKED`, StatusPending, limit).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: claim deliveries: %w", err)
	}
	return out, nil
}

// RecordOutcome writes the result of one attempt.
func (r *DeliveryRepo) RecordOutcome(ctx context.Context, id uuid.UUID, status string, code *int, errMsg *string, next time.Time) error {
	updates := map[string]any{
		"status":           status,
		"attempts":         gorm.Expr("attempts + 1"),
		"last_status_code": code,
		"last_error":       errMsg,
		"next_attempt_at":  next,
	}
	if status == StatusDelivered {
		updates["delivered_at"] = time.Now()
	}
	if err := r.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("webhook: record outcome: %w", err)
	}
	return nil
}

// Prune deletes delivery rows older than the retention window. 30 days on
// every plan, deliberately not tied to FeatureAuditRetentionDays: "forever"
// retention of delivery bodies on Pro is storage cost on a db-f1-micro with
// no matching merchant value.
func (r *DeliveryRepo) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM webhook_deliveries WHERE created_at < now() - ?::interval`,
		fmt.Sprintf("%d hours", int(olderThan.Hours())))
	if res.Error != nil {
		return 0, fmt.Errorf("webhook: prune deliveries: %w", res.Error)
	}
	return res.RowsAffected, nil
}
```

Add `"time"`, `"github.com/google/uuid"` and `"gorm.io/gorm"` to the imports.

- [ ] **Step 5: Write worker.go**

```go
package webhook

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxAttempts before a delivery is dead-lettered.
	MaxAttempts = 6
	// FailureThreshold of CONSECUTIVE failures before the subscription is
	// disabled and the merchant told. Higher than MaxAttempts so one bad
	// delivery cannot disable a working endpoint — it takes sustained
	// failure across different events.
	FailureThreshold = 10
	// RetentionWindow for delivery rows.
	RetentionWindow = 30 * 24 * time.Hour
)

// Worker drains webhook_deliveries.
type Worker struct {
	deliveries *DeliveryRepo
	subs       *SubscriptionRepo
	sender     *Sender
	logger     *slog.Logger
	batch      int
	notify     func(sub Subscription)
}

func NewWorker(deliveries *DeliveryRepo, subs *SubscriptionRepo, sender *Sender, logger *slog.Logger, batch int, notify func(Subscription)) *Worker {
	if batch <= 0 {
		batch = 4 // bounded: these goroutines share a pod with API traffic
	}
	return &Worker{deliveries: deliveries, subs: subs, sender: sender, logger: logger, batch: batch, notify: notify}
}

// Tick sends one batch. Returns how many deliveries were attempted.
func (w *Worker) Tick(ctx context.Context) (int, error) {
	due, err := w.deliveries.ClaimDue(ctx, w.batch)
	if err != nil {
		return 0, err
	}
	for _, d := range due {
		w.attempt(ctx, d)
	}
	return len(due), nil
}

func (w *Worker) attempt(ctx context.Context, d Delivery) {
	sub, err := w.subs.ByID(ctx, d.SubscriptionID)
	if err != nil || sub == nil {
		return // subscription deleted mid-flight; the cascade will clean up
	}

	code, sendErr := w.sender.Send(ctx, *sub, d)
	var codePtr *int
	if code != 0 {
		codePtr = &code
	}

	if sendErr == nil {
		if err := w.deliveries.RecordOutcome(ctx, d.ID, StatusDelivered, codePtr, nil, time.Now()); err != nil && w.logger != nil {
			w.logger.Error("webhook: record delivered failed", "err", err)
		}
		if err := w.subs.RecordSuccess(ctx, sub.ID); err != nil && w.logger != nil {
			w.logger.Error("webhook: record success failed", "err", err)
		}
		return
	}

	// Never log the endpoint's response body — arbitrary remote content.
	msg := truncate(sendErr.Error(), maxErrorLen)
	attempts := d.Attempts + 1
	status, next := StatusPending, time.Now().Add(backoff(attempts))
	if attempts >= MaxAttempts {
		status, next = StatusFailed, time.Now()
	}
	if err := w.deliveries.RecordOutcome(ctx, d.ID, status, codePtr, &msg, next); err != nil && w.logger != nil {
		w.logger.Error("webhook: record failure failed", "err", err)
	}

	if status != StatusFailed {
		return
	}
	disabled, err := w.subs.RecordFailure(ctx, sub.ID, FailureThreshold)
	if err != nil && w.logger != nil {
		w.logger.Error("webhook: record subscription failure", "err", err)
	}
	if disabled {
		if w.logger != nil {
			w.logger.Warn("webhook subscription auto-disabled",
				slog.String("subscription_id", sub.ID.String()))
		}
		if w.notify != nil {
			w.notify(*sub)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Start runs Tick on an interval until ctx is cancelled.
func (w *Worker) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := w.Tick(ctx); err != nil && w.logger != nil {
					w.logger.Error("webhook worker tick failed", "err", err)
				}
			}
		}
	}()
	return done
}

var _ = uuid.Nil
```

Add to `subscription_repo.go`:

```go
// ByID returns one subscription, or (nil, nil) if it no longer exists.
func (r *SubscriptionRepo) ByID(ctx context.Context, id uuid.UUID) (*Subscription, error) {
	var s Subscription
	err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("webhook: get subscription: %w", err)
	}
	return &s, nil
}
```

- [ ] **Step 6: Write the worker integration test**

```go
//go:build integration

package webhook_test

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
	"github.com/google/uuid"
)

func TestWorker_DeliversAndMarksDelivered(t *testing.T) {
	var hits int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	db := testdb.NewDB(t)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	guard := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	w := webhook.NewWorker(deliveries, subs, webhook.NewSender(guard, srv.Client()), slog.Default(), 4, nil)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	require.NoError(t, db.Exec(`UPDATE webhook_subscriptions SET url = ? WHERE id = ?`, srv.URL, sub.ID).Error)
	_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
		SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
		AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
	}})
	require.NoError(t, err)

	n, err := w.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.EqualValues(t, 1, atomic.LoadInt32(&hits))

	var got webhook.Delivery
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, webhook.StatusDelivered, got.Status)
	require.NotNil(t, got.DeliveredAt)
}

func TestWorker_RetriesWithBackoffThenDeadLetters(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	db := testdb.NewDB(t)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	guard := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	w := webhook.NewWorker(deliveries, subs, webhook.NewSender(guard, srv.Client()), slog.Default(), 4, nil)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	require.NoError(t, db.Exec(`UPDATE webhook_subscriptions SET url = ? WHERE id = ?`, srv.URL, sub.ID).Error)
	_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
		SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
		AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
	}})
	require.NoError(t, err)

	for i := 0; i < webhook.MaxAttempts; i++ {
		_, err := w.Tick(ctx)
		require.NoError(t, err)
		// Make the next attempt due immediately rather than sleeping out the backoff.
		require.NoError(t, db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = now()`).Error)
	}

	var got webhook.Delivery
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, webhook.StatusFailed, got.Status)
	require.Equal(t, webhook.MaxAttempts, got.Attempts)
	require.NotNil(t, got.LastStatusCode)
	require.Equal(t, 500, *got.LastStatusCode)
}
```

- [ ] **Step 7: Run everything and watch it pass**

```bash
go test ./internal/webhook/... -v
TEST_DATABASE_URL="$DSN" go test -tags=integration ./internal/webhook/... -v
```
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/webhook/
git commit -m "feat(webhooks): delivery worker with backoff, dead-lettering and auto-disable"
```

---

### Task 6: Wire the loops into main.go

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (beside the outbox publisher, around line 1690)

**Interfaces:**
- Consumes: `NewDispatcher`, `NewWorker`, `NewSender`, `ssrfguard.New` (Tasks 1, 4, 5)
- Produces: nothing consumed by later tasks

- [ ] **Step 1: Add the wiring**

Directly after the existing `if m == mode.Admin || m == mode.Both { pub := outbox.New(...) ... }` block:

```go
	// Outbound webhooks (#562). Two loops beside the outbox publisher, on the
	// same engines: a merchant subscription is admin-domain state, and the
	// storefront replica must not run a second copy of either poll.
	//
	// In-process rather than a separate Deployment, deliberately. The
	// alternative costs another chart, another ArgoCD Application and another
	// pod's memory on a cluster that has already had rollouts deadlock under
	// memory pressure. FOR UPDATE SKIP LOCKED makes both loops safe across
	// KEDA replicas. The bounded worker batch and the 5s per-request timeout
	// in internal/webhook cap what a slow merchant endpoint can tie up.
	//
	// Because dispatch and delivery are decoupled by webhook_deliveries,
	// moving the delivery loop to its own workload later is a deployment
	// change, not a redesign.
	webhookCtx, webhookCancel := context.WithCancel(context.Background())
	defer webhookCancel()
	var webhookDispatcherDone, webhookWorkerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		whSubs := webhook.NewSubscriptionRepo(conn)
		whDeliveries := webhook.NewDeliveryRepo(conn)
		whSender := webhook.NewSender(ssrfguard.New(nil), nil)

		dispatcher := webhook.NewDispatcher(conn, whSubs, whDeliveries, log, 100)
		webhookDispatcherDone = dispatcher.Start(webhookCtx, 5*time.Second)

		worker := webhook.NewWorker(whDeliveries, whSubs, whSender, log, 4, nil)
		webhookWorkerDone = worker.Start(webhookCtx, 5*time.Second)

		log.Info("webhook dispatcher and delivery worker started")
	}
```

Add imports:
```go
	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
```

- [ ] **Step 2: Drain both loops on shutdown**

Find where `publisherDone` is waited on during graceful shutdown and add the same treatment for `webhookDispatcherDone` and `webhookWorkerDone`, following the existing pattern exactly.

- [ ] **Step 3: Verify it builds and the whole suite still passes**

```bash
go build ./... && go vet ./...
TEST_DATABASE_URL="$DSN" go test -tags=integration -p 1 ./...
```
Expected: build and vet clean; every package `ok`. **Run the full suite** — the last two features here each tripped a guard in an untouched package.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(webhooks): run dispatcher and delivery worker in-process on admin engines"
```

---

### Task 7: Admin API

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/webhooks.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (register routes with the other admin handlers)
- Test: `services/marketplace-api/internal/handlers/admin/webhooks_test.go`

**Interfaces:**
- Consumes: `SubscriptionRepo`, `DeliveryRepo`, `Sender`, `GenerateSecret`, `MaxEventTypes` (Tasks 1–5), `ssrfguard` errors
- Produces: routes under the existing authenticated admin group —
  - `POST   /webhooks` → create (returns the secret **once**)
  - `GET    /webhooks` → list for the current store
  - `PATCH  /webhooks/:id` → update url / event types / enabled
  - `DELETE /webhooks/:id`
  - `POST   /webhooks/:id/test` → send a synthetic `webhook.test` delivery
  - `GET    /webhooks/:id/deliveries` → recent deliveries
  - `POST   /webhooks/:id/deliveries/:deliveryID/replay` → reset to pending, due now

- [ ] **Step 1: Write the failing handler test**

Follow the table-driven style of the neighbouring `internal/handlers/admin/*_test.go` files. Cover:

```go
// A merchant-supplied URL is validated at REGISTRATION as well as delivery.
// Without this, a private URL is stored and only rejected later, hidden in
// the delivery log.
func TestCreate_RejectsNonPublicURL(t *testing.T) { /* expects 400, code validation_failed */ }

func TestCreate_RejectsPlainHTTP(t *testing.T) { /* expects 400 */ }

func TestCreate_RejectsUnknownEventType(t *testing.T) {
	// Only the 18 constants in internal/outbox are selectable. An unknown
	// type would silently never fire, which reads to a merchant as a broken
	// webhook rather than a typo.
}

func TestCreate_RejectsMoreThanMaxEventTypes(t *testing.T) { /* MaxEventTypes + 1 → 400 */ }

func TestCreate_ReturnsTheSecretExactlyOnce(t *testing.T) {
	// The create response carries `secret`; a subsequent GET must not.
}

func TestList_ScopesToTheCallersTenantAndStore(t *testing.T) {
	// A subscription belonging to another tenant must not be returned.
}

func TestReplay_ResetsAFailedDeliveryToPendingAndDueNow(t *testing.T) {}
```

- [ ] **Step 2: Run and watch them fail**

Run: `go test ./internal/handlers/admin/ -run TestCreate -v`
Expected: FAIL — handler undefined.

- [ ] **Step 3: Implement the handler**

Key requirements, each with a reason:

```go
// allowedEventTypes is the closed set a subscription may select, built from
// internal/outbox's constants. Validating against it means a typo is a 400
// at registration rather than a webhook that silently never fires.
var allowedEventTypes = map[string]bool{
	outbox.EventOrderPlaced: true, outbox.EventOrderConfirmed: true,
	outbox.EventOrderFulfilled: true, outbox.EventOrderPartiallyFulfilled: true,
	outbox.EventOrderCancelled: true, outbox.EventOrderRefunded: true,
	outbox.EventReturnRequested: true, outbox.EventReturnApproved: true,
	outbox.EventReturnReceived: true, outbox.EventReturnRefunded: true,
	outbox.EventReturnRejected: true,
	outbox.EventProductCreated: true, outbox.EventProductUpdated: true,
	outbox.EventProductDeleted: true,
	outbox.EventCategoryCreated: true, outbox.EventCategoryUpdated: true,
	outbox.EventCategoryDeleted: true,
	outbox.EventAbandonedCartRecoveryEmail: true,
}
```

- Bind with Gin tags; return the repo's standard error envelope via `apperrors.CodeValidationFailed` on any rejection — match the shape used by neighbouring admin handlers, do not invent one.
- Run `ssrfguard.Check` on create and on update. Map `ErrNotHTTPS`, `ErrPrivateAddress`, `ErrUnresolvable` to distinct, human messages: a merchant needs to know *which* rule they hit.
- `GenerateSecret()` on create; return it in the create response only. `Subscription.Secret` is `json:"-"`, so reads can't leak it.
- Every query scopes on the tenant and store from the auth context. **No handler may take a tenant id from the request body.**
- Test-send builds a synthetic `Delivery{EventType: "webhook.test"}` and calls `Sender.Send` directly, returning the status code and any error to the caller so a merchant can debug without waiting for a real event.
- Replay sets `status='pending'`, `attempts=0`, `next_attempt_at=now()` for a delivery belonging to a subscription in the caller's store.

- [ ] **Step 4: Register the routes**

In `main.go`, alongside the other admin handler registrations, inside the authenticated admin group. Follow the surrounding registration style exactly.

- [ ] **Step 5: Run and watch them pass**

Run: `go test ./internal/handlers/admin/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/webhooks.go services/marketplace-api/internal/handlers/admin/webhooks_test.go services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(webhooks): admin API for subscriptions, test sends and delivery replay"
```

---

### Task 8: Delivery pruning

**Files:**
- Create: `services/marketplace-api/cmd/webhook-delivery-prune-cron/main.go`
- Test: `services/marketplace-api/internal/webhook/prune_integration_test.go`
- Modify: root `Makefile` (add `./internal/webhook/...` to `test-int` if Task 2 didn't)

**Interfaces:**
- Consumes: `DeliveryRepo.Prune`, `RetentionWindow` (Task 5)
- Produces: a CronJob entrypoint following `cmd/refund-sweep-cron` exactly

- [ ] **Step 1: Write the failing test**

```go
//go:build integration

func TestPrune_RemovesRowsPastRetentionAndKeepsRecentOnes(t *testing.T) {
	db := testdb.NewDB(t)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()
	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})

	_, err := deliveries.FanOut(ctx, []webhook.Delivery{
		{SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed", AggregateID: uuid.New(), Status: webhook.StatusDelivered},
		{SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed", AggregateID: uuid.New(), Status: webhook.StatusDelivered},
	})
	require.NoError(t, err)

	// Age exactly one row past the window.
	require.NoError(t, db.Exec(`
		UPDATE webhook_deliveries SET created_at = now() - interval '31 days'
		 WHERE id = (SELECT id FROM webhook_deliveries LIMIT 1)`).Error)

	n, err := deliveries.Prune(ctx, webhook.RetentionWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	var remaining int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&remaining).Error)
	require.EqualValues(t, 1, remaining, "a delivery inside the window must survive")
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `TEST_DATABASE_URL="$DSN" go test -tags=integration ./internal/webhook/ -run TestPrune -v`

- [ ] **Step 3: Write the cron entrypoint**

Copy the structure of `cmd/refund-sweep-cron/main.go`: load config, open the DB, call `Prune(ctx, webhook.RetentionWindow)`, log the count, exit non-zero on error.

- [ ] **Step 4: Run and watch it pass, then commit**

```bash
git add services/marketplace-api/cmd/webhook-delivery-prune-cron/ services/marketplace-api/internal/webhook/prune_integration_test.go Makefile
git commit -m "feat(webhooks): prune delivery rows past the 30-day retention window"
```

Note: the CronJob manifest lives in `tesserix-k8s`, not this repo — a separate PR modelled on the existing `refund-sweep` CronJob.

---

### Task 9: Admin UI

**Files:**
- Create: `apps/admin/app/(dashboard)/settings/webhooks/page.tsx`
- Create: `apps/admin/components/settings/WebhooksSettingsClient.tsx`
- Create: `apps/admin/lib/api/webhooks.ts`
- Test: `apps/admin/tests/unit/components/settings/WebhooksSettingsClient.test.tsx`

**Interfaces:**
- Consumes: the Task 7 admin API
- Produces: nothing consumed by later tasks

- [ ] **Step 1: Write the failing component test**

Follow the existing `apps/admin/tests/unit/components/**` conventions (vitest + Testing Library). Cover:

- The secret is shown once after creation, with a copy control and a warning that it will not be shown again
- A disabled subscription renders its `disabled_reason` prominently — a merchant must be able to see *why* without opening a delivery
- The delivery list shows status and response code, and offers replay only on failed deliveries
- Test-send reports the status code it got back
- Form errors from the API (`ErrPrivateAddress`, `ErrNotHTTPS`) surface as readable messages, wired with `aria-describedby`

- [ ] **Step 2: Run and watch them fail**

Run: `cd apps/admin && npx vitest run tests/unit/components/settings/WebhooksSettingsClient.test.tsx`

- [ ] **Step 3: Build the UI**

Read `apps/admin/components/settings/BrandingSettingsClient.tsx` first and follow its structure — section headers, `ToggleSwitch`, the save pattern, `@tesserix/web` primitives. Design per the repo root CLAUDE.md: Paper/Ink/Moss, Source Serif 4 display / Source Sans 3 UI, hairline rules rather than bordered cards, one accent, light mode only, WCAG 2.1 AA.

Event-type selection is a checkbox group over the 18 types, grouped by aggregate (order / return / product / category), with the aggregate name as a group label so the list reads rather than sprawls.

- [ ] **Step 4: Run and watch them pass**

```bash
cd apps/admin && npx vitest run && npx tsc --noEmit
```

Note: `@tesserix/web` and `@tesserix/otto-widget` come from GitHub Packages and may be missing from `node_modules`, which makes some specs fail for reasons unrelated to your change. Confirm any failure reproduces on unmodified `origin/main` (`git stash`, re-run, `git stash pop`) before attributing it to yourself.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/app/\(dashboard\)/settings/webhooks/ apps/admin/components/settings/WebhooksSettingsClient.tsx apps/admin/lib/api/webhooks.ts apps/admin/tests/unit/components/settings/WebhooksSettingsClient.test.tsx
git commit -m "feat(webhooks): admin UI for subscriptions, test sends and delivery history"
```

---

### Task 10: Narrow the pricing guard and document verification

**Files:**
- Modify: `apps/onboarding/tests/unit/pricing-surfaces-truth.spec.ts`
- Create: `docs/webhooks.md`

- [ ] **Step 1: Narrow the guard, do not delete it**

`pricing-surfaces-truth.spec.ts` currently fails on any pricing surface mentioning **webhooks or code injection**. Webhooks now exist; custom code injection still does not. Remove only the webhook assertion, keep the code-injection one, and update the failure message to say why the two were separated.

Note: webhooks are on **every** plan, so this does not license restoring a *Studio* "webhooks" bullet — it is not a tier differentiator. If plan copy changes at all, both public surfaces must change together (`Pricing.tsx` and `admin/lib/copy/pricing.ts`).

- [ ] **Step 2: Write the merchant-facing verification recipe**

`docs/webhooks.md`: the event catalogue, the payload shape, and a worked signature-verification example showing how to recompute `v1` over `"<t>.<body>"` and reject a stale `t`. Without this the signature is undocumented and merchants will skip verification.

- [ ] **Step 3: Run the onboarding suite and commit**

```bash
cd apps/onboarding && npx playwright test --config=playwright.unit.config.ts
git add apps/onboarding/tests/unit/pricing-surfaces-truth.spec.ts docs/webhooks.md
git commit -m "docs(webhooks): verification recipe, and narrow the pricing guard to code injection"
```

---

## Before opening the PR

- [ ] `go build ./... && go vet ./... && gofmt -l .`
- [ ] Full integration suite on a throwaway database: `TEST_DATABASE_URL="$DSN" go test -tags=integration -p 1 -count=1 ./...` — **every** package, not just `internal/webhook`
- [ ] Down migration round-trips: `go run ./cmd/migrate down 1 && go run ./cmd/migrate up`
- [ ] `cd apps/admin && npx tsc --noEmit`
- [ ] `docker rm -f m8-wh`
- [ ] Confirm no log line anywhere carries a webhook URL's response body, a signing secret, or a subscriber email
