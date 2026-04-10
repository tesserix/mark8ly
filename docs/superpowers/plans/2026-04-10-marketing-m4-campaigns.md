# Marketing M4 — Campaigns Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship campaign CRUD (admin), customer segments, campaign send/schedule/pause with batch dispatch, delivery analytics, and stuck-campaign recovery. 3 starter templates for content creation.

**Architecture:** New `internal/campaign/` package (models, repository, service, segment engine, send worker). Migration 000012. Batch recipient dispatch (500/batch, 1s delay). Content sanitized via existing product/sanitizer. Campaign heartbeat + stuck recovery (same pattern as csvjob).

**Tech Stack:** Go 1.26, Gin, GORM, shopspring/decimal, bluemonday (sanitizer). Next.js 16, React 19, Tailwind.

**Prerequisite:** M3 (Loyalty) must be on main — segments query customer_loyalties.

---

## Status

> **Pending.** All tasks open. Branch: `feat/marketing-m4-campaigns`.

---

## Scope check

Adds `services/marketplace-api/internal/campaign/` (models, repository, service, segment engine, send worker), migration `000012_campaigns.up.sql`, admin handlers for campaigns CRUD + segments CRUD, send/schedule/pause actions, and the admin UI campaign management pages. Extends `admin.Deps` and `RegisterAdmin` route table with campaign + segment groups.

Spec sections authoritative for this milestone:
- Design spec §3.4 (Migration 000012 — Campaigns)
- Design spec §4.4 (Campaign API endpoints)
- Design spec §6.2 (Admin UI pages for campaigns)
- Design spec §8.7 (Content sanitization)
- Design spec §8.8 (Background jobs — campaign send)
- Design spec §8.1 (Atomic analytics counter mutations)
- Design spec §10.3 (Campaign wizard — 3 starter templates)
- Design spec §10.4 (Empty states + confirmation dialog)
- `mark8ly/.impeccable.md` — Paper · Ink · Moss design context

**Out of scope (deferred):**
- Email template builder/WYSIWYG — plain text/HTML + 3 templates only
- A/B testing for campaigns
- SMS and push notification channels (schema supports type column, but M4 implements email only)
- Advanced segment builder UI with drag-and-drop rules
- Automatic batch coupon code generation

---

## Decisions locked (from the spec — do NOT re-debate)

1. **Worker pattern:** in-process goroutine (same pattern as csvjob), **NOT** Pub/Sub.
2. **Batch dispatch:** 500 recipients per batch, 1s inter-batch delay.
3. **Stuck recovery:** heartbeat_at on campaigns row, stale after 15 min → status reset to `paused` on startup. `FOR UPDATE SKIP LOCKED` prevents cross-pod races.
4. **Content sanitization:** route through `internal/product/sanitizer.go` (bluemonday) before storage and before send. No new sanitizer.
5. **Analytics counters:** atomic SQL increments (`SET delivered = delivered + 1`), never ORM read-modify-write.
6. **Segment engine:** resolves rules against `customer_loyalties` + `orders` tables. Returns list of email addresses.
7. **Campaign types:** email only for M4 (schema supports `sms` and `push` but not implemented).
8. **Templates:** 3 starter templates — "Announce a sale", "Re-engage inactive customers", "Welcome new subscribers".
9. **Design system:** Paper · Ink · Moss tokens, Source Serif 4 display, Source Sans 3 body, `@tesserix/web` primitives first.

---

## File structure produced by M4

### New backend files

```
services/marketplace-api/
  migrations/
    000012_campaigns.up.sql
    000012_campaigns.down.sql
  internal/campaign/
    models.go                      Campaign, CustomerSegment, CampaignRecipient, status constants, segment rules
    repository.go                  CRUD + CreateRecipientsInBatches + IncrementAnalytics + heartbeat
    repository_test.go
    service.go                     Create, Update, Schedule, Send, Pause, Resume, analytics
    service_test.go
    segment_engine.go              Resolve segment rules → email list from customer_loyalties + orders
    segment_engine_test.go
    send_worker.go                 Background goroutine, heartbeat, stuck recovery, batch dispatch
    send_worker_test.go
  internal/handlers/admin/
    campaigns.go                   CampaignHandler — CRUD + send/schedule/pause
    campaigns_dto.go               Request/response DTOs + mappers
    segments.go                    SegmentHandler — list + create
    segments_dto.go                Request/response DTOs
  internal/authz/
    campaign_roles.go              Role constants for campaign + segment endpoints
```

### Modified backend files

```
services/marketplace-api/
  migrations.go                    Bump ExpectedSchemaVersion to 12
  internal/handlers/admin/routes.go   Add CampaignHandler + SegmentHandler to Deps, register routes
  cmd/marketplace-api/main.go         Wire campaign package, start send worker goroutine
  pkg/apperrors/errors.go             Add campaign error codes
```

### New frontend files

```
apps/admin/
  lib/api/campaigns.ts                 API client for campaigns + segments
  app/marketing/campaigns/page.tsx     Campaign list page
  app/marketing/campaigns/new/page.tsx Campaign create wizard (3-step)
  app/marketing/campaigns/[id]/page.tsx Campaign detail + analytics
  app/marketing/segments/page.tsx      Segment list page
  app/marketing/segments/new/page.tsx  Segment create page
  components/marketing/
    CampaignList.tsx                   Client-side list with status filters
    CampaignWizard.tsx                 3-step wizard: audience → content → schedule
    CampaignTemplates.tsx              3 starter templates
    CampaignAnalytics.tsx              Delivery analytics display
    CampaignSendDialog.tsx             Confirmation dialog ("Send to N recipients?")
    SegmentList.tsx                     Segment list component
    SegmentForm.tsx                     Segment create form
```

### Modified frontend files

```
apps/admin/
  components/shell/AdminShell.tsx      Update marketing sidebar hrefs from /dashboard to real routes
```

---

## Task 1 — Migration 000012

**What:** Create the SQL migration for `customer_segments`, `campaigns`, and `campaign_recipients` tables.

**Files to create:**
- `services/marketplace-api/migrations/000012_campaigns.up.sql`
- `services/marketplace-api/migrations/000012_campaigns.down.sql`

**Files to modify:**
- `services/marketplace-api/migrations.go` — change `ExpectedSchemaVersion` from current value to `12`

### Steps

- [ ] **1.1** Create `services/marketplace-api/migrations/000012_campaigns.up.sql` with exact SQL from spec §3.4 plus the following additions from spec §9:
  - Add `tenant_id UUID NOT NULL` to `campaign_recipients` for consistency
  - Add `heartbeat_at TIMESTAMPTZ` to `campaigns` for stuck recovery (same as csvjob pattern)

```sql
-- 000012_campaigns.up.sql

CREATE TABLE customer_segments (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID         NOT NULL,
    store_id    UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    rules       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    member_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE campaigns (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name            VARCHAR(200) NOT NULL,
    type            VARCHAR(20)  NOT NULL DEFAULT 'email',
    status          VARCHAR(20)  NOT NULL DEFAULT 'draft',
    subject         VARCHAR(300),
    content         TEXT,
    segment_id      UUID         REFERENCES customer_segments(id),
    coupon_id       UUID         REFERENCES coupons(id),
    scheduled_at    TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    heartbeat_at    TIMESTAMPTZ,
    total_recipients INT         NOT NULL DEFAULT 0,
    delivered       INT          NOT NULL DEFAULT 0,
    opened          INT          NOT NULL DEFAULT 0,
    clicked         INT          NOT NULL DEFAULT 0,
    converted       INT          NOT NULL DEFAULT 0,
    unsubscribed    INT          NOT NULL DEFAULT 0,
    failed          INT          NOT NULL DEFAULT 0,
    revenue         NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX campaigns_store_status_idx ON campaigns (store_id, status);

CREATE TABLE campaign_recipients (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    campaign_id     UUID         NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    customer_email  VARCHAR(300) NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    sent_at         TIMESTAMPTZ,
    opened_at       TIMESTAMPTZ,
    clicked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX cr_campaign_idx ON campaign_recipients (campaign_id);
CREATE INDEX cr_email_idx ON campaign_recipients (customer_email);
```

- [ ] **1.2** Create `services/marketplace-api/migrations/000012_campaigns.down.sql`:

```sql
DROP TABLE IF EXISTS campaign_recipients;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS customer_segments;
```

- [ ] **1.3** Update `services/marketplace-api/migrations.go` — change `ExpectedSchemaVersion` to `12`.

> **NOTE:** Migration 000012 references `coupons(id)` FK — the coupons table must exist from M1 migration 000009. If migrations 000009-000011 are not yet on main, replace the `coupon_id` FK with a plain `coupon_id UUID` column (no REFERENCES) and add the FK in a later migration. Add a TODO comment.

- [ ] **1.4** Run migration locally and verify:

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
make mp-migrate-up
```

### TDD

- [ ] **1.5** Write test: verify the migration applies cleanly and tables exist. Use the pattern from existing migration tests if present, or manually verify with `SELECT 1 FROM customer_segments LIMIT 0` etc.

### Commit

```
feat(marketplace-api): add migration 000012 for campaigns, segments, recipients
```

---

## Task 2 — Campaign models

**What:** Define GORM models for Campaign, CustomerSegment, and CampaignRecipient. Follow the pattern from `internal/order/models.go`.

**File to create:** `services/marketplace-api/internal/campaign/models.go`

### Steps

- [ ] **2.1** Create `services/marketplace-api/internal/campaign/models.go`:

```go
package campaign

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// Campaign status constants.
const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusPaused    = "paused"
	StatusCancelled = "cancelled"
)

// Campaign type constants.
const (
	TypeEmail = "email"
)

// Recipient status constants.
const (
	RecipientPending      = "pending"
	RecipientSent         = "sent"
	RecipientDelivered    = "delivered"
	RecipientOpened       = "opened"
	RecipientClicked      = "clicked"
	RecipientBounced      = "bounced"
	RecipientUnsubscribed = "unsubscribed"
)

// CustomerSegment defines a reusable audience filter. Rules is a JSONB
// array of rule objects; the segment engine resolves them to email lists.
type CustomerSegment struct {
	ID          uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID     uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	Name        string         `gorm:"column:name;type:varchar(200);not null"`
	Description *string        `gorm:"column:description;type:text"`
	Rules       datatypes.JSON `gorm:"column:rules;type:jsonb;not null;default:'[]'::jsonb"`
	MemberCount int            `gorm:"column:member_count;type:int;not null;default:0"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomerSegment) TableName() string { return "customer_segments" }

// Campaign is the root aggregate for email campaigns.
type Campaign struct {
	ID               uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	Name             string          `gorm:"column:name;type:varchar(200);not null"`
	Type             string          `gorm:"column:type;type:varchar(20);not null;default:email"`
	Status           string          `gorm:"column:status;type:varchar(20);not null;default:draft"`
	Subject          *string         `gorm:"column:subject;type:varchar(300)"`
	Content          *string         `gorm:"column:content;type:text"`
	SegmentID        *uuid.UUID      `gorm:"column:segment_id;type:uuid"`
	CouponID         *uuid.UUID      `gorm:"column:coupon_id;type:uuid"`
	ScheduledAt      *time.Time      `gorm:"column:scheduled_at"`
	SentAt           *time.Time      `gorm:"column:sent_at"`
	HeartbeatAt      *time.Time      `gorm:"column:heartbeat_at"`
	TotalRecipients  int             `gorm:"column:total_recipients;not null;default:0"`
	Delivered        int             `gorm:"column:delivered;not null;default:0"`
	Opened           int             `gorm:"column:opened;not null;default:0"`
	Clicked          int             `gorm:"column:clicked;not null;default:0"`
	Converted        int             `gorm:"column:converted;not null;default:0"`
	Unsubscribed     int             `gorm:"column:unsubscribed;not null;default:0"`
	Failed           int             `gorm:"column:failed;not null;default:0"`
	Revenue          decimal.Decimal `gorm:"column:revenue;type:numeric(12,2);not null;default:0"`
	CreatedAt        time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (Campaign) TableName() string { return "campaigns" }

// IsTerminal reports whether the campaign is in a final state.
func (c Campaign) IsTerminal() bool {
	return c.Status == StatusSent || c.Status == StatusCancelled
}

// CampaignRecipient tracks per-recipient delivery state.
type CampaignRecipient struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID      uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	CampaignID    uuid.UUID  `gorm:"column:campaign_id;type:uuid;not null"`
	CustomerEmail string     `gorm:"column:customer_email;type:varchar(300);not null"`
	Status        string     `gorm:"column:status;type:varchar(20);not null;default:pending"`
	SentAt        *time.Time `gorm:"column:sent_at"`
	OpenedAt      *time.Time `gorm:"column:opened_at"`
	ClickedAt     *time.Time `gorm:"column:clicked_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:now()"`
}

func (CampaignRecipient) TableName() string { return "campaign_recipients" }

// SegmentRule is the Go representation of a single rule in the segments
// rules JSONB array. The segment engine interprets these.
//
// Supported rule types for M4:
//   - "loyalty_tier" — field: tier value (e.g., "gold", "silver")
//   - "has_ordered" — customers with at least one order in the store
//   - "inactive_days" — no order in N days (value is string int, e.g., "90")
//   - "all" — all customers in customer_loyalties for this store
type SegmentRule struct {
	Type  string `json:"type"`   // rule type
	Field string `json:"field"`  // optional field qualifier
	Value string `json:"value"`  // filter value
}
```

### TDD

- [ ] **2.2** Write `services/marketplace-api/internal/campaign/models_test.go`:
  - Test `Campaign.IsTerminal()` returns true for "sent" and "cancelled", false for others.
  - Test `TableName()` returns correct table names for all three models.

### Commit

```
feat(campaign): add GORM models for Campaign, CustomerSegment, CampaignRecipient
```

---

## Task 3 — Repository

**What:** Data access layer with CRUD, batch recipient insert, atomic analytics counter increment, and heartbeat update. Follow the pattern from `internal/order/repository.go` — every mutating method takes an explicit `*gorm.DB` so callers can thread their own transaction.

**File to create:** `services/marketplace-api/internal/campaign/repository.go`

### Steps

- [ ] **3.1** Create `services/marketplace-api/internal/campaign/repository.go`:

```go
package campaign

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for campaigns, segments, and
// recipients. Read methods take context + db. Mutating methods take
// explicit *gorm.DB so callers can pass their own transaction.
type Repository interface {
	// --- Segments ---
	CreateSegment(tx *gorm.DB, s *CustomerSegment) error
	ListSegmentsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID) ([]CustomerSegment, error)
	GetSegmentByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*CustomerSegment, error)
	UpdateSegmentMemberCount(tx *gorm.DB, id uuid.UUID, count int) error

	// --- Campaigns ---
	CreateCampaign(tx *gorm.DB, c *Campaign) error
	GetCampaignByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Campaign, error)
	ListCampaignsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID, status string, page, pageSize int) ([]Campaign, int64, error)
	UpdateCampaign(tx *gorm.DB, c *Campaign) error
	UpdateCampaignStatus(tx *gorm.DB, id uuid.UUID, status string) error
	DeleteCampaign(tx *gorm.DB, id uuid.UUID) error

	// UpdateHeartbeat sets heartbeat_at = now() for the given campaign.
	UpdateHeartbeat(tx *gorm.DB, id uuid.UUID) error

	// IncrementAnalytics atomically increments a single analytics counter
	// column. column must be one of: delivered, opened, clicked, converted,
	// unsubscribed, failed. Uses raw SQL UPDATE ... SET col = col + 1.
	IncrementAnalytics(tx *gorm.DB, id uuid.UUID, column string) error

	// SetSentAt marks campaign as sent with a timestamp.
	SetSentAt(tx *gorm.DB, id uuid.UUID, sentAt time.Time) error

	// FindStuckCampaigns returns campaigns with status='sending' whose
	// heartbeat is stale. Uses FOR UPDATE SKIP LOCKED.
	FindStuckCampaigns(ctx context.Context, db *gorm.DB, staleDuration time.Duration) ([]Campaign, error)

	// --- Recipients ---
	CreateRecipientsInBatches(tx *gorm.DB, recipients []CampaignRecipient, batchSize int) error
	ListRecipientsByCampaign(ctx context.Context, db *gorm.DB, campaignID uuid.UUID, status string, page, pageSize int) ([]CampaignRecipient, int64, error)
	GetPendingRecipients(ctx context.Context, db *gorm.DB, campaignID uuid.UUID, limit int) ([]CampaignRecipient, error)
	UpdateRecipientStatus(tx *gorm.DB, id uuid.UUID, status string) error
	CountRecipientsByCampaign(ctx context.Context, db *gorm.DB, campaignID uuid.UUID) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a campaign repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }
```

- [ ] **3.2** Implement all methods. Key patterns:

**CreateRecipientsInBatches** — use GORM's `CreateInBatches`:
```go
func (r *gormRepository) CreateRecipientsInBatches(tx *gorm.DB, recipients []CampaignRecipient, batchSize int) error {
	if len(recipients) == 0 {
		return nil
	}
	return tx.CreateInBatches(&recipients, batchSize).Error
}
```

**IncrementAnalytics** — atomic SQL, never read-modify-write:
```go
func (r *gormRepository) IncrementAnalytics(tx *gorm.DB, id uuid.UUID, column string) error {
	// Allowlist of valid columns to prevent SQL injection.
	allowed := map[string]bool{
		"delivered": true, "opened": true, "clicked": true,
		"converted": true, "unsubscribed": true, "failed": true,
	}
	if !allowed[column] {
		return fmt.Errorf("campaign: invalid analytics column %q", column)
	}
	res := tx.Model(&Campaign{}).Where("id = ?", id).
		Update(column, gorm.Expr(column+" + 1"))
	return res.Error
}
```

**FindStuckCampaigns** — same pattern as csvjob's `FindOrphanedJobs`:
```go
func (r *gormRepository) FindStuckCampaigns(ctx context.Context, db *gorm.DB, staleDuration time.Duration) ([]Campaign, error) {
	var campaigns []Campaign
	cutoff := time.Now().Add(-staleDuration)
	err := db.WithContext(ctx).
		Raw("SELECT * FROM campaigns WHERE status = ? AND (heartbeat_at IS NULL OR heartbeat_at < ?) FOR UPDATE SKIP LOCKED",
			StatusSending, cutoff).
		Scan(&campaigns).Error
	return campaigns, err
}
```

**UpdateHeartbeat:**
```go
func (r *gormRepository) UpdateHeartbeat(tx *gorm.DB, id uuid.UUID) error {
	res := tx.Model(&Campaign{}).Where("id = ?", id).
		Update("heartbeat_at", gorm.Expr("now()"))
	if res.Error != nil {
		return fmt.Errorf("campaign: update heartbeat: %w", res.Error)
	}
	return nil
}
```

**ListCampaignsByStore** — with pagination and optional status filter:
```go
func (r *gormRepository) ListCampaignsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID, status string, page, pageSize int) ([]Campaign, int64, error) {
	var campaigns []Campaign
	var total int64
	q := db.WithContext(ctx).Model(&Campaign{}).Where("store_id = ?", storeID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&campaigns).Error
	return campaigns, total, err
}
```

### TDD

- [ ] **3.3** Write `services/marketplace-api/internal/campaign/repository_test.go`:
  - Test `CreateCampaign` + `GetCampaignByID` round-trip.
  - Test `CreateRecipientsInBatches` with 1500 recipients (3 batches of 500).
  - Test `IncrementAnalytics` — create campaign, increment "delivered" 3 times, assert delivered=3.
  - Test `IncrementAnalytics` with invalid column returns error.
  - Test `FindStuckCampaigns` — create a campaign with status=sending + stale heartbeat, verify it's returned. Create one with fresh heartbeat, verify it's NOT returned.
  - Test `ListCampaignsByStore` with status filter and pagination.

### Commit

```
feat(campaign): add repository with batch insert, atomic analytics, stuck recovery
```

---

## Task 4 — Segment engine

**What:** Resolve segment rules to a list of customer emails. Queries `customer_loyalties` (from M3) and `orders` tables.

**File to create:** `services/marketplace-api/internal/campaign/segment_engine.go`

### Steps

- [ ] **4.1** Create `services/marketplace-api/internal/campaign/segment_engine.go`:

```go
package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SegmentEngine resolves segment rules to email addresses. It queries
// customer_loyalties and orders tables.
type SegmentEngine struct {
	db *gorm.DB
}

// NewSegmentEngine constructs the engine.
func NewSegmentEngine(db *gorm.DB) *SegmentEngine {
	return &SegmentEngine{db: db}
}

// ResolveEmails parses the rules JSONB and returns matching customer
// emails for the given store. If multiple rules exist, they are ANDed
// (intersection). If rules is empty or contains a single "all" rule,
// returns all enrolled customers.
func (e *SegmentEngine) ResolveEmails(ctx context.Context, storeID uuid.UUID, rulesJSON []byte) ([]string, error) {
	var rules []SegmentRule
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		return nil, fmt.Errorf("segment: invalid rules JSON: %w", err)
	}

	if len(rules) == 0 {
		return e.allEnrolled(ctx, storeID)
	}

	// Build result set — start with all enrolled, filter down per rule.
	var emails []string
	for i, rule := range rules {
		var ruleEmails []string
		var err error
		switch rule.Type {
		case "all":
			ruleEmails, err = e.allEnrolled(ctx, storeID)
		case "loyalty_tier":
			ruleEmails, err = e.byLoyaltyTier(ctx, storeID, rule.Value)
		case "has_ordered":
			ruleEmails, err = e.hasOrdered(ctx, storeID)
		case "inactive_days":
			days, parseErr := strconv.Atoi(rule.Value)
			if parseErr != nil {
				return nil, fmt.Errorf("segment: invalid inactive_days value %q: %w", rule.Value, parseErr)
			}
			ruleEmails, err = e.inactiveDays(ctx, storeID, days)
		default:
			return nil, fmt.Errorf("segment: unknown rule type %q", rule.Type)
		}
		if err != nil {
			return nil, err
		}
		if i == 0 {
			emails = ruleEmails
		} else {
			emails = intersect(emails, ruleEmails)
		}
	}
	return emails, nil
}

func (e *SegmentEngine) allEnrolled(ctx context.Context, storeID uuid.UUID) ([]string, error) {
	var emails []string
	err := e.db.WithContext(ctx).
		Raw("SELECT customer_email FROM customer_loyalties WHERE store_id = ?", storeID).
		Scan(&emails).Error
	return emails, err
}

func (e *SegmentEngine) byLoyaltyTier(ctx context.Context, storeID uuid.UUID, tier string) ([]string, error) {
	var emails []string
	err := e.db.WithContext(ctx).
		Raw("SELECT customer_email FROM customer_loyalties WHERE store_id = ? AND tier = ?", storeID, tier).
		Scan(&emails).Error
	return emails, err
}

func (e *SegmentEngine) hasOrdered(ctx context.Context, storeID uuid.UUID) ([]string, error) {
	var emails []string
	err := e.db.WithContext(ctx).
		Raw("SELECT DISTINCT customer_email FROM orders WHERE store_id = ? AND status != 'cancelled'", storeID).
		Scan(&emails).Error
	return emails, err
}

func (e *SegmentEngine) inactiveDays(ctx context.Context, storeID uuid.UUID, days int) ([]string, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	// Customers enrolled but whose last order is before cutoff (or no order).
	var emails []string
	err := e.db.WithContext(ctx).
		Raw(`SELECT cl.customer_email FROM customer_loyalties cl
			WHERE cl.store_id = ?
			AND cl.customer_email NOT IN (
				SELECT DISTINCT customer_email FROM orders
				WHERE store_id = ? AND status != 'cancelled' AND placed_at > ?
			)`, storeID, storeID, cutoff).
		Scan(&emails).Error
	return emails, err
}

// intersect returns elements present in both slices.
func intersect(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	var result []string
	for _, v := range a {
		if _, ok := set[v]; ok {
			result = append(result, v)
		}
	}
	return result
}
```

### TDD

- [ ] **4.2** Write `services/marketplace-api/internal/campaign/segment_engine_test.go`:
  - Test `ResolveEmails` with "all" rule returns all customer_loyalties for store.
  - Test `ResolveEmails` with "loyalty_tier" rule filters correctly.
  - Test `ResolveEmails` with "has_ordered" returns distinct emails from orders.
  - Test `ResolveEmails` with "inactive_days" returns enrolled customers with no recent orders.
  - Test `ResolveEmails` with two rules returns intersection.
  - Test `ResolveEmails` with unknown rule type returns error.
  - Test `ResolveEmails` with empty rules returns all enrolled.
  - Test `intersect` helper with overlapping and disjoint slices.

> **NOTE:** These tests need `customer_loyalties` and `orders` tables from M3 and M2 respectively. Use the test database with migrations applied up to 000012.

### Commit

```
feat(campaign): add segment engine to resolve rules to email lists
```

---

## Task 5 — Service layer

**What:** Business logic for campaign lifecycle: create, update, schedule, send (with batching), pause, resume, analytics. Content sanitization before storage.

**File to create:** `services/marketplace-api/internal/campaign/service.go`

### Steps

- [ ] **5.1** Create `services/marketplace-api/internal/campaign/service.go`:

```go
package campaign

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ServiceConfig bundles service dependencies.
type ServiceConfig struct {
	DB            *gorm.DB
	Repo          Repository
	SegmentEngine *SegmentEngine
	Logger        *slog.Logger
}

// Service is the campaign business logic layer.
type Service struct {
	db            *gorm.DB
	repo          Repository
	segmentEngine *SegmentEngine
	logger        *slog.Logger
}

// NewService constructs a campaign Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:            cfg.DB,
		repo:          cfg.Repo,
		segmentEngine: cfg.SegmentEngine,
		logger:        cfg.Logger,
	}
}
```

- [ ] **5.2** Implement `CreateCampaign`:
  - Validate required fields (name, store_id, tenant_id).
  - Sanitize content via `product.Sanitize(content)` before storage.
  - Default type to "email", status to "draft".
  - Call `repo.CreateCampaign`.

- [ ] **5.3** Implement `UpdateCampaign`:
  - Only allow update when status is "draft".
  - Sanitize content before storage.
  - Call `repo.UpdateCampaign`.

- [ ] **5.4** Implement `ScheduleCampaign`:
  - Validate campaign is in "draft" status.
  - Validate `scheduled_at` is in the future.
  - Resolve segment emails and create recipients in batches of 500.
  - Update campaign status to "scheduled", set total_recipients.
  - Update segment member_count.

- [ ] **5.5** Implement `SendCampaign`:
  - Validate campaign is in "draft" or "scheduled" status.
  - If no recipients exist yet, resolve segment and create them.
  - Update status to "sending".
  - Return the campaign (the send worker picks it up).

- [ ] **5.6** Implement `PauseCampaign`:
  - Validate campaign is in "sending" status.
  - Update status to "paused".

- [ ] **5.7** Implement `ResumeCampaign`:
  - Validate campaign is in "paused" status.
  - Update status to "sending".

- [ ] **5.8** Implement `DeleteCampaign`:
  - Only allow delete when status is "draft".
  - Call `repo.DeleteCampaign`.

- [ ] **5.9** Implement segment methods:
  - `CreateSegment` — validate name, rules JSON schema, persist.
  - `ListSegments` — delegate to repo.
  - `PreviewSegment` — resolve rules and return count + sample emails.

- [ ] **5.10** Add campaign-specific error codes to `services/marketplace-api/pkg/apperrors/errors.go`:

```go
// Campaign M4 — added in Marketing M4.
CodeCampaignNotFound      Code = "campaign_not_found"
CodeCampaignNotDraft      Code = "campaign_not_draft"
CodeCampaignNotSending    Code = "campaign_not_sending"
CodeCampaignNotPaused     Code = "campaign_not_paused"
CodeSegmentNotFound       Code = "segment_not_found"
CodeSegmentInvalidRules   Code = "segment_invalid_rules"
CodeCampaignNoRecipients  Code = "campaign_no_recipients"
CodeCampaignSchedulePast  Code = "campaign_schedule_past"
```

### TDD

- [ ] **5.11** Write `services/marketplace-api/internal/campaign/service_test.go`:
  - Test `CreateCampaign` sanitizes content (HTML tags stripped per policy).
  - Test `CreateCampaign` with empty name returns validation error.
  - Test `UpdateCampaign` on non-draft campaign returns `CodeCampaignNotDraft`.
  - Test `ScheduleCampaign` with past time returns `CodeCampaignSchedulePast`.
  - Test `ScheduleCampaign` resolves segment and creates recipients.
  - Test `SendCampaign` on draft campaign transitions to "sending".
  - Test `PauseCampaign` on non-sending campaign returns error.
  - Test `DeleteCampaign` on non-draft campaign returns error.
  - Test `CreateSegment` with invalid rules JSON returns error.

### Commit

```
feat(campaign): add service layer with lifecycle management and content sanitization
```

---

## Task 6 — Send worker

**What:** Background goroutine that picks up campaigns in "sending" status, dispatches to recipients in batches of 500 with 1s delay, maintains heartbeat, and recovers stuck campaigns on startup. Same architecture as the csvjob worker.

**File to create:** `services/marketplace-api/internal/campaign/send_worker.go`

### Steps

- [ ] **6.1** Create `services/marketplace-api/internal/campaign/send_worker.go`:

```go
package campaign

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// SendBatchSize is the number of recipients per dispatch batch.
	SendBatchSize = 500
	// SendBatchDelay is the inter-batch delay.
	SendBatchDelay = 1 * time.Second
	// HeartbeatInterval is the heartbeat update interval.
	HeartbeatInterval = 5 * time.Second
	// StaleDuration is the threshold for stuck campaign detection.
	StaleDuration = 15 * time.Minute
	// PollInterval is the worker poll interval for new sendable campaigns.
	PollInterval = 5 * time.Second
)

// SendWorkerConfig bundles send worker dependencies.
type SendWorkerConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
}

// SendWorker polls for campaigns in "sending" status and dispatches
// recipients in batches.
type SendWorker struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewSendWorker constructs a send worker.
func NewSendWorker(cfg SendWorkerConfig) *SendWorker {
	return &SendWorker{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
	}
}
```

- [ ] **6.2** Implement `RecoverStuckCampaigns` — called once on startup:

```go
// RecoverStuckCampaigns finds campaigns with status='sending' and stale
// heartbeat, resets them to 'paused'. Same pattern as csvjob.RecoverOrphanedJobs.
func RecoverStuckCampaigns(ctx context.Context, repo Repository, db *gorm.DB, staleDuration time.Duration, logger *slog.Logger) error {
	campaigns, err := repo.FindStuckCampaigns(ctx, db, staleDuration)
	if err != nil {
		return fmt.Errorf("campaign: recover stuck: %w", err)
	}
	for _, c := range campaigns {
		logger.Info("campaign: recovering stuck campaign", "campaign_id", c.ID, "heartbeat_at", c.HeartbeatAt)
		if err := repo.UpdateCampaignStatus(db, c.ID, StatusPaused); err != nil {
			logger.Error("campaign: recover stuck", "campaign_id", c.ID, "err", err)
		}
	}
	return nil
}
```

- [ ] **6.3** Implement `Run` — the main polling loop:

```go
// Run starts the send worker polling loop. Blocks until ctx is cancelled.
func (w *SendWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pollAndDispatch(ctx)
		}
	}
}
```

- [ ] **6.4** Implement `pollAndDispatch` — find campaigns in "sending" or "scheduled" (past scheduled_at), process one at a time:

```go
func (w *SendWorker) pollAndDispatch(ctx context.Context) {
	// Check for scheduled campaigns ready to send.
	// ... find campaigns where status='scheduled' AND scheduled_at <= now()
	// ... update their status to 'sending'

	// Find campaigns in 'sending' status.
	campaigns, _, err := w.repo.ListCampaignsByStore(ctx, w.db, uuid.Nil, StatusSending, 1, 10)
	// NOTE: ListCampaignsByStore uses store_id filter; for the worker we
	// need a cross-store query. Add a dedicated method:
	// FindSendableCampaigns(ctx, db) ([]Campaign, error)
	// ... process each campaign
	for _, c := range campaigns {
		if err := w.dispatchCampaign(ctx, c); err != nil {
			w.logger.Error("campaign: dispatch error", "campaign_id", c.ID, "err", err)
		}
	}
	_ = err
}
```

- [ ] **6.5** Implement `dispatchCampaign` — the per-campaign processing loop:

```go
func (w *SendWorker) dispatchCampaign(ctx context.Context, c Campaign) error {
	// Start heartbeat ticker.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go w.heartbeatLoop(heartbeatCtx, c.ID)

	for {
		// Fetch next batch of pending recipients.
		recipients, err := w.repo.GetPendingRecipients(ctx, w.db, c.ID, SendBatchSize)
		if err != nil {
			return fmt.Errorf("fetch pending: %w", err)
		}
		if len(recipients) == 0 {
			break // All dispatched.
		}

		// Dispatch batch.
		for _, r := range recipients {
			// M4 scope: log the "send" rather than calling a real email
			// service. Real Pub/Sub integration is a follow-up.
			w.logger.Info("campaign: dispatching email",
				"campaign_id", c.ID,
				"email", r.CustomerEmail)

			if err := w.repo.UpdateRecipientStatus(w.db, r.ID, RecipientSent); err != nil {
				w.logger.Error("campaign: update recipient", "id", r.ID, "err", err)
				_ = w.repo.IncrementAnalytics(w.db, c.ID, "failed")
				continue
			}
			_ = w.repo.IncrementAnalytics(w.db, c.ID, "delivered")
		}

		// Check for pause/cancel.
		refreshed, err := w.repo.GetCampaignByID(ctx, w.db, c.ID)
		if err != nil {
			return err
		}
		if refreshed.Status == StatusPaused || refreshed.Status == StatusCancelled {
			w.logger.Info("campaign: stopped by user", "campaign_id", c.ID, "status", refreshed.Status)
			return nil
		}

		// Inter-batch delay.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(SendBatchDelay):
		}
	}

	// Mark campaign as sent.
	now := time.Now()
	if err := w.repo.SetSentAt(w.db, c.ID, now); err != nil {
		return err
	}
	return w.repo.UpdateCampaignStatus(w.db, c.ID, StatusSent)
}

func (w *SendWorker) heartbeatLoop(ctx context.Context, campaignID uuid.UUID) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.repo.UpdateHeartbeat(w.db, campaignID); err != nil {
				w.logger.Error("campaign: heartbeat", "campaign_id", campaignID, "err", err)
			}
		}
	}
}
```

- [ ] **6.6** Add `FindSendableCampaigns` to the Repository interface — campaigns in "sending" status across all stores (the worker is not store-scoped):

```go
// Add to Repository interface:
FindSendableCampaigns(ctx context.Context, db *gorm.DB) ([]Campaign, error)

// Add to Repository interface:
FindScheduledReady(ctx context.Context, db *gorm.DB) ([]Campaign, error)
```

Implement:
```go
func (r *gormRepository) FindSendableCampaigns(ctx context.Context, db *gorm.DB) ([]Campaign, error) {
	var campaigns []Campaign
	err := db.WithContext(ctx).
		Where("status = ?", StatusSending).
		Order("updated_at ASC").
		Limit(10).
		Find(&campaigns).Error
	return campaigns, err
}

func (r *gormRepository) FindScheduledReady(ctx context.Context, db *gorm.DB) ([]Campaign, error) {
	var campaigns []Campaign
	err := db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", StatusScheduled, time.Now()).
		Find(&campaigns).Error
	return campaigns, err
}
```

### TDD

- [ ] **6.7** Write `services/marketplace-api/internal/campaign/send_worker_test.go`:
  - Test `RecoverStuckCampaigns` — create campaign with status=sending + stale heartbeat, verify it's reset to paused.
  - Test `RecoverStuckCampaigns` — campaign with fresh heartbeat is NOT reset.
  - Test `dispatchCampaign` — create campaign with 10 recipients, run dispatch, verify all recipients are marked "sent" and delivered counter = 10.
  - Test batch pause — create campaign with 1000 recipients, pause mid-send, verify dispatch stops.
  - Test scheduled campaign activation — create scheduled campaign with past scheduled_at, verify worker transitions it to sending.

### Commit

```
feat(campaign): add send worker with heartbeat, batch dispatch, stuck recovery
```

---

## Task 7 — Content sanitization integration

**What:** Ensure campaign content is routed through the existing `internal/product/sanitizer.go` before storage. This was already wired in the service layer (Task 5), but verify and add an explicit test.

**File to modify:** `services/marketplace-api/internal/campaign/service.go` (verify sanitization calls)

### Steps

- [ ] **7.1** Verify that `CreateCampaign` and `UpdateCampaign` call `product.Sanitize(content)` on the content field before persisting. If not already done in Task 5, add it now.

- [ ] **7.2** Add a targeted integration test in `service_test.go`:
  - Create a campaign with content containing `<script>alert('xss')</script><p>Hello</p>`.
  - Verify the stored content is `<p>Hello</p>` (script tag stripped).
  - Update campaign content with `<img src=x onerror=alert(1)><strong>Bold</strong>`.
  - Verify stored content is `<strong>Bold</strong>` (img with onerror stripped).

### Commit

```
test(campaign): verify content sanitization strips malicious HTML
```

---

## Task 8 — Admin handlers

**What:** HTTP handlers for campaigns CRUD, send/schedule/pause, and segments CRUD. Follow the pattern from `internal/handlers/admin/categories.go`.

### Files to create:
- `services/marketplace-api/internal/handlers/admin/campaigns.go`
- `services/marketplace-api/internal/handlers/admin/campaigns_dto.go`
- `services/marketplace-api/internal/handlers/admin/segments.go`
- `services/marketplace-api/internal/handlers/admin/segments_dto.go`
- `services/marketplace-api/internal/authz/campaign_roles.go`

### Steps

- [ ] **8.1** Create `services/marketplace-api/internal/authz/campaign_roles.go`:

```go
package authz

// CampaignsViewRole gates GET endpoints for campaigns and segments.
var CampaignsViewRole = RoleStaff

// CampaignsEditRole gates POST/PATCH/DELETE and send/schedule/pause.
var CampaignsEditRole = RoleAdmin
```

- [ ] **8.2** Create `services/marketplace-api/internal/handlers/admin/campaigns_dto.go`:

```go
package admin

import (
	"github.com/mark8ly/marketplace-api/internal/campaign"
)

// --- Request DTOs ---

type CreateCampaignRequest struct {
	Name      string  `json:"name" binding:"required"`
	Type      string  `json:"type"`      // defaults to "email"
	Subject   string  `json:"subject"`
	Content   string  `json:"content"`
	SegmentID *string `json:"segment_id"`
	CouponID  *string `json:"coupon_id"`
}

type UpdateCampaignRequest struct {
	Name      *string `json:"name"`
	Subject   *string `json:"subject"`
	Content   *string `json:"content"`
	SegmentID *string `json:"segment_id"`
	CouponID  *string `json:"coupon_id"`
}

type ScheduleCampaignRequest struct {
	ScheduledAt string `json:"scheduled_at" binding:"required"` // RFC3339
}

// --- Response DTOs ---

type CampaignResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	Subject         *string `json:"subject"`
	Content         *string `json:"content"`
	SegmentID       *string `json:"segment_id"`
	CouponID        *string `json:"coupon_id"`
	ScheduledAt     *string `json:"scheduled_at"`
	SentAt          *string `json:"sent_at"`
	TotalRecipients int     `json:"total_recipients"`
	Delivered       int     `json:"delivered"`
	Opened          int     `json:"opened"`
	Clicked         int     `json:"clicked"`
	Converted       int     `json:"converted"`
	Unsubscribed    int     `json:"unsubscribed"`
	Failed          int     `json:"failed"`
	Revenue         string  `json:"revenue"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ToCampaignResponse maps a domain Campaign to its response DTO.
func ToCampaignResponse(c *campaign.Campaign) CampaignResponse {
	// ... map fields, format times as RFC3339, format revenue as string
}
```

- [ ] **8.3** Create `services/marketplace-api/internal/handlers/admin/campaigns.go`:

```go
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CampaignHandler bundles dependencies for campaign admin endpoints.
type CampaignHandler struct {
	svc    *campaign.Service
	repo   campaign.Repository
	logger *slog.Logger
}

// NewCampaignHandler constructs a CampaignHandler.
func NewCampaignHandler(svc *campaign.Service, repo campaign.Repository, logger *slog.Logger) *CampaignHandler {
	return &CampaignHandler{svc: svc, repo: repo, logger: logger}
}

// List handles GET /admin/stores/:storeId/campaigns.
func (h *CampaignHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	status := c.Query("status")
	// Parse page/page_size from query with defaults.
	// Call repo.ListCampaignsByStore.
	// Return JSON { data: [...], meta: { page, page_size, total, total_pages } }.
}

// Create handles POST /admin/stores/:storeId/campaigns.
func (h *CampaignHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	var req CreateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	// Map DTO → service input, call svc.CreateCampaign, return 201 + response.
}

// Get handles GET /admin/stores/:storeId/campaigns/:id.
func (h *CampaignHandler) Get(c *gin.Context) {
	// Parse id, call repo.GetCampaignByID, return response with analytics.
}

// Patch handles PATCH /admin/stores/:storeId/campaigns/:id.
func (h *CampaignHandler) Patch(c *gin.Context) {
	// Parse id + body, call svc.UpdateCampaign, return updated response.
}

// Delete handles DELETE /admin/stores/:storeId/campaigns/:id.
func (h *CampaignHandler) Delete(c *gin.Context) {
	// Parse id, call svc.DeleteCampaign, return 204.
}

// Send handles POST /admin/stores/:storeId/campaigns/:id/send.
func (h *CampaignHandler) Send(c *gin.Context) {
	// Parse id, call svc.SendCampaign, return 200 with updated campaign.
}

// Schedule handles POST /admin/stores/:storeId/campaigns/:id/schedule.
func (h *CampaignHandler) Schedule(c *gin.Context) {
	// Parse id + body (scheduled_at), call svc.ScheduleCampaign, return 200.
}

// Pause handles POST /admin/stores/:storeId/campaigns/:id/pause.
func (h *CampaignHandler) Pause(c *gin.Context) {
	// Parse id, call svc.PauseCampaign, return 200.
}
```

- [ ] **8.4** Create `services/marketplace-api/internal/handlers/admin/segments_dto.go`:

```go
package admin

type CreateSegmentRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Rules       string `json:"rules" binding:"required"` // JSON array string
}

type SegmentResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Rules       string  `json:"rules"`     // JSON array string
	MemberCount int     `json:"member_count"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}
```

- [ ] **8.5** Create `services/marketplace-api/internal/handlers/admin/segments.go`:

```go
package admin

// SegmentHandler bundles dependencies for segment admin endpoints.
type SegmentHandler struct {
	svc    *campaign.Service
	logger *slog.Logger
}

func NewSegmentHandler(svc *campaign.Service, logger *slog.Logger) *SegmentHandler {
	return &SegmentHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/segments.
func (h *SegmentHandler) List(c *gin.Context) {
	// Parse storeID, call svc.ListSegments, return JSON { data: [...] }.
}

// Create handles POST /admin/stores/:storeId/segments.
func (h *SegmentHandler) Create(c *gin.Context) {
	// Parse body, call svc.CreateSegment, return 201.
}
```

### TDD

- [ ] **8.6** Write handler tests (follow existing test patterns if present in the repo):
  - Test `Create` returns 201 with valid body.
  - Test `Create` returns 400 with missing name.
  - Test `List` returns paginated results.
  - Test `Send` on non-draft campaign returns appropriate error.
  - Test `Delete` on non-draft campaign returns error.
  - Test `Schedule` with past date returns 422.

### Commit

```
feat(campaign): add admin HTTP handlers for campaigns and segments CRUD
```

---

## Task 9 — Wire routes + main.go + send worker startup

**What:** Add CampaignHandler + SegmentHandler to `admin.Deps`, register routes in `RegisterAdmin`, wire campaign package in `main.go`, and start the send worker goroutine.

### Files to modify:
- `services/marketplace-api/internal/handlers/admin/routes.go`
- `services/marketplace-api/cmd/marketplace-api/main.go`

### Steps

- [ ] **9.1** Add to `admin.Deps` struct in `routes.go`:

```go
// Add these fields to the Deps struct:
CampaignHandler  *CampaignHandler
SegmentHandler   *SegmentHandler
```

- [ ] **9.2** Register routes in `RegisterAdmin` (add after the abandoned-carts block, before the closing brace):

```go
// Campaigns — marketing M4.
if deps.CampaignHandler != nil {
	campaigns := storeRoute.Group("/campaigns")
	{
		campaigns.GET("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
			deps.CampaignHandler.List)
		campaigns.POST("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.CampaignHandler.Create)
		campaigns.GET("/:id",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
			deps.CampaignHandler.Get)
		campaigns.PATCH("/:id",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.CampaignHandler.Patch)
		campaigns.DELETE("/:id",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.CampaignHandler.Delete)
		campaigns.POST("/:id/send",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.CampaignHandler.Send)
		campaigns.POST("/:id/schedule",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.CampaignHandler.Schedule)
		campaigns.POST("/:id/pause",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.CampaignHandler.Pause)
	}
}

// Segments — marketing M4.
if deps.SegmentHandler != nil {
	segments := storeRoute.Group("/segments")
	{
		segments.GET("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
			deps.SegmentHandler.List)
		segments.POST("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
			deps.SegmentHandler.Create)
	}
}
```

- [ ] **9.3** Wire in `main.go` — add campaign package wiring inside the `if m == mode.Admin || m == mode.Both` block, after the settings handlers wiring (around line 204):

```go
// Campaign wiring (Marketing M4).
campaignRepo := campaign.NewRepository(conn)
segmentEngine := campaign.NewSegmentEngine(conn)
campaignSvc := campaign.NewService(campaign.ServiceConfig{
	DB:            conn,
	Repo:          campaignRepo,
	SegmentEngine: segmentEngine,
	Logger:        log,
})
campaignHandler := admin.NewCampaignHandler(campaignSvc, campaignRepo, log)
segmentHandler := admin.NewSegmentHandler(campaignSvc, log)
```

Add to `adminDeps` assignment:
```go
CampaignHandler:  campaignHandler,
SegmentHandler:   segmentHandler,
```

- [ ] **9.4** Start the campaign send worker goroutine in `main.go` — after the csvjob worker block (around line 310), add:

```go
// Campaign send worker — runs in admin and both modes.
// On startup, recover stuck campaigns (stale heartbeat > 15 min → paused).
// Then poll for sendable campaigns every 5s.
var campaignWorkerDone <-chan struct{}
if m == mode.Admin || m == mode.Both {
	// Recovery scan on startup.
	if err := campaign.RecoverStuckCampaigns(context.Background(), campaignRepo, conn, campaign.StaleDuration, log); err != nil {
		log.Error("campaign: recovery scan failed", "err", err)
		// Non-fatal — proceed without recovery.
	} else {
		log.Info("campaign: recovery scan complete")
	}

	// Polling goroutine.
	campaignDone := make(chan struct{})
	campaignWorkerDone = campaignDone
	sendWorker := campaign.NewSendWorker(campaign.SendWorkerConfig{
		DB:     conn,
		Repo:   campaignRepo,
		Logger: log,
	})
	go func() {
		defer close(campaignDone)
		sendWorker.Run(workerCtx)
	}()
}
```

- [ ] **9.5** Add the `campaign` import to `main.go`:

```go
"github.com/mark8ly/marketplace-api/internal/campaign"
```

- [ ] **9.6** Ensure graceful shutdown waits for the campaign worker (in the shutdown section, alongside the csvjob worker shutdown):

```go
// Wait for campaign worker if running.
if campaignWorkerDone != nil {
	<-campaignWorkerDone
}
```

### TDD

- [ ] **9.7** Run `go build ./cmd/marketplace-api/` to verify compilation. Run existing tests to ensure nothing is broken.

### Commit

```
feat(campaign): wire routes, handlers, and send worker into main.go
```

---

## Task 10 — Admin UI

**What:** Build the campaign management UI in the admin app. Includes: API client, campaign list page, create wizard (3 steps: audience -> content with 3 templates -> schedule), detail page with analytics, segment list, and segment create page.

### Sub-task 10.1 — API client

**File to create:** `apps/admin/lib/api/campaigns.ts`

- [ ] **10.1.1** Create the API client following the pattern from `apps/admin/lib/api/marketplace-api.ts`:

```typescript
// apps/admin/lib/api/campaigns.ts

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

// --- Campaign types ---

export interface AdminCampaign {
  id: string;
  name: string;
  type: string;
  status: "draft" | "scheduled" | "sending" | "sent" | "paused" | "cancelled";
  subject: string | null;
  content: string | null;
  segment_id: string | null;
  coupon_id: string | null;
  scheduled_at: string | null;
  sent_at: string | null;
  total_recipients: number;
  delivered: number;
  opened: number;
  clicked: number;
  converted: number;
  unsubscribed: number;
  failed: number;
  revenue: string;
  created_at: string;
  updated_at: string;
}

export interface AdminSegment {
  id: string;
  name: string;
  description: string | null;
  rules: string;
  member_count: number;
  created_at: string;
  updated_at: string;
}

export interface ListCampaignsResponse {
  data: AdminCampaign[];
  meta: { page: number; page_size: number; total: number; total_pages: number };
}

// --- API functions ---

export async function listCampaigns(
  storeId: string,
  query: { status?: string; page?: number; pageSize?: number },
  session: SessionHeaders,
): Promise<ListCampaignsResponse | null> {
  // Build URL, fetch with headers, handle 401/403/404 → null.
}

export async function getCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<AdminCampaign | null> {
  // GET /api/v1/admin/stores/:storeId/campaigns/:id
}

export async function createCampaign(
  storeId: string,
  body: { name: string; type?: string; subject?: string; content?: string; segment_id?: string; coupon_id?: string },
  session: SessionHeaders,
): Promise<AdminCampaign> {
  // POST /api/v1/admin/stores/:storeId/campaigns
}

export async function updateCampaign(
  storeId: string,
  campaignId: string,
  body: Record<string, unknown>,
  session: SessionHeaders,
): Promise<AdminCampaign> {
  // PATCH /api/v1/admin/stores/:storeId/campaigns/:id
}

export async function deleteCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<void> {
  // DELETE
}

export async function sendCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<AdminCampaign> {
  // POST /api/v1/admin/stores/:storeId/campaigns/:id/send
}

export async function scheduleCampaign(
  storeId: string,
  campaignId: string,
  scheduledAt: string,
  session: SessionHeaders,
): Promise<AdminCampaign> {
  // POST /api/v1/admin/stores/:storeId/campaigns/:id/schedule
}

export async function pauseCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<AdminCampaign> {
  // POST /api/v1/admin/stores/:storeId/campaigns/:id/pause
}

export async function listSegments(
  storeId: string,
  session: SessionHeaders,
): Promise<{ data: AdminSegment[] } | null> {
  // GET /api/v1/admin/stores/:storeId/segments
}

export async function createSegment(
  storeId: string,
  body: { name: string; description?: string; rules: string },
  session: SessionHeaders,
): Promise<AdminSegment> {
  // POST /api/v1/admin/stores/:storeId/segments
}
```

### Sub-task 10.2 — Campaign list page

**Files to create:**
- `apps/admin/app/marketing/campaigns/page.tsx`
- `apps/admin/components/marketing/CampaignList.tsx`

- [ ] **10.2.1** Create `apps/admin/app/marketing/campaigns/page.tsx` following the pattern from `apps/admin/app/products/new/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCampaigns } from "@/lib/api/campaigns";
import { CampaignList } from "@/components/marketing/CampaignList";

export default async function CampaignsPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="mx-auto max-w-5xl px-8 py-16">
          <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
            No store selected
          </h1>
          <p className="mt-4 text-[color:var(--ink-900)] opacity-70">
            Set up a store before managing campaigns.
          </p>
        </main>
      </AdminShell>
    );
  }

  const result = await listCampaigns(currentStore.id, {}, { userId, tenantId });
  const campaigns = result?.data ?? [];
  const meta = result?.meta;

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-5xl px-8 py-8">
        <CampaignList campaigns={campaigns} meta={meta} storeId={currentStore.id} />
      </main>
    </AdminShell>
  );
}
```

- [ ] **10.2.2** Create `apps/admin/components/marketing/CampaignList.tsx`:
  - Table with columns: Name, Status (badge), Recipients, Delivered, Opened, Sent/Scheduled date.
  - Status filter tabs: All, Draft, Scheduled, Sending, Sent, Paused.
  - Empty state: "No campaigns yet" with CTA button "Create your first campaign".
  - Link to `/marketing/campaigns/new` for create.
  - Link to `/marketing/campaigns/[id]` for detail.
  - Paper/Ink/Moss design tokens, Source Serif 4 heading, hairline rules.

### Sub-task 10.3 — Campaign create wizard (3-step)

**Files to create:**
- `apps/admin/app/marketing/campaigns/new/page.tsx`
- `apps/admin/components/marketing/CampaignWizard.tsx`
- `apps/admin/components/marketing/CampaignTemplates.tsx`

- [ ] **10.3.1** Create `apps/admin/app/marketing/campaigns/new/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listSegments } from "@/lib/api/campaigns";
import { CampaignWizard } from "@/components/marketing/CampaignWizard";

export default async function NewCampaignPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) { /* same no-store fallback */ }

  const segmentsResult = await listSegments(currentStore.id, { userId, tenantId });
  const segments = segmentsResult?.data ?? [];

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-8">
        <CampaignWizard
          storeId={currentStore.id}
          segments={segments}
          session={{ userId, tenantId }}
        />
      </main>
    </AdminShell>
  );
}
```

- [ ] **10.3.2** Create `apps/admin/components/marketing/CampaignWizard.tsx`:

The wizard has 3 steps, tracked with `useState`:

**Step 1 — Audience:**
- Campaign name input.
- Segment dropdown (select from existing segments or "All customers").
- Display estimated recipient count.

**Step 2 — Content:**
- 3 starter template cards (see 10.3.3 below). Clicking one pre-fills subject + content.
- Subject input.
- Content textarea (plain HTML editing — no WYSIWYG per spec out-of-scope).
- Optional coupon ID input.

**Step 3 — Schedule:**
- Radio: "Send now" or "Schedule for later".
- If scheduled: datetime picker for `scheduled_at`.
- Review summary: name, segment, recipient count, subject preview.
- Action buttons:
  - "Send now" → calls `createCampaign` then `sendCampaign`. Shows confirmation dialog first.
  - "Schedule" → calls `createCampaign` then `scheduleCampaign`.
  - "Save as draft" → calls `createCampaign` only.

Navigation: "Back" and "Next" buttons between steps. Progress indicator (Step 1 of 3, etc.).

Design: Paper/Ink/Moss tokens, Source Serif 4 for step titles, hairline rule between steps.

- [ ] **10.3.3** Create `apps/admin/components/marketing/CampaignTemplates.tsx`:

```tsx
export interface CampaignTemplate {
  id: string;
  name: string;
  subject: string;
  content: string;
}

export const CAMPAIGN_TEMPLATES: CampaignTemplate[] = [
  {
    id: "announce-sale",
    name: "Announce a sale",
    subject: "Big savings await — our biggest sale starts now",
    content: `<h2>Our Biggest Sale of the Season</h2>
<p>For a limited time, enjoy exclusive discounts across our entire collection. Whether you've had your eye on something special or are looking for something new, now is the perfect time.</p>
<p><strong>Shop now and save before it's gone.</strong></p>`,
  },
  {
    id: "re-engage",
    name: "Re-engage inactive customers",
    subject: "We miss you — here's something special",
    content: `<h2>It's Been a While</h2>
<p>We noticed you haven't visited in a while, and we wanted to reach out. Our collection has grown since your last visit, and we think you'll love what's new.</p>
<p><strong>Come back and see what you've been missing.</strong></p>`,
  },
  {
    id: "welcome",
    name: "Welcome new subscribers",
    subject: "Welcome — glad you're here",
    content: `<h2>Welcome to the Family</h2>
<p>Thank you for joining us. We're a small team passionate about quality, and we're glad to have you along for the journey.</p>
<p>As a welcome, here's a look at some of our most popular items to get you started.</p>`,
  },
];
```

Render as 3 clickable cards in a grid. Selected template has a moss border. Clicking fills the subject/content fields.

- [ ] **10.3.4** Create `apps/admin/components/marketing/CampaignSendDialog.tsx`:

Confirmation dialog using `@tesserix/web` Dialog component:
- Title: "Send campaign?"
- Body: "This will send '{campaignName}' to {recipientCount} recipients. This action cannot be undone."
- Actions: "Cancel" (secondary) and "Send now" (primary, ink-900 background).

### Sub-task 10.4 — Campaign detail page with analytics

**Files to create:**
- `apps/admin/app/marketing/campaigns/[id]/page.tsx`
- `apps/admin/components/marketing/CampaignAnalytics.tsx`

- [ ] **10.4.1** Create `apps/admin/app/marketing/campaigns/[id]/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCampaign } from "@/lib/api/campaigns";
import { CampaignAnalytics } from "@/components/marketing/CampaignAnalytics";
import { notFound } from "next/navigation";

interface Props {
  params: Promise<{ id: string }>;
}

export default async function CampaignDetailPage({ params }: Props) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) { /* no-store fallback */ }

  const campaign = await getCampaign(currentStore.id, id, { userId, tenantId });
  if (!campaign) notFound();

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-5xl px-8 py-8">
        <CampaignAnalytics campaign={campaign} storeId={currentStore.id} session={{ userId, tenantId }} />
      </main>
    </AdminShell>
  );
}
```

- [ ] **10.4.2** Create `apps/admin/components/marketing/CampaignAnalytics.tsx`:
  - Header: Campaign name (Source Serif 4), status badge, action buttons (Pause/Resume/Delete based on status).
  - Content preview section (sanitized HTML rendered in a bordered card).
  - Analytics grid (6 metric cards): Total Recipients, Delivered, Opened, Clicked, Converted, Failed.
  - Each card shows the count and a percentage (count / total_recipients * 100).
  - Revenue display if > 0.
  - Sent/Scheduled date display.
  - If status is "sending", show a progress indicator (delivered / total_recipients).

### Sub-task 10.5 — Segment pages

**Files to create:**
- `apps/admin/app/marketing/segments/page.tsx`
- `apps/admin/app/marketing/segments/new/page.tsx`
- `apps/admin/components/marketing/SegmentList.tsx`
- `apps/admin/components/marketing/SegmentForm.tsx`

- [ ] **10.5.1** Create `apps/admin/app/marketing/segments/page.tsx` — list page following same pattern. Table with columns: Name, Member Count, Created.

- [ ] **10.5.2** Create `apps/admin/components/marketing/SegmentList.tsx` — table component with empty state.

- [ ] **10.5.3** Create `apps/admin/app/marketing/segments/new/page.tsx` — create page.

- [ ] **10.5.4** Create `apps/admin/components/marketing/SegmentForm.tsx`:
  - Name input.
  - Description textarea.
  - Rules builder: dropdown for rule type (All, Loyalty Tier, Has Ordered, Inactive Days), value input per type. Add/remove rules.
  - Preview button → shows count of matching customers.
  - Submit → calls `createSegment`.

### Sub-task 10.6 — Update sidebar

- [ ] **10.6.1** Update `apps/admin/components/shell/AdminShell.tsx` — change the marketing sidebar links from placeholder `/dashboard` to real routes:

```typescript
// Change this:
{
  key: "marketing",
  label: "Marketing",
  icon: Megaphone,
  children: [
    { label: "Campaigns", href: "/dashboard" },
    { label: "Coupons", href: "/dashboard" },
    { label: "Gift Cards", href: "/dashboard" },
    { label: "Loyalty", href: "/dashboard" },
  ],
},

// To this:
{
  key: "marketing",
  label: "Marketing",
  icon: Megaphone,
  children: [
    { label: "Campaigns", href: "/marketing/campaigns" },
    { label: "Segments", href: "/marketing/segments" },
    { label: "Coupons", href: "/dashboard" },         // M1 routes — placeholder until M1 ships
    { label: "Gift Cards", href: "/dashboard" },       // M2 routes — placeholder until M2 ships
    { label: "Loyalty", href: "/dashboard" },          // M3 routes — placeholder until M3 ships
  ],
},
```

### Commit

```
feat(admin): add campaign management UI with wizard, templates, analytics, segments
```

---

## Task 11 — Build verification + commit

**What:** Verify everything compiles and passes tests.

### Steps

- [ ] **11.1** Go backend build:

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go build ./cmd/marketplace-api/
go vet ./...
```

- [ ] **11.2** Go backend tests:

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go test ./internal/campaign/... -v -count=1
```

- [ ] **11.3** Frontend build:

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/admin
npx next build
```

- [ ] **11.4** Fix any build errors or test failures. Iterate until clean.

- [ ] **11.5** If any fixes were needed, create a final fix commit:

```
fix(campaign): address build issues from M4 integration
```

---

## Dependency graph

```
Task 1 (Migration)
  └── Task 2 (Models) ← depends on tables existing
       └── Task 3 (Repository) ← depends on models
            ├── Task 4 (Segment Engine) ← depends on repo for test data setup
            └── Task 5 (Service) ← depends on repo + segment engine
                 ├── Task 6 (Send Worker) ← depends on service + repo
                 ├── Task 7 (Sanitization test) ← depends on service
                 └── Task 8 (Handlers) ← depends on service
                      └── Task 9 (Wiring) ← depends on handlers
                           └── Task 10 (Admin UI) ← depends on routes being wired
                                └── Task 11 (Build verification)
```

Tasks 4, 5 can be started in parallel once Task 3 is done.
Tasks 6, 7, 8 can be started in parallel once Task 5 is done.
Task 10 sub-tasks (10.1–10.6) can be parallelized across agents.

---

## Risk register

| Risk | Mitigation |
|------|-----------|
| M3 (Loyalty) not on main — `customer_loyalties` table missing | Guard: segment engine returns empty list with warning log if table doesn't exist. Migration 000012 can be applied independently (no FK to customer_loyalties). |
| M1 (Coupons) not on main — `coupons(id)` FK in migration | Guard: use plain `coupon_id UUID` column without REFERENCES if coupons table doesn't exist. Add FK later. |
| Send worker blocks Knative scale-to-zero | Same risk as csvjob — accepted. Real mitigation is the minScale patch pattern from M7e, applied if needed. |
| Email dispatch is a no-op in M4 | Accepted. M4 ships the mechanics (batch processing, status tracking, analytics). Real email delivery via notification-service Pub/Sub is a follow-up. |
| Segment queries slow on large datasets | customer_loyalties has `cl_store_tier_idx`; orders has store_id indexes. Monitor and add materialized view if needed. |
