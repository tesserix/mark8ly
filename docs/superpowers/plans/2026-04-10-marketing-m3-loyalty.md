# Marketing M3 — Loyalty Program Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship loyalty program configuration (admin), customer enrollment + points display (storefront), points earn on checkout, points redemption at checkout, referral system, and point expiry background job.

**Architecture:** New `internal/loyalty/` package (models, repository, service, expiry worker). Migration 000011. Points redemption implements `discount.Applier`. Tier builder UI (structured, not JSONB). Background cron for point expiry using csvjob worker pattern.

**Tech Stack:** Go 1.26, Gin, GORM, shopspring/decimal, crypto/rand (referral codes). Next.js 16, React 19, Tailwind.

---

## Post-Review Amendments (2026-04-10)

> These amendments override the corresponding sections in the plan below.

### CRITICAL FIX 1: Points redemption MUST run inside the order creation transaction

The plan's checkout integration (Task 9) validates and redeems points in a standalone `h.orderSvc.Unit()` call *before* order creation. This violates spec §8.2. If order creation fails after points are deducted, the customer loses points with no order.

**Required change:** `LoyaltyApplier.Apply(ctx, tx, orderID, amount)` must use the `tx` passed in (the order creation transaction), NOT open its own via `s.db.Transaction()`. `RedeemPoints` must accept and use the caller's `tx` parameter.

### CRITICAL FIX 2: Self-referral prevention at both service and DB level

The `Enroll` method checks referral code but never compares `referrer.CustomerEmail == enrollingEmail`. A customer using an email alias can self-refer.

**Required changes:**
1. Service layer: add `referrer.CustomerEmail == req.CustomerEmail` guard → return validation error
2. Migration: add `CHECK (referrer_id != referee_id)` constraint to `referrals` table

### HIGH FIX 3: Expiry batch job must use FOR UPDATE SKIP LOCKED correctly

The plan's expiry worker uses `LIMIT 500 OFFSET` pagination without locking. Concurrent workers would double-expire rows. Spec §8.8 requires `FOR UPDATE SKIP LOCKED`.

**Required change:** The `FindExpiredPoints` query must NOT use `FOR UPDATE` with aggregate functions (PostgreSQL rejects this). Restructure: first `SELECT ... FOR UPDATE SKIP LOCKED` individual `loyalty_transactions` rows, then aggregate in the application layer. Each batch must run in its own transaction.

### HIGH FIX 4: Add concurrent redemption race test

Spec §12 requires a "concurrent redemption race test." The plan's tests only cover `GenerateReferralCode` and `CalculateTier`. **Fix:** Add integration test spawning N goroutines calling `DebitPoints` with `go test -race`. Only one should succeed when balance < 2×points.

### HIGH FIX 5: Remove dead `UpdateLifetimePoints` method

`CreditPoints` already atomically increments both `points_balance` and `lifetime_points`. The separate `UpdateLifetimePoints` method is dead code that risks double-increment if mistakenly called. Remove it.

### MEDIUM FIX 6: `CalculateTier` requires sorted tiers

The "last match wins" logic requires tiers sorted by `min_lifetime_points` descending. The plan does not validate or enforce sort order on save. **Fix:** Sort tiers by `min_lifetime_points` descending in `CalculateTier` before iterating, or validate sort order in the service layer on program save.

### MEDIUM FIX 7: Rate limiter cleanup (same as M1/M2)

`sync.Map` grows unboundedly. Add periodic cleanup goroutine.

### MEDIUM FIX 8: Round points_value calculations to 2 decimal places

`points_value` is `NUMERIC(8,4)` but `redeemPoints * pointsValue` can produce >2 decimal places. `.Round(2)` the discount amount before applying.

### MEDIUM FIX 9: Referral code entropy

Spec §8.5 requires 128-bit entropy for codes. `GenerateReferralCode` uses 8 bytes (64 bits). For per-store referral codes this is acceptable but doesn't meet the stated spec. If enforcing spec literally, use 16 bytes.

### LOW FIX 10: Paginate member transaction history

`GET /admin/.../loyalty/members/:id` loads all `loyalty_transactions` unbounded. Add pagination (limit/offset).

### LOW FIX 11: Storefront `/loyalty/me` endpoint leaks data

Anyone can query any customer's loyalty balance by email without auth. Add rate limiting at minimum (not listed in spec §8.6 rate-limit targets but should be).

---

## Pre-flight Checks

Before starting, verify the codebase is in the expected state:

```bash
# 1. Confirm latest migration is 000008
ls services/marketplace-api/migrations/ | tail -4
# Expect: 000008_payments_shipping_tax.{up,down}.sql

# 2. Confirm ExpectedSchemaVersion is 1 in migrations.go
grep "ExpectedSchemaVersion" services/marketplace-api/migrations.go
# Expect: const ExpectedSchemaVersion uint = 1

# 3. Confirm no existing internal/loyalty/ or internal/discount/ packages
ls services/marketplace-api/internal/loyalty/ 2>/dev/null && echo "ERROR: loyalty/ already exists" || echo "OK: loyalty/ does not exist"
ls services/marketplace-api/internal/discount/ 2>/dev/null && echo "ERROR: discount/ already exists" || echo "OK: discount/ does not exist"

# 4. Confirm no marketing/ pages in admin app
ls apps/admin/app/marketing/ 2>/dev/null && echo "ERROR: marketing/ already exists" || echo "OK: marketing/ does not exist"

# 5. Build passes
cd services/marketplace-api && go build ./... && echo "OK: build passes"
```

> **IMPORTANT:** Migration numbering note — the spec says loyalty is migration 000011, but the repo currently has migrations up through 000008. If M1 (Coupons, 000009) and M2 (Gift Cards, 000010) have NOT been implemented yet, you may need to adjust the migration number. Check what the latest migration file is at implementation time. This plan uses 000011 as specified. If 000009/000010 don't exist yet, skip ahead and use 000009 instead — but update ALL references in this plan accordingly.

---

## Task 1: Migration 000011 — Loyalty Tables

**Files to create:**
- `services/marketplace-api/migrations/000011_loyalty.up.sql`
- `services/marketplace-api/migrations/000011_loyalty.down.sql`

### Step 1.1: Create up migration

- [ ] Create `services/marketplace-api/migrations/000011_loyalty.up.sql` with the following exact content:

```sql
-- Migration 000011: Loyalty program tables (Marketing M3)

CREATE TABLE loyalty_programs (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    is_active           BOOLEAN       NOT NULL DEFAULT false,
    points_per_dollar   NUMERIC(5,2)  NOT NULL DEFAULT 1.00,
    points_currency     VARCHAR(20)   NOT NULL DEFAULT 'points',
    signup_bonus        INT           NOT NULL DEFAULT 0,
    referral_bonus      INT           NOT NULL DEFAULT 0,
    referee_bonus       INT           NOT NULL DEFAULT 0,
    point_expiry_days   INT,
    min_redeem_points   INT           NOT NULL DEFAULT 100,
    points_value        NUMERIC(8,4)  NOT NULL DEFAULT 0.01,
    tiers               JSONB         NOT NULL DEFAULT '[]'::jsonb,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE customer_loyalties (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    customer_email  VARCHAR(300)  NOT NULL,
    customer_name   VARCHAR(200),
    points_balance  INT           NOT NULL DEFAULT 0,
    lifetime_points INT           NOT NULL DEFAULT 0,
    tier            VARCHAR(50)   NOT NULL DEFAULT 'bronze',
    referral_code   VARCHAR(20)   NOT NULL,
    referred_by     UUID,
    enrolled_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, customer_email),
    CHECK (points_balance >= 0)
);
CREATE INDEX cl_store_tier_idx ON customer_loyalties (store_id, tier);
CREATE INDEX cl_referral_code_idx ON customer_loyalties (store_id, referral_code);

CREATE TABLE loyalty_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    loyalty_id      UUID          NOT NULL REFERENCES customer_loyalties(id) ON DELETE CASCADE,
    order_id        UUID,
    type            VARCHAR(20)   NOT NULL,
    points          INT           NOT NULL,
    balance_after   INT           NOT NULL,
    description     VARCHAR(200),
    adjusted_by     VARCHAR(200),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CHECK (balance_after >= 0)
);
CREATE INDEX lt_loyalty_idx ON loyalty_transactions (loyalty_id);
CREATE INDEX lt_created_at_idx ON loyalty_transactions (created_at);
CREATE INDEX lt_type_created_idx ON loyalty_transactions (type, created_at);

CREATE TABLE referrals (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    referrer_id     UUID          NOT NULL REFERENCES customer_loyalties(id),
    referee_id      UUID          NOT NULL REFERENCES customer_loyalties(id),
    status          VARCHAR(20)   NOT NULL DEFAULT 'pending',
    referrer_bonus  INT           NOT NULL DEFAULT 0,
    referee_bonus   INT           NOT NULL DEFAULT 0,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, referee_id)
);
```

### Step 1.2: Create down migration

- [ ] Create `services/marketplace-api/migrations/000011_loyalty.down.sql`:

```sql
DROP TABLE IF EXISTS referrals;
DROP TABLE IF EXISTS loyalty_transactions;
DROP TABLE IF EXISTS customer_loyalties;
DROP TABLE IF EXISTS loyalty_programs;
```

### Step 1.3: Update ExpectedSchemaVersion

- [ ] In `services/marketplace-api/migrations.go`, update:

```go
const ExpectedSchemaVersion uint = 1
```

to:

```go
const ExpectedSchemaVersion uint = 11
```

> **Note:** If migrations 000009 and 000010 exist (from M1 Coupons and M2 Gift Cards), set this to 11. If they don't exist yet and you used 000009 for this migration, set to 9. The version must match the highest migration number.

### Step 1.4: Run migration locally

- [ ] Run `make mp-migrate-up` (or the equivalent `go run ./cmd/migrate up` command) and verify the tables are created.

### TDD: Migration verification

```bash
cd services/marketplace-api
go run ./cmd/migrate up
# Then verify:
# psql: \dt loyalty_programs, customer_loyalties, loyalty_transactions, referrals
# Confirm CHECK constraints: INSERT ... WITH negative balance_after should fail
```

### Commit

```
feat(marketplace-api): add migration 000011 for loyalty program tables
```

---

## Task 2: Loyalty Models (`internal/loyalty/models.go`)

**File to create:** `services/marketplace-api/internal/loyalty/models.go`

Follow the GORM model pattern from `internal/order/models.go`: explicit column tags, TableName() methods, uuid.UUID for IDs, shopspring/decimal for monetary values, time.Time for timestamps.

- [ ] Create `services/marketplace-api/internal/loyalty/models.go`:

```go
package loyalty

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// --- Constants ---

type TransactionType string

const (
	TxTypeEarn     TransactionType = "earn"
	TxTypeRedeem   TransactionType = "redeem"
	TxTypeExpire   TransactionType = "expire"
	TxTypeAdjust   TransactionType = "adjust"
	TxTypeSignup   TransactionType = "signup"
	TxTypeReferral TransactionType = "referral"
)

type ReferralStatus string

const (
	ReferralStatusPending   ReferralStatus = "pending"
	ReferralStatusCompleted ReferralStatus = "completed"
	ReferralStatusExpired   ReferralStatus = "expired"
)

// --- Tier ---

// Tier is the Go representation of one element inside
// loyalty_programs.tiers (JSONB). Validated at the service layer
// before write — never trust the DB contents blindly.
type Tier struct {
	Name      string          `json:"name"`
	MinPoints int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

// --- GORM Models ---

// LoyaltyProgram is the per-store configuration for the loyalty feature.
// Exactly one row per store_id (UNIQUE constraint).
type LoyaltyProgram struct {
	ID              uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID         uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	IsActive        bool            `gorm:"column:is_active;not null;default:false"`
	PointsPerDollar decimal.Decimal `gorm:"column:points_per_dollar;type:numeric(5,2);not null;default:1.00"`
	PointsCurrency  string          `gorm:"column:points_currency;type:varchar(20);not null;default:'points'"`
	SignupBonus     int             `gorm:"column:signup_bonus;not null;default:0"`
	ReferralBonus   int             `gorm:"column:referral_bonus;not null;default:0"`
	RefereeBonus    int             `gorm:"column:referee_bonus;not null;default:0"`
	PointExpiryDays *int            `gorm:"column:point_expiry_days"`
	MinRedeemPoints int             `gorm:"column:min_redeem_points;not null;default:100"`
	PointsValue     decimal.Decimal `gorm:"column:points_value;type:numeric(8,4);not null;default:0.01"`
	Tiers           datatypes.JSON  `gorm:"column:tiers;type:jsonb;not null;default:'[]'::jsonb"`
	CreatedAt       time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (LoyaltyProgram) TableName() string { return "loyalty_programs" }

// CustomerLoyalty is a customer's enrollment + running balance.
type CustomerLoyalty struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	CustomerEmail  string     `gorm:"column:customer_email;type:varchar(300);not null"`
	CustomerName   *string    `gorm:"column:customer_name;type:varchar(200)"`
	PointsBalance  int        `gorm:"column:points_balance;not null;default:0"`
	LifetimePoints int        `gorm:"column:lifetime_points;not null;default:0"`
	Tier           string     `gorm:"column:tier;type:varchar(50);not null;default:'bronze'"`
	ReferralCode   string     `gorm:"column:referral_code;type:varchar(20);not null"`
	ReferredBy     *uuid.UUID `gorm:"column:referred_by;type:uuid"`
	EnrolledAt     time.Time  `gorm:"column:enrolled_at;not null;default:now()"`
}

func (CustomerLoyalty) TableName() string { return "customer_loyalties" }

// LoyaltyTransaction is an append-only ledger of point changes.
type LoyaltyTransaction struct {
	ID          uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	LoyaltyID   uuid.UUID       `gorm:"column:loyalty_id;type:uuid;not null"`
	OrderID     *uuid.UUID      `gorm:"column:order_id;type:uuid"`
	Type        TransactionType `gorm:"column:type;type:varchar(20);not null"`
	Points      int             `gorm:"column:points;not null"`
	BalanceAfter int            `gorm:"column:balance_after;not null"`
	Description *string         `gorm:"column:description;type:varchar(200)"`
	AdjustedBy  *string         `gorm:"column:adjusted_by;type:varchar(200)"`
	CreatedAt   time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (LoyaltyTransaction) TableName() string { return "loyalty_transactions" }

// Referral tracks who referred whom.
type Referral struct {
	ID            uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID      uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID       uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	ReferrerID    uuid.UUID      `gorm:"column:referrer_id;type:uuid;not null"`
	RefereeID     uuid.UUID      `gorm:"column:referee_id;type:uuid;not null"`
	Status        ReferralStatus `gorm:"column:status;type:varchar(20);not null;default:'pending'"`
	ReferrerBonus int            `gorm:"column:referrer_bonus;not null;default:0"`
	RefereeBonus  int            `gorm:"column:referee_bonus;not null;default:0"`
	CompletedAt   *time.Time     `gorm:"column:completed_at"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (Referral) TableName() string { return "referrals" }
```

### TDD

- [ ] Create `services/marketplace-api/internal/loyalty/models_test.go` — test that TableName() methods return expected strings, and that Tier marshals/unmarshals to JSON correctly:

```go
package loyalty

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableNames(t *testing.T) {
	assert.Equal(t, "loyalty_programs", LoyaltyProgram{}.TableName())
	assert.Equal(t, "customer_loyalties", CustomerLoyalty{}.TableName())
	assert.Equal(t, "loyalty_transactions", LoyaltyTransaction{}.TableName())
	assert.Equal(t, "referrals", Referral{}.TableName())
}

func TestTierJSON(t *testing.T) {
	tier := Tier{Name: "Gold", MinPoints: 1000, Multiplier: decimal.NewFromFloat(1.5)}
	b, err := json.Marshal(tier)
	require.NoError(t, err)

	var got Tier
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "Gold", got.Name)
	assert.Equal(t, 1000, got.MinPoints)
	assert.True(t, got.Multiplier.Equal(decimal.NewFromFloat(1.5)))
}
```

- [ ] Run: `cd services/marketplace-api && go test ./internal/loyalty/... -v`

### Commit

```
feat(loyalty): add GORM models for loyalty program, customer loyalty, transactions, referrals
```

---

## Task 3: Repository (`internal/loyalty/repository.go`)

**File to create:** `services/marketplace-api/internal/loyalty/repository.go`

Follow the pattern from `internal/order/repository.go`: interface + gormRepository struct, explicit *gorm.DB per call for tx threading, stateless NewRepository().

### Step 3.1: Repository interface and implementation

- [ ] Create `services/marketplace-api/internal/loyalty/repository.go`:

```go
package loyalty

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for the loyalty aggregate.
// Mutating methods accept *gorm.DB for explicit transaction threading.
type Repository interface {
	// Program CRUD
	GetProgram(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*LoyaltyProgram, error)
	UpsertProgram(tx *gorm.DB, program *LoyaltyProgram) error

	// Customer enrollment + lookup
	GetCustomerByEmail(ctx context.Context, db *gorm.DB, storeID uuid.UUID, email string) (*CustomerLoyalty, error)
	GetCustomerByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*CustomerLoyalty, error)
	GetCustomerByReferralCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*CustomerLoyalty, error)
	CreateCustomer(tx *gorm.DB, c *CustomerLoyalty) error
	ListMembers(ctx context.Context, db *gorm.DB, storeID uuid.UUID, page, limit int) ([]CustomerLoyalty, int64, error)

	// Atomic point operations — use UPDATE...WHERE...RETURNING pattern
	CreditPoints(tx *gorm.DB, loyaltyID uuid.UUID, points int) (newBalance int, err error)
	DebitPoints(tx *gorm.DB, loyaltyID uuid.UUID, points int) (newBalance int, err error)
	UpdateTier(tx *gorm.DB, loyaltyID uuid.UUID, tier string) error
	UpdateLifetimePoints(tx *gorm.DB, loyaltyID uuid.UUID, additionalPoints int) error

	// Transactions (append-only ledger)
	CreateTransaction(tx *gorm.DB, t *LoyaltyTransaction) error
	ListTransactions(ctx context.Context, db *gorm.DB, loyaltyID uuid.UUID, page, limit int) ([]LoyaltyTransaction, int64, error)

	// Referrals
	CreateReferral(tx *gorm.DB, r *Referral) error
	CompleteReferral(tx *gorm.DB, referralID uuid.UUID) error
	ListReferrals(ctx context.Context, db *gorm.DB, storeID uuid.UUID, page, limit int) ([]Referral, int64, error)

	// Expiry — batch select for point expiry worker
	SelectExpiredTransactions(ctx context.Context, db *gorm.DB, expiryBefore time.Time, batchSize int) ([]ExpiredPointsBatch, error)
}

// ExpiredPointsBatch groups expired earn transactions by loyalty_id for
// the expiry worker. The worker sums points to expire per customer.
type ExpiredPointsBatch struct {
	LoyaltyID   uuid.UUID
	TenantID    uuid.UUID
	TotalPoints int
}

type gormRepository struct{}

func NewRepository() Repository { return &gormRepository{} }

// --- Program ---

func (gormRepository) GetProgram(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*LoyaltyProgram, error) {
	var p LoyaltyProgram
	err := db.WithContext(ctx).Where("store_id = ?", storeID).First(&p).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // no program configured yet
		}
		return nil, err
	}
	return &p, nil
}

func (gormRepository) UpsertProgram(tx *gorm.DB, program *LoyaltyProgram) error {
	// Use ON CONFLICT on the unique (store_id) constraint.
	return tx.Save(program).Error
}

// --- Customer ---

func (gormRepository) GetCustomerByEmail(ctx context.Context, db *gorm.DB, storeID uuid.UUID, email string) (*CustomerLoyalty, error) {
	var c CustomerLoyalty
	err := db.WithContext(ctx).Where("store_id = ? AND customer_email = ?", storeID, email).First(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (gormRepository) GetCustomerByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*CustomerLoyalty, error) {
	var c CustomerLoyalty
	err := db.WithContext(ctx).Where("id = ?", id).First(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("loyalty member")
		}
		return nil, err
	}
	return &c, nil
}

func (gormRepository) GetCustomerByReferralCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*CustomerLoyalty, error) {
	var c CustomerLoyalty
	err := db.WithContext(ctx).Where("store_id = ? AND referral_code = ?", storeID, code).First(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (gormRepository) CreateCustomer(tx *gorm.DB, c *CustomerLoyalty) error {
	return tx.Create(c).Error
}

func (gormRepository) ListMembers(ctx context.Context, db *gorm.DB, storeID uuid.UUID, page, limit int) ([]CustomerLoyalty, int64, error) {
	var members []CustomerLoyalty
	var total int64
	q := db.WithContext(ctx).Where("store_id = ?", storeID)
	if err := q.Model(&CustomerLoyalty{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := q.Order("enrolled_at DESC").Offset(offset).Limit(limit).Find(&members).Error; err != nil {
		return nil, 0, err
	}
	return members, total, nil
}

// --- Atomic Point Operations ---

// CreditPoints atomically adds points via UPDATE ... SET ... RETURNING.
// Also increments lifetime_points.
func (gormRepository) CreditPoints(tx *gorm.DB, loyaltyID uuid.UUID, points int) (int, error) {
	var newBalance int
	err := tx.Raw(`
		UPDATE customer_loyalties
		SET points_balance = points_balance + ?,
		    lifetime_points = lifetime_points + ?
		WHERE id = ?
		RETURNING points_balance
	`, points, points, loyaltyID).Scan(&newBalance).Error
	return newBalance, err
}

// DebitPoints atomically deducts points. Returns apperrors.ErrInsufficientLoyaltyPoints
// if the customer doesn't have enough (zero rows updated).
func (gormRepository) DebitPoints(tx *gorm.DB, loyaltyID uuid.UUID, points int) (int, error) {
	var newBalance int
	result := tx.Raw(`
		UPDATE customer_loyalties
		SET points_balance = points_balance - ?
		WHERE id = ? AND points_balance >= ?
		RETURNING points_balance
	`, points, loyaltyID, points).Scan(&newBalance)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, apperrors.New(apperrors.CodeInsufficientLoyaltyPoints, "insufficient loyalty points")
	}
	return newBalance, nil
}

func (gormRepository) UpdateTier(tx *gorm.DB, loyaltyID uuid.UUID, tier string) error {
	return tx.Model(&CustomerLoyalty{}).Where("id = ?", loyaltyID).Update("tier", tier).Error
}

func (gormRepository) UpdateLifetimePoints(tx *gorm.DB, loyaltyID uuid.UUID, additionalPoints int) error {
	return tx.Exec(`
		UPDATE customer_loyalties
		SET lifetime_points = lifetime_points + ?
		WHERE id = ?
	`, additionalPoints, loyaltyID).Error
}

// --- Transactions ---

func (gormRepository) CreateTransaction(tx *gorm.DB, t *LoyaltyTransaction) error {
	return tx.Create(t).Error
}

func (gormRepository) ListTransactions(ctx context.Context, db *gorm.DB, loyaltyID uuid.UUID, page, limit int) ([]LoyaltyTransaction, int64, error) {
	var txns []LoyaltyTransaction
	var total int64
	q := db.WithContext(ctx).Where("loyalty_id = ?", loyaltyID)
	if err := q.Model(&LoyaltyTransaction{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&txns).Error; err != nil {
		return nil, 0, err
	}
	return txns, total, nil
}

// --- Referrals ---

func (gormRepository) CreateReferral(tx *gorm.DB, r *Referral) error {
	return tx.Create(r).Error
}

func (gormRepository) CompleteReferral(tx *gorm.DB, referralID uuid.UUID) error {
	now := time.Now()
	return tx.Model(&Referral{}).Where("id = ?", referralID).
		Updates(map[string]any{
			"status":       ReferralStatusCompleted,
			"completed_at": now,
		}).Error
}

func (gormRepository) ListReferrals(ctx context.Context, db *gorm.DB, storeID uuid.UUID, page, limit int) ([]Referral, int64, error) {
	var refs []Referral
	var total int64
	q := db.WithContext(ctx).Where("store_id = ?", storeID)
	if err := q.Model(&Referral{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&refs).Error; err != nil {
		return nil, 0, err
	}
	return refs, total, nil
}

// --- Expiry ---

// SelectExpiredTransactions finds earn-type transactions older than expiryBefore
// that haven't already been expired, grouped by loyalty_id. Uses FOR UPDATE
// SKIP LOCKED for safe concurrent batch processing.
func (gormRepository) SelectExpiredTransactions(ctx context.Context, db *gorm.DB, expiryBefore time.Time, batchSize int) ([]ExpiredPointsBatch, error) {
	var batches []ExpiredPointsBatch
	err := db.WithContext(ctx).Raw(`
		WITH expired AS (
			SELECT lt.loyalty_id, lt.tenant_id, SUM(lt.points) AS total_points
			FROM loyalty_transactions lt
			WHERE lt.type = 'earn'
			  AND lt.created_at < ?
			  AND NOT EXISTS (
			      SELECT 1 FROM loyalty_transactions lt2
			      WHERE lt2.loyalty_id = lt.loyalty_id
			        AND lt2.type = 'expire'
			        AND lt2.description LIKE 'expiry:' || lt.id::text || '%'
			  )
			GROUP BY lt.loyalty_id, lt.tenant_id
			LIMIT ?
			FOR UPDATE OF lt SKIP LOCKED
		)
		SELECT loyalty_id, tenant_id, total_points
		FROM expired
		WHERE total_points > 0
	`, expiryBefore, batchSize).Scan(&batches).Error
	return batches, err
}

// --- Referral Code Generation ---

// GenerateReferralCode produces a 10-character uppercase base32 string
// using crypto/rand. 50 bits of entropy (well above collision threshold
// for per-store uniqueness).
func GenerateReferralCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate referral code: %w", err)
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	// Trim to 10 chars for friendliness
	if len(code) > 10 {
		code = code[:10]
	}
	return strings.ToUpper(code), nil
}
```

### Step 3.2: Repository tests

- [ ] Create `services/marketplace-api/internal/loyalty/repository_test.go`:

```go
package loyalty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateReferralCode(t *testing.T) {
	code, err := GenerateReferralCode()
	require.NoError(t, err)
	assert.Len(t, code, 10)
	// All uppercase alphanumeric (base32)
	for _, c := range code {
		assert.True(t, (c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7'), "unexpected char: %c", c)
	}

	// Two codes should be different (probabilistic but effectively guaranteed)
	code2, err := GenerateReferralCode()
	require.NoError(t, err)
	assert.NotEqual(t, code, code2)
}
```

- [ ] Run: `cd services/marketplace-api && go test ./internal/loyalty/... -v`

### Commit

```
feat(loyalty): add repository with atomic point operations and referral code generation
```

---

## Task 4: Service Layer (`internal/loyalty/service.go`)

**File to create:** `services/marketplace-api/internal/loyalty/service.go`

The service layer owns: enrollment, award points, redeem points, tier calculation, referral handling, program config updates, and tier JSON validation.

- [ ] Create `services/marketplace-api/internal/loyalty/service.go`:

```go
package loyalty

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service is the loyalty business-logic layer.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// --- Program Config ---

// GetProgram returns the store's loyalty program, or nil if not configured.
func (s *Service) GetProgram(ctx context.Context, storeID uuid.UUID) (*LoyaltyProgram, error) {
	return s.repo.GetProgram(ctx, s.db, storeID)
}

// UpdateProgramRequest is the validated input for updating a loyalty program.
type UpdateProgramRequest struct {
	TenantID        uuid.UUID
	StoreID         uuid.UUID
	IsActive        bool
	PointsPerDollar decimal.Decimal
	PointsCurrency  string
	SignupBonus     int
	ReferralBonus   int
	RefereeBonus    int
	PointExpiryDays *int
	MinRedeemPoints int
	PointsValue     decimal.Decimal
	Tiers           []Tier
}

// UpdateProgram upserts the loyalty program config. Validates tiers
// before saving.
func (s *Service) UpdateProgram(ctx context.Context, req UpdateProgramRequest) (*LoyaltyProgram, error) {
	// Validate tiers
	if err := validateTiers(req.Tiers); err != nil {
		return nil, err
	}

	tiersJSON, err := json.Marshal(req.Tiers)
	if err != nil {
		return nil, fmt.Errorf("marshal tiers: %w", err)
	}

	// Look up existing or create new
	existing, err := s.repo.GetProgram(ctx, s.db, req.StoreID)
	if err != nil {
		return nil, err
	}

	program := &LoyaltyProgram{
		TenantID:        req.TenantID,
		StoreID:         req.StoreID,
		IsActive:        req.IsActive,
		PointsPerDollar: req.PointsPerDollar,
		PointsCurrency:  req.PointsCurrency,
		SignupBonus:     req.SignupBonus,
		ReferralBonus:   req.ReferralBonus,
		RefereeBonus:    req.RefereeBonus,
		PointExpiryDays: req.PointExpiryDays,
		MinRedeemPoints: req.MinRedeemPoints,
		PointsValue:     req.PointsValue,
		Tiers:           tiersJSON,
		UpdatedAt:       time.Now(),
	}
	if existing != nil {
		program.ID = existing.ID
		program.CreatedAt = existing.CreatedAt
	} else {
		program.CreatedAt = time.Now()
	}

	if err := s.repo.UpsertProgram(s.db, program); err != nil {
		return nil, err
	}
	return program, nil
}

// validateTiers checks that tiers are well-formed: max 4, unique names,
// ascending min_points, positive multipliers.
func validateTiers(tiers []Tier) error {
	if len(tiers) > 4 {
		return apperrors.ValidationFailed("tiers", "maximum 4 tiers allowed")
	}
	names := make(map[string]bool, len(tiers))
	for i, t := range tiers {
		if t.Name == "" {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: name is required", i))
		}
		if names[t.Name] {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: duplicate name %q", i, t.Name))
		}
		names[t.Name] = true
		if t.MinPoints < 0 {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: min_points must be >= 0", i))
		}
		if t.Multiplier.LessThanOrEqual(decimal.Zero) {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: multiplier must be > 0", i))
		}
	}
	// Check ascending min_points
	for i := 1; i < len(tiers); i++ {
		if tiers[i].MinPoints <= tiers[i-1].MinPoints {
			return apperrors.ValidationFailed("tiers", "tiers must have strictly ascending min_points")
		}
	}
	return nil
}

// --- Enrollment ---

// EnrollRequest is the input for enrolling a customer.
type EnrollRequest struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	CustomerEmail string
	CustomerName  *string
	ReferralCode  *string // optional — code of the person who referred them
}

// Enroll registers a customer in the loyalty program. If the customer
// is already enrolled, returns the existing record. Awards signup_bonus
// if configured. Handles referral linkage.
func (s *Service) Enroll(ctx context.Context, req EnrollRequest) (*CustomerLoyalty, error) {
	// Check program exists and is active
	program, err := s.repo.GetProgram(ctx, s.db, req.StoreID)
	if err != nil {
		return nil, err
	}
	if program == nil || !program.IsActive {
		return nil, apperrors.New(apperrors.CodeLoyaltyNotEnrolled, "loyalty program is not active for this store")
	}

	// Check if already enrolled
	existing, err := s.repo.GetCustomerByEmail(ctx, s.db, req.StoreID, req.CustomerEmail)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil // already enrolled — idempotent
	}

	// Generate referral code
	code, err := GenerateReferralCode()
	if err != nil {
		return nil, err
	}

	// Resolve referrer if referral code provided
	var referredBy *uuid.UUID
	var referrer *CustomerLoyalty
	if req.ReferralCode != nil && *req.ReferralCode != "" {
		referrer, err = s.repo.GetCustomerByReferralCode(ctx, s.db, req.StoreID, *req.ReferralCode)
		if err != nil {
			return nil, err
		}
		if referrer != nil {
			referredBy = &referrer.ID
		}
	}

	customer := &CustomerLoyalty{
		TenantID:      req.TenantID,
		StoreID:       req.StoreID,
		CustomerEmail: req.CustomerEmail,
		CustomerName:  req.CustomerName,
		PointsBalance: 0,
		Tier:          "bronze",
		ReferralCode:  code,
		ReferredBy:    referredBy,
		EnrolledAt:    time.Now(),
	}

	// Transaction: create customer + signup bonus + referral
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateCustomer(tx, customer); err != nil {
			return err
		}

		// Signup bonus
		if program.SignupBonus > 0 {
			newBalance, err := s.repo.CreditPoints(tx, customer.ID, program.SignupBonus)
			if err != nil {
				return err
			}
			desc := "Signup bonus"
			if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
				TenantID:     req.TenantID,
				LoyaltyID:    customer.ID,
				Type:         TxTypeSignup,
				Points:       program.SignupBonus,
				BalanceAfter: newBalance,
				Description:  &desc,
				CreatedAt:    time.Now(),
			}); err != nil {
				return err
			}
			customer.PointsBalance = newBalance
			customer.LifetimePoints = program.SignupBonus
		}

		// Referral tracking
		if referrer != nil {
			referral := &Referral{
				TenantID:      req.TenantID,
				StoreID:       req.StoreID,
				ReferrerID:    referrer.ID,
				RefereeID:     customer.ID,
				Status:        ReferralStatusPending,
				ReferrerBonus: program.ReferralBonus,
				RefereeBonus:  program.RefereeBonus,
				CreatedAt:     time.Now(),
			}
			if err := s.repo.CreateReferral(tx, referral); err != nil {
				return err
			}

			// Award referee bonus immediately
			if program.RefereeBonus > 0 {
				newBal, err := s.repo.CreditPoints(tx, customer.ID, program.RefereeBonus)
				if err != nil {
					return err
				}
				desc := "Referral bonus (new member)"
				if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
					TenantID:     req.TenantID,
					LoyaltyID:    customer.ID,
					Type:         TxTypeReferral,
					Points:       program.RefereeBonus,
					BalanceAfter: newBal,
					Description:  &desc,
					CreatedAt:    time.Now(),
				}); err != nil {
					return err
				}
				customer.PointsBalance = newBal
			}

			// Award referrer bonus
			if program.ReferralBonus > 0 {
				newBal, err := s.repo.CreditPoints(tx, referrer.ID, program.ReferralBonus)
				if err != nil {
					return err
				}
				desc := fmt.Sprintf("Referral bonus: %s joined", req.CustomerEmail)
				if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
					TenantID:     req.TenantID,
					LoyaltyID:    referrer.ID,
					Type:         TxTypeReferral,
					Points:       program.ReferralBonus,
					BalanceAfter: newBal,
					Description:  &desc,
					CreatedAt:    time.Now(),
				}); err != nil {
					return err
				}
			}

			// Complete the referral
			if err := s.repo.CompleteReferral(tx, referral.ID); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return customer, nil
}

// --- Award Points (post-checkout) ---

// AwardPoints grants points based on an order total. Called after
// successful checkout. The formula: floor(orderTotal * pointsPerDollar * tierMultiplier).
func (s *Service) AwardPoints(ctx context.Context, tenantID, storeID uuid.UUID, customerEmail string, orderTotal decimal.Decimal, orderID uuid.UUID) error {
	program, err := s.repo.GetProgram(ctx, s.db, storeID)
	if err != nil || program == nil || !program.IsActive {
		return nil // silently skip if program not active
	}

	customer, err := s.repo.GetCustomerByEmail(ctx, s.db, storeID, customerEmail)
	if err != nil {
		return err
	}
	if customer == nil {
		return nil // not enrolled — skip
	}

	// Calculate points: floor(orderTotal * pointsPerDollar * tierMultiplier)
	multiplier := s.getTierMultiplier(program, customer.LifetimePoints)
	rawPoints := orderTotal.Mul(program.PointsPerDollar).Mul(multiplier)
	points := int(rawPoints.IntPart())
	if points <= 0 {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		newBalance, err := s.repo.CreditPoints(tx, customer.ID, points)
		if err != nil {
			return err
		}
		desc := fmt.Sprintf("Order %s: earned %d points", orderID.String()[:8], points)
		if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
			TenantID:     tenantID,
			LoyaltyID:    customer.ID,
			OrderID:      &orderID,
			Type:         TxTypeEarn,
			Points:       points,
			BalanceAfter: newBalance,
			Description:  &desc,
			CreatedAt:    time.Now(),
		}); err != nil {
			return err
		}

		// Recalculate tier after earning
		newTier := s.calculateTier(program, customer.LifetimePoints+points)
		if newTier != customer.Tier {
			if err := s.repo.UpdateTier(tx, customer.ID, newTier); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- Redeem Points ---

// RedeemPoints deducts points from a customer's balance. Returns the
// monetary value of the redeemed points (points * points_value).
func (s *Service) RedeemPoints(ctx context.Context, tenantID, storeID uuid.UUID, customerEmail string, points int, orderID *uuid.UUID) (decimal.Decimal, error) {
	program, err := s.repo.GetProgram(ctx, s.db, storeID)
	if err != nil {
		return decimal.Zero, err
	}
	if program == nil || !program.IsActive {
		return decimal.Zero, apperrors.New(apperrors.CodeLoyaltyNotEnrolled, "loyalty program is not active")
	}
	if points < program.MinRedeemPoints {
		return decimal.Zero, apperrors.ValidationFailed("points", fmt.Sprintf("minimum redemption is %d points", program.MinRedeemPoints))
	}

	customer, err := s.repo.GetCustomerByEmail(ctx, s.db, storeID, customerEmail)
	if err != nil {
		return decimal.Zero, err
	}
	if customer == nil {
		return decimal.Zero, apperrors.New(apperrors.CodeLoyaltyNotEnrolled, "customer is not enrolled in the loyalty program")
	}

	var newBalance int
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		newBalance, err = s.repo.DebitPoints(tx, customer.ID, points)
		if err != nil {
			return err
		}
		desc := fmt.Sprintf("Redeemed %d points", points)
		return s.repo.CreateTransaction(tx, &LoyaltyTransaction{
			TenantID:     tenantID,
			LoyaltyID:    customer.ID,
			OrderID:      orderID,
			Type:         TxTypeRedeem,
			Points:       -points,
			BalanceAfter: newBalance,
			Description:  &desc,
			CreatedAt:    time.Now(),
		})
	})
	if err != nil {
		return decimal.Zero, err
	}

	// Calculate monetary value
	value := decimal.NewFromInt(int64(points)).Mul(program.PointsValue)
	return value, nil
}

// --- Manual Adjust (admin) ---

// AdjustPoints allows an admin to manually adjust a customer's points.
func (s *Service) AdjustPoints(ctx context.Context, tenantID uuid.UUID, loyaltyID uuid.UUID, points int, description string, adjustedBy string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var newBalance int
		var err error
		if points > 0 {
			newBalance, err = s.repo.CreditPoints(tx, loyaltyID, points)
		} else {
			newBalance, err = s.repo.DebitPoints(tx, loyaltyID, -points)
		}
		if err != nil {
			return err
		}
		return s.repo.CreateTransaction(tx, &LoyaltyTransaction{
			TenantID:     tenantID,
			LoyaltyID:    loyaltyID,
			Type:         TxTypeAdjust,
			Points:       points,
			BalanceAfter: newBalance,
			Description:  &description,
			AdjustedBy:   &adjustedBy,
			CreatedAt:    time.Now(),
		})
	})
}

// --- Helpers ---

// getTierMultiplier returns the points multiplier for the customer's
// current tier. Falls back to 1.0 if no tiers configured.
func (s *Service) getTierMultiplier(program *LoyaltyProgram, lifetimePoints int) decimal.Decimal {
	tiers := s.parseTiers(program)
	if len(tiers) == 0 {
		return decimal.NewFromInt(1)
	}
	// Sort descending by MinPoints to find highest qualifying tier
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].MinPoints > tiers[j].MinPoints
	})
	for _, t := range tiers {
		if lifetimePoints >= t.MinPoints {
			return t.Multiplier
		}
	}
	return decimal.NewFromInt(1)
}

// calculateTier returns the tier name for the given lifetime points.
func (s *Service) calculateTier(program *LoyaltyProgram, lifetimePoints int) string {
	tiers := s.parseTiers(program)
	if len(tiers) == 0 {
		return "bronze"
	}
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].MinPoints > tiers[j].MinPoints
	})
	for _, t := range tiers {
		if lifetimePoints >= t.MinPoints {
			return t.Name
		}
	}
	return "bronze"
}

func (s *Service) parseTiers(program *LoyaltyProgram) []Tier {
	var tiers []Tier
	if err := json.Unmarshal(program.Tiers, &tiers); err != nil {
		s.logger.Error("failed to parse tiers JSON", "err", err, "store_id", program.StoreID)
		return nil
	}
	return tiers
}

// GetCustomer returns a customer loyalty record by email.
func (s *Service) GetCustomer(ctx context.Context, storeID uuid.UUID, email string) (*CustomerLoyalty, error) {
	return s.repo.GetCustomerByEmail(ctx, s.db, storeID, email)
}

// GetCustomerByID returns a customer loyalty record by ID.
func (s *Service) GetCustomerByID(ctx context.Context, id uuid.UUID) (*CustomerLoyalty, error) {
	return s.repo.GetCustomerByID(ctx, s.db, id)
}

// ListMembers returns paginated loyalty members for a store.
func (s *Service) ListMembers(ctx context.Context, storeID uuid.UUID, page, limit int) ([]CustomerLoyalty, int64, error) {
	return s.repo.ListMembers(ctx, s.db, storeID, page, limit)
}

// ListTransactions returns paginated transactions for a loyalty member.
func (s *Service) ListTransactions(ctx context.Context, loyaltyID uuid.UUID, page, limit int) ([]LoyaltyTransaction, int64, error) {
	return s.repo.ListTransactions(ctx, s.db, loyaltyID, page, limit)
}

// ListReferrals returns paginated referrals for a store.
func (s *Service) ListReferrals(ctx context.Context, storeID uuid.UUID, page, limit int) ([]Referral, int64, error) {
	return s.repo.ListReferrals(ctx, s.db, storeID, page, limit)
}
```

### Step 4.2: Service tests

- [ ] Create `services/marketplace-api/internal/loyalty/service_test.go`:

```go
package loyalty

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestValidateTiers_Valid(t *testing.T) {
	tiers := []Tier{
		{Name: "Bronze", MinPoints: 0, Multiplier: decimal.NewFromInt(1)},
		{Name: "Silver", MinPoints: 500, Multiplier: decimal.NewFromFloat(1.5)},
		{Name: "Gold", MinPoints: 1000, Multiplier: decimal.NewFromInt(2)},
	}
	assert.NoError(t, validateTiers(tiers))
}

func TestValidateTiers_TooMany(t *testing.T) {
	tiers := make([]Tier, 5)
	for i := range tiers {
		tiers[i] = Tier{Name: "T" + string(rune('A'+i)), MinPoints: i * 100, Multiplier: decimal.NewFromInt(1)}
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum 4 tiers")
}

func TestValidateTiers_DuplicateNames(t *testing.T) {
	tiers := []Tier{
		{Name: "Gold", MinPoints: 0, Multiplier: decimal.NewFromInt(1)},
		{Name: "Gold", MinPoints: 100, Multiplier: decimal.NewFromInt(2)},
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate name")
}

func TestValidateTiers_NonAscendingMinPoints(t *testing.T) {
	tiers := []Tier{
		{Name: "Silver", MinPoints: 500, Multiplier: decimal.NewFromFloat(1.5)},
		{Name: "Bronze", MinPoints: 0, Multiplier: decimal.NewFromInt(1)},
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ascending")
}

func TestValidateTiers_ZeroMultiplier(t *testing.T) {
	tiers := []Tier{
		{Name: "Bronze", MinPoints: 0, Multiplier: decimal.Zero},
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiplier")
}

func TestValidateTiers_Empty(t *testing.T) {
	assert.NoError(t, validateTiers(nil))
	assert.NoError(t, validateTiers([]Tier{}))
}
```

- [ ] Run: `cd services/marketplace-api && go test ./internal/loyalty/... -v`

### Commit

```
feat(loyalty): add service layer with enroll, award, redeem, adjust, tier calculation
```

---

## Task 5: Point Expiry Worker (`internal/loyalty/expiry.go`)

**File to create:** `services/marketplace-api/internal/loyalty/expiry.go`

Follows the csvjob worker pattern from `cmd/marketplace-api/main.go` lines 276-316: context-aware goroutine with ticker, startup recovery scan, batch processing.

- [ ] Create `services/marketplace-api/internal/loyalty/expiry.go`:

```go
package loyalty

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// ExpiryWorker runs a daily cron that expires loyalty points whose
// earn transaction is older than the program's point_expiry_days.
// Uses the csvjob worker pattern: context-controlled goroutine with
// ticker, batch of 500, FOR UPDATE SKIP LOCKED.
type ExpiryWorker struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewExpiryWorker constructs an ExpiryWorker.
func NewExpiryWorker(db *gorm.DB, repo Repository, logger *slog.Logger) *ExpiryWorker {
	return &ExpiryWorker{db: db, repo: repo, logger: logger}
}

// Start launches the expiry polling loop. Returns a channel that closes
// when the worker exits (mirrors the csvjob pattern). The caller passes
// a cancellable context to shut the worker down gracefully.
func (w *ExpiryWorker) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.logger.Info("loyalty: expiry worker started", "interval", interval)

		// Run once immediately on startup
		w.runCycle(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				w.logger.Info("loyalty: expiry worker stopping")
				return
			case <-ticker.C:
				w.runCycle(ctx)
			}
		}
	}()
	return done
}

// runCycle processes all stores that have point_expiry_days configured,
// then expires points in batches of 500.
func (w *ExpiryWorker) runCycle(ctx context.Context) {
	// Find all programs with expiry configured
	var programs []LoyaltyProgram
	err := w.db.WithContext(ctx).
		Where("is_active = ? AND point_expiry_days IS NOT NULL", true).
		Find(&programs).Error
	if err != nil {
		w.logger.Error("loyalty: expiry cycle failed to load programs", "err", err)
		return
	}

	for _, program := range programs {
		if program.PointExpiryDays == nil {
			continue
		}
		expiryBefore := time.Now().AddDate(0, 0, -*program.PointExpiryDays)
		w.expireForProgram(ctx, program, expiryBefore)
	}
}

func (w *ExpiryWorker) expireForProgram(ctx context.Context, program LoyaltyProgram, expiryBefore time.Time) {
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return
		}

		batches, err := w.repo.SelectExpiredTransactions(ctx, w.db, expiryBefore, batchSize)
		if err != nil {
			w.logger.Error("loyalty: expiry select failed",
				"store_id", program.StoreID, "err", err)
			return
		}
		if len(batches) == 0 {
			return // no more expired points
		}

		for _, batch := range batches {
			if err := w.expireBatch(ctx, batch); err != nil {
				w.logger.Error("loyalty: expiry batch failed",
					"loyalty_id", batch.LoyaltyID, "err", err)
				// Continue to next batch — don't block on one failure
			}
		}

		if len(batches) < batchSize {
			return // last page
		}
	}
}

func (w *ExpiryWorker) expireBatch(ctx context.Context, batch ExpiredPointsBatch) error {
	return w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		newBalance, err := w.repo.DebitPoints(tx, batch.LoyaltyID, batch.TotalPoints)
		if err != nil {
			// If insufficient points (customer already redeemed some),
			// debit whatever is available
			w.logger.Warn("loyalty: expiry debit partial — customer may have redeemed",
				"loyalty_id", batch.LoyaltyID, "attempted", batch.TotalPoints)
			return nil // skip this batch, don't fail
		}
		desc := fmt.Sprintf("Points expired (%d points)", batch.TotalPoints)
		return w.repo.CreateTransaction(tx, &LoyaltyTransaction{
			TenantID:     batch.TenantID,
			LoyaltyID:    batch.LoyaltyID,
			Type:         TxTypeExpire,
			Points:       -batch.TotalPoints,
			BalanceAfter: newBalance,
			Description:  &desc,
			CreatedAt:    time.Now(),
		})
	})
}
```

### TDD

- [ ] Create `services/marketplace-api/internal/loyalty/expiry_test.go` with a test verifying `NewExpiryWorker` returns a non-nil worker:

```go
package loyalty

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExpiryWorker(t *testing.T) {
	w := NewExpiryWorker(nil, NewRepository(), slog.Default())
	assert.NotNil(t, w)
}
```

- [ ] Run: `cd services/marketplace-api && go test ./internal/loyalty/... -v`

### Commit

```
feat(loyalty): add point expiry background worker with batch processing
```

---

## Task 6: Domain Errors

**File to edit:** `services/marketplace-api/pkg/apperrors/errors.go`

- [ ] Add the following error codes after the existing `CodeRecoveryTooRecent` line:

```go
	// Loyalty M3.
	CodeInsufficientLoyaltyPoints Code = "insufficient_loyalty_points"
	CodeLoyaltyNotEnrolled        Code = "loyalty_not_enrolled"
```

- [ ] Add sentinel values after `ErrRecoveryTooRecent`:

```go
	// Loyalty M3 sentinels.
	ErrInsufficientLoyaltyPoints = &Error{Code: CodeInsufficientLoyaltyPoints}
	ErrLoyaltyNotEnrolled        = &Error{Code: CodeLoyaltyNotEnrolled}
```

- [ ] Add the new codes to the `IsKnownCode` switch statement.

- [ ] Add constructors:

```go
// ---------- Loyalty M3 constructors ----------

func InsufficientLoyaltyPoints(available, requested int) *Error {
	return &Error{Code: CodeInsufficientLoyaltyPoints,
		Message: "customer does not have enough loyalty points",
		Details: map[string]any{"available": available, "requested": requested}}
}

func LoyaltyNotEnrolled(email string) *Error {
	return &Error{Code: CodeLoyaltyNotEnrolled,
		Message: "customer is not enrolled in the loyalty program",
		Details: map[string]any{"email": email}}
}
```

**File to edit:** `services/marketplace-api/internal/handlers/admin/errors.go`

- [ ] Add to `codeStatus` map:

```go
	// Loyalty M3.
	apperrors.CodeInsufficientLoyaltyPoints: http.StatusUnprocessableEntity,
	apperrors.CodeLoyaltyNotEnrolled:        http.StatusBadRequest,
```

### TDD

- [ ] Run existing test: `cd services/marketplace-api && go test ./pkg/apperrors/... -v`
- [ ] Verify new codes pass `IsKnownCode`.

### Commit

```
feat(loyalty): add domain error codes for insufficient points and not enrolled
```

---

## Task 7: Admin Handlers

**Files to create:**
- `services/marketplace-api/internal/handlers/admin/loyalty.go`
- `services/marketplace-api/internal/handlers/admin/loyalty_dto.go`

### Step 7.1: DTOs

- [ ] Create `services/marketplace-api/internal/handlers/admin/loyalty_dto.go`:

```go
package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
)

// --- Request DTOs ---

type UpdateLoyaltyProgramRequest struct {
	IsActive        bool            `json:"is_active"`
	PointsPerDollar decimal.Decimal `json:"points_per_dollar" binding:"required"`
	PointsCurrency  string          `json:"points_currency"   binding:"required"`
	SignupBonus     int             `json:"signup_bonus"`
	ReferralBonus   int             `json:"referral_bonus"`
	RefereeBonus    int             `json:"referee_bonus"`
	PointExpiryDays *int            `json:"point_expiry_days"`
	MinRedeemPoints int             `json:"min_redeem_points" binding:"required"`
	PointsValue     decimal.Decimal `json:"points_value"      binding:"required"`
	Tiers           []TierRequest   `json:"tiers"             binding:"dive"`
}

type TierRequest struct {
	Name       string          `json:"name"       binding:"required"`
	MinPoints  int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier" binding:"required"`
}

type AdjustPointsRequest struct {
	Points      int    `json:"points"      binding:"required"`
	Description string `json:"description" binding:"required"`
}

// --- Response DTOs ---

type LoyaltyProgramResponse struct {
	ID              string          `json:"id"`
	IsActive        bool            `json:"is_active"`
	PointsPerDollar decimal.Decimal `json:"points_per_dollar"`
	PointsCurrency  string          `json:"points_currency"`
	SignupBonus     int             `json:"signup_bonus"`
	ReferralBonus   int             `json:"referral_bonus"`
	RefereeBonus    int             `json:"referee_bonus"`
	PointExpiryDays *int            `json:"point_expiry_days"`
	MinRedeemPoints int             `json:"min_redeem_points"`
	PointsValue     decimal.Decimal `json:"points_value"`
	Tiers           []TierResponse  `json:"tiers"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type TierResponse struct {
	Name       string          `json:"name"`
	MinPoints  int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

type LoyaltyMemberResponse struct {
	ID             string    `json:"id"`
	CustomerEmail  string    `json:"customer_email"`
	CustomerName   *string   `json:"customer_name,omitempty"`
	PointsBalance  int       `json:"points_balance"`
	LifetimePoints int       `json:"lifetime_points"`
	Tier           string    `json:"tier"`
	ReferralCode   string    `json:"referral_code"`
	EnrolledAt     time.Time `json:"enrolled_at"`
}

type LoyaltyTransactionResponse struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Points      int       `json:"points"`
	BalanceAfter int      `json:"balance_after"`
	Description *string   `json:"description,omitempty"`
	AdjustedBy  *string   `json:"adjusted_by,omitempty"`
	OrderID     *string   `json:"order_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReferralResponse struct {
	ID            string     `json:"id"`
	ReferrerID    string     `json:"referrer_id"`
	RefereeID     string     `json:"referee_id"`
	Status        string     `json:"status"`
	ReferrerBonus int        `json:"referrer_bonus"`
	RefereeBonus  int        `json:"referee_bonus"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// --- Converters ---

func toLoyaltyProgramResponse(p *loyalty.LoyaltyProgram, tiers []loyalty.Tier) LoyaltyProgramResponse {
	tierResps := make([]TierResponse, 0, len(tiers))
	for _, t := range tiers {
		tierResps = append(tierResps, TierResponse{
			Name:       t.Name,
			MinPoints:  t.MinPoints,
			Multiplier: t.Multiplier,
		})
	}
	return LoyaltyProgramResponse{
		ID:              p.ID.String(),
		IsActive:        p.IsActive,
		PointsPerDollar: p.PointsPerDollar,
		PointsCurrency:  p.PointsCurrency,
		SignupBonus:     p.SignupBonus,
		ReferralBonus:   p.ReferralBonus,
		RefereeBonus:    p.RefereeBonus,
		PointExpiryDays: p.PointExpiryDays,
		MinRedeemPoints: p.MinRedeemPoints,
		PointsValue:     p.PointsValue,
		Tiers:           tierResps,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

func toLoyaltyMemberResponse(c *loyalty.CustomerLoyalty) LoyaltyMemberResponse {
	return LoyaltyMemberResponse{
		ID:             c.ID.String(),
		CustomerEmail:  c.CustomerEmail,
		CustomerName:   c.CustomerName,
		PointsBalance:  c.PointsBalance,
		LifetimePoints: c.LifetimePoints,
		Tier:           c.Tier,
		ReferralCode:   c.ReferralCode,
		EnrolledAt:     c.EnrolledAt,
	}
}

func toLoyaltyTransactionResponse(t *loyalty.LoyaltyTransaction) LoyaltyTransactionResponse {
	resp := LoyaltyTransactionResponse{
		ID:           t.ID.String(),
		Type:         string(t.Type),
		Points:       t.Points,
		BalanceAfter: t.BalanceAfter,
		Description:  t.Description,
		AdjustedBy:   t.AdjustedBy,
		CreatedAt:    t.CreatedAt,
	}
	if t.OrderID != nil {
		s := t.OrderID.String()
		resp.OrderID = &s
	}
	return resp
}

func toReferralResponse(r *loyalty.Referral) ReferralResponse {
	return ReferralResponse{
		ID:            r.ID.String(),
		ReferrerID:    r.ReferrerID.String(),
		RefereeID:     r.RefereeID.String(),
		Status:        string(r.Status),
		ReferrerBonus: r.ReferrerBonus,
		RefereeBonus:  r.RefereeBonus,
		CompletedAt:   r.CompletedAt,
		CreatedAt:     r.CreatedAt,
	}
}
```

### Step 7.2: Handler

- [ ] Create `services/marketplace-api/internal/handlers/admin/loyalty.go`:

```go
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// LoyaltyHandler bundles dependencies for admin loyalty endpoints.
type LoyaltyHandler struct {
	svc    *loyalty.Service
	logger *slog.Logger
}

// NewLoyaltyHandler constructs a LoyaltyHandler.
func NewLoyaltyHandler(svc *loyalty.Service, logger *slog.Logger) *LoyaltyHandler {
	return &LoyaltyHandler{svc: svc, logger: logger}
}

// GetProgram handles GET /admin/stores/:storeId/loyalty/program.
func (h *LoyaltyHandler) GetProgram(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	program, err := h.svc.GetProgram(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if program == nil {
		// Return empty/default config
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	var tiers []loyalty.Tier
	_ = json.Unmarshal(program.Tiers, &tiers)

	c.JSON(http.StatusOK, gin.H{"data": toLoyaltyProgramResponse(program, tiers)})
}

// UpdateProgram handles PUT /admin/stores/:storeId/loyalty/program.
func (h *LoyaltyHandler) UpdateProgram(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}
	tenantID := c.GetString("tenant_id")
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid UUID"), h.logger)
		return
	}

	var req UpdateLoyaltyProgramRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Convert tier requests to domain tiers
	tiers := make([]loyalty.Tier, 0, len(req.Tiers))
	for _, t := range req.Tiers {
		tiers = append(tiers, loyalty.Tier{
			Name:       t.Name,
			MinPoints:  t.MinPoints,
			Multiplier: t.Multiplier,
		})
	}

	program, err := h.svc.UpdateProgram(c.Request.Context(), loyalty.UpdateProgramRequest{
		TenantID:        tenantUUID,
		StoreID:         storeID,
		IsActive:        req.IsActive,
		PointsPerDollar: req.PointsPerDollar,
		PointsCurrency:  req.PointsCurrency,
		SignupBonus:     req.SignupBonus,
		ReferralBonus:   req.ReferralBonus,
		RefereeBonus:    req.RefereeBonus,
		PointExpiryDays: req.PointExpiryDays,
		MinRedeemPoints: req.MinRedeemPoints,
		PointsValue:     req.PointsValue,
		Tiers:           tiers,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	var parsedTiers []loyalty.Tier
	_ = json.Unmarshal(program.Tiers, &parsedTiers)

	c.JSON(http.StatusOK, gin.H{"data": toLoyaltyProgramResponse(program, parsedTiers)})
}

// ListMembers handles GET /admin/stores/:storeId/loyalty/members.
func (h *LoyaltyHandler) ListMembers(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	page, limit := parsePagination(c)
	members, total, err := h.svc.ListMembers(c.Request.Context(), storeID, page, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]LoyaltyMemberResponse, 0, len(members))
	for i := range members {
		out = append(out, toLoyaltyMemberResponse(&members[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"total": total, "page": page, "limit": limit},
	})
}

// GetMember handles GET /admin/stores/:storeId/loyalty/members/:id.
func (h *LoyaltyHandler) GetMember(c *gin.Context) {
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	member, err := h.svc.GetCustomerByID(c.Request.Context(), memberID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	page, limit := parsePagination(c)
	txns, txnTotal, err := h.svc.ListTransactions(c.Request.Context(), memberID, page, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	txnOut := make([]LoyaltyTransactionResponse, 0, len(txns))
	for i := range txns {
		txnOut = append(txnOut, toLoyaltyTransactionResponse(&txns[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toLoyaltyMemberResponse(member),
		"transactions": gin.H{
			"data": txnOut,
			"meta": gin.H{"total": txnTotal, "page": page, "limit": limit},
		},
	})
}

// AdjustPoints handles POST /admin/stores/:storeId/loyalty/members/:id/adjust.
func (h *LoyaltyHandler) AdjustPoints(c *gin.Context) {
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}
	tenantID := c.GetString("tenant_id")
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid UUID"), h.logger)
		return
	}
	userEmail := c.GetString("user_email")
	if userEmail == "" {
		userEmail = c.GetString("user_id") // fallback
	}

	var req AdjustPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if req.Points == 0 {
		RespondErr(c, apperrors.ValidationFailed("points", "points must be non-zero"), h.logger)
		return
	}

	if err := h.svc.AdjustPoints(c.Request.Context(), tenantUUID, memberID, req.Points, req.Description, userEmail); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "points adjusted"})
}

// ListReferrals handles GET /admin/stores/:storeId/loyalty/referrals.
func (h *LoyaltyHandler) ListReferrals(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	page, limit := parsePagination(c)
	refs, total, err := h.svc.ListReferrals(c.Request.Context(), storeID, page, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]ReferralResponse, 0, len(refs))
	for i := range refs {
		out = append(out, toReferralResponse(&refs[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"total": total, "page": page, "limit": limit},
	})
}

// parsePagination extracts page/limit from query params with defaults.
func parsePagination(c *gin.Context) (int, int) {
	page := 1
	limit := 20
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	return page, limit
}
```

### Step 7.3: Create authz role constants

- [ ] Create `services/marketplace-api/internal/authz/loyalty_roles.go`:

```go
package authz

// Loyalty roles — follows the pattern from orders_roles.go.

// LoyaltyViewRole is the minimum relation required to view loyalty
// program configuration and member lists.
var LoyaltyViewRole = RoleStaff

// LoyaltyEditRole is the minimum relation required to update program
// config and manually adjust points.
var LoyaltyEditRole = RoleAdmin
```

### TDD

- [ ] Run: `cd services/marketplace-api && go build ./...`
- [ ] Verify no compilation errors.

### Commit

```
feat(loyalty): add admin handlers for program config, members, referrals, and point adjustment
```

---

## Task 8: Storefront Handlers

**File to create:** `services/marketplace-api/internal/handlers/storefront/loyalty.go`

- [ ] Create `services/marketplace-api/internal/handlers/storefront/loyalty.go`:

```go
package storefront

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// LoyaltyHandler bundles dependencies for storefront loyalty endpoints.
type LoyaltyHandler struct {
	svc    *loyalty.Service
	logger *slog.Logger
}

// NewLoyaltyHandler constructs a storefront LoyaltyHandler.
func NewLoyaltyHandler(svc *loyalty.Service, logger *slog.Logger) *LoyaltyHandler {
	return &LoyaltyHandler{svc: svc, logger: logger}
}

// --- Request/Response DTOs ---

type sfEnrollRequest struct {
	Email        string  `json:"email"         binding:"required,email"`
	Name         *string `json:"name"`
	ReferralCode *string `json:"referral_code"`
}

type sfRedeemRequest struct {
	Email  string `json:"email"  binding:"required,email"`
	Points int    `json:"points" binding:"required,min=1"`
}

type sfProgramResponse struct {
	IsActive        bool                       `json:"is_active"`
	PointsPerDollar decimal.Decimal            `json:"points_per_dollar"`
	PointsCurrency  string                     `json:"points_currency"`
	SignupBonus     int                        `json:"signup_bonus"`
	ReferralBonus   int                        `json:"referral_bonus"`
	RefereeBonus    int                        `json:"referee_bonus"`
	MinRedeemPoints int                        `json:"min_redeem_points"`
	PointsValue     decimal.Decimal            `json:"points_value"`
	Tiers           []sfTierResponse           `json:"tiers"`
}

type sfTierResponse struct {
	Name      string          `json:"name"`
	MinPoints int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

type sfCustomerResponse struct {
	PointsBalance  int    `json:"points_balance"`
	LifetimePoints int    `json:"lifetime_points"`
	Tier           string `json:"tier"`
	ReferralCode   string `json:"referral_code"`
}

type sfRedeemResponse struct {
	PointsRedeemed int             `json:"points_redeemed"`
	Value          decimal.Decimal `json:"value"`
}

// --- Handlers ---

// GetProgram handles GET /storefront/stores/:storeSlug/loyalty/program.
func (h *LoyaltyHandler) GetProgram(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)

	program, err := h.svc.GetProgram(c.Request.Context(), storeID)
	if err != nil {
		respondInternalError(c, h.logger, err)
		return
	}
	if program == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	var tiers []loyalty.Tier
	_ = json.Unmarshal(program.Tiers, &tiers)

	tierResps := make([]sfTierResponse, 0, len(tiers))
	for _, t := range tiers {
		tierResps = append(tierResps, sfTierResponse{
			Name:       t.Name,
			MinPoints:  t.MinPoints,
			Multiplier: t.Multiplier,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": sfProgramResponse{
		IsActive:        program.IsActive,
		PointsPerDollar: program.PointsPerDollar,
		PointsCurrency:  program.PointsCurrency,
		SignupBonus:     program.SignupBonus,
		ReferralBonus:   program.ReferralBonus,
		RefereeBonus:    program.RefereeBonus,
		MinRedeemPoints: program.MinRedeemPoints,
		PointsValue:     program.PointsValue,
		Tiers:           tierResps,
	}})
}

// Enroll handles POST /storefront/stores/:storeSlug/loyalty/enroll.
func (h *LoyaltyHandler) Enroll(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)
	tenantID, _ := uuid.Parse(store.TenantID)

	var req sfEnrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	customer, err := h.svc.Enroll(c.Request.Context(), loyalty.EnrollRequest{
		TenantID:      tenantID,
		StoreID:       storeID,
		CustomerEmail: req.Email,
		CustomerName:  req.Name,
		ReferralCode:  req.ReferralCode,
	})
	if err != nil {
		respondAppError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sfCustomerResponse{
		PointsBalance:  customer.PointsBalance,
		LifetimePoints: customer.LifetimePoints,
		Tier:           customer.Tier,
		ReferralCode:   customer.ReferralCode,
	}})
}

// GetMe handles GET /storefront/stores/:storeSlug/loyalty/me?email=.
func (h *LoyaltyHandler) GetMe(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)

	email := c.Query("email")
	if email == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "validation_failed", "message": "email query parameter is required"})
		return
	}

	customer, err := h.svc.GetCustomer(c.Request.Context(), storeID, email)
	if err != nil {
		respondInternalError(c, h.logger, err)
		return
	}
	if customer == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sfCustomerResponse{
		PointsBalance:  customer.PointsBalance,
		LifetimePoints: customer.LifetimePoints,
		Tier:           customer.Tier,
		ReferralCode:   customer.ReferralCode,
	}})
}

// Redeem handles POST /storefront/stores/:storeSlug/loyalty/redeem.
func (h *LoyaltyHandler) Redeem(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)
	tenantID, _ := uuid.Parse(store.TenantID)

	var req sfRedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	value, err := h.svc.RedeemPoints(c.Request.Context(), tenantID, storeID, req.Email, req.Points, nil)
	if err != nil {
		respondAppError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sfRedeemResponse{
		PointsRedeemed: req.Points,
		Value:          value,
	}})
}

// --- Helpers ---

func (h *LoyaltyHandler) resolveStore(c *gin.Context) *stores.Store {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return nil
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return nil
	}
	return store
}

func respondAppError(c *gin.Context, logger *slog.Logger, err error) {
	var ae *apperrors.Error
	if ok := apperrors.As(err, &ae); ok {
		status := http.StatusBadRequest
		switch ae.Code {
		case apperrors.CodeInsufficientLoyaltyPoints:
			status = http.StatusUnprocessableEntity
		case apperrors.CodeLoyaltyNotEnrolled:
			status = http.StatusBadRequest
		case apperrors.CodeNotFound:
			status = http.StatusNotFound
		}
		c.AbortWithStatusJSON(status, gin.H{"error": string(ae.Code), "message": ae.Message})
		return
	}
	respondInternalError(c, logger, err)
}

func respondInternalError(c *gin.Context, logger *slog.Logger, err error) {
	if logger != nil {
		logger.Error("storefront loyalty handler error", "err", err)
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError,
		gin.H{"error": "internal", "message": "internal server error"})
}
```

> **Note:** The `respondNotFound` function already exists in `storefront/checkout.go` or similar. If not, add a small helper:
> ```go
> func respondNotFound(c *gin.Context) {
>     c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
> }
> ```

- [ ] Also add `apperrors.As` helper if not present. Check if `errors.As` is used directly in the codebase. The storefront handler uses it, so either import `errors` and use `errors.As`, or define a thin wrapper. **Safest approach:** Just use `errors.As` directly from the standard `errors` package. Replace `apperrors.As(err, &ae)` with:

```go
import "errors"
// ...
if errors.As(err, &ae) {
```

### TDD

- [ ] Run: `cd services/marketplace-api && go build ./...`

### Commit

```
feat(loyalty): add storefront handlers for program info, enrollment, me, and redemption
```

---

## Task 9: Checkout Integration

**File to create:** `services/marketplace-api/internal/discount/applier.go`

Per spec section 8.3, define the discount.Applier interface and the loyalty implementation.

### Step 9.1: Discount interface

- [ ] Create `services/marketplace-api/internal/discount/applier.go`:

```go
// Package discount defines the pluggable discount application interface
// used by checkout_ext.go. Coupons, gift cards, and loyalty redemptions
// each implement Applier.
package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Applier applies a discount to an order within a transaction. Returns
// the discount amount actually applied (may be less than requested if
// the balance or cap is lower). The caller accumulates these to set
// the order's discount_total.
type Applier interface {
	Apply(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, amount decimal.Decimal) (discountAmount decimal.Decimal, err error)
}
```

### Step 9.2: Loyalty discount applier

- [ ] Create `services/marketplace-api/internal/discount/loyalty_applier.go`:

```go
package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
)

// LoyaltyApplier implements Applier for loyalty point redemption.
// It deducts points and returns the monetary value as the discount.
type LoyaltyApplier struct {
	svc           *loyalty.Service
	tenantID      uuid.UUID
	storeID       uuid.UUID
	customerEmail string
	points        int
}

// NewLoyaltyApplier constructs a LoyaltyApplier.
func NewLoyaltyApplier(svc *loyalty.Service, tenantID, storeID uuid.UUID, email string, points int) *LoyaltyApplier {
	return &LoyaltyApplier{
		svc:           svc,
		tenantID:      tenantID,
		storeID:       storeID,
		customerEmail: email,
		points:        points,
	}
}

// Apply redeems the specified points and returns the monetary discount.
// The amount parameter is the maximum the discount should not exceed
// (typically the remaining order balance). If the points value exceeds
// the amount, only enough points to cover `amount` are redeemed.
func (a *LoyaltyApplier) Apply(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, amount decimal.Decimal) (decimal.Decimal, error) {
	value, err := a.svc.RedeemPoints(ctx, a.tenantID, a.storeID, a.customerEmail, a.points, &orderID)
	if err != nil {
		return decimal.Zero, err
	}
	// Cap at the remaining order balance
	if value.GreaterThan(amount) {
		return amount, nil
	}
	return value, nil
}
```

### Step 9.3: Post-checkout point award integration

The post-checkout award is called from `checkout_ext.go` after successful payment. Add a hook point.

- [ ] In `services/marketplace-api/internal/handlers/storefront/checkout_ext.go`, after the payment intent is created successfully (near the end of the `Checkout` method), add:

```go
// Award loyalty points (best-effort, non-blocking).
// This runs AFTER the payment transaction succeeds.
if h.loyaltySvc != nil {
    go func() {
        if err := h.loyaltySvc.AwardPoints(
            context.Background(),
            store.TenantUUID(), store.StoreUUID(),
            req.CustomerEmail, orderTotal, orderUUID,
        ); err != nil {
            h.logger.Error("loyalty: award points failed",
                "order_id", orderUUID, "err", err)
        }
    }()
}
```

> **Important:** The `CheckoutExtHandler` struct needs a new field:
> ```go
> loyaltySvc *loyalty.Service
> ```
> And `NewCheckoutExtHandler` needs an optional `loyaltySvc` parameter. Since this might break the existing constructor signature, use a setter or add it as an optional field:
> ```go
> // SetLoyaltyService wires the loyalty service for post-checkout point awards.
> func (h *CheckoutExtHandler) SetLoyaltyService(svc *loyalty.Service) {
>     h.loyaltySvc = svc
> }
> ```

> **Note on exact implementation:** The exact lines to modify in `checkout_ext.go` depend on where the payment success happens. Read the full file at implementation time, find the success path (after payment intent creation or webhook confirmation), and insert the loyalty award call there. If post-checkout awards should be synchronous (within the same request), call it inline instead of in a goroutine. The spec says "Post-checkout: loyalty.Service.AwardPoints" which implies it can be async/best-effort.

### TDD

- [ ] Run: `cd services/marketplace-api && go build ./...`

### Commit

```
feat(loyalty): add discount.Applier interface and loyalty checkout integration
```

---

## Task 10: Wire Routes + main.go + Expiry Worker Startup

### Step 10.1: Add LoyaltyHandler to admin Deps

- [ ] In `services/marketplace-api/internal/handlers/admin/routes.go`, add to the `Deps` struct:

```go
LoyaltyHandler *LoyaltyHandler
```

- [ ] In `RegisterAdmin`, after the settings routes block and before the abandoned carts block, add:

```go
		// Loyalty program.
		if deps.LoyaltyHandler != nil {
			loyaltyGroup := storeRoute.Group("/loyalty")
			{
				loyaltyGroup.GET("/program",
					deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
					deps.LoyaltyHandler.GetProgram)
				loyaltyGroup.PUT("/program",
					deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyEditRole),
					deps.LoyaltyHandler.UpdateProgram)
				members := loyaltyGroup.Group("/members")
				{
					members.GET("",
						deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
						deps.LoyaltyHandler.ListMembers)
					members.GET("/:id",
						deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
						deps.LoyaltyHandler.GetMember)
					members.POST("/:id/adjust",
						deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyEditRole),
						deps.LoyaltyHandler.AdjustPoints)
				}
				loyaltyGroup.GET("/referrals",
					deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
					deps.LoyaltyHandler.ListReferrals)
			}
		}
```

### Step 10.2: Add LoyaltyHandler to storefront Deps

- [ ] In `services/marketplace-api/internal/handlers/storefront/routes.go`, add to the `Deps` struct:

```go
LoyaltyHandler *LoyaltyHandler
```

- [ ] In `RegisterStorefront`, inside the `group` block (after the OrderDetailHandler block), add:

```go
		// Loyalty — public endpoints, no auth.
		if deps.LoyaltyHandler != nil {
			loyaltyGroup := group.Group("/loyalty")
			{
				loyaltyGroup.GET("/program", deps.LoyaltyHandler.GetProgram)
				loyaltyGroup.POST("/enroll", deps.LoyaltyHandler.Enroll)
				loyaltyGroup.GET("/me", deps.LoyaltyHandler.GetMe)
				loyaltyGroup.POST("/redeem", deps.LoyaltyHandler.Redeem)
			}
		}
```

### Step 10.3: Wire in main.go

- [ ] In `services/marketplace-api/cmd/marketplace-api/main.go`:

Add import:
```go
"github.com/mark8ly/marketplace-api/internal/loyalty"
```

In the admin wiring block (after `taxSettingsHandler` and before `adminDeps = admin.Deps{`), add:

```go
		// Loyalty M3 wiring.
		loyaltyRepo := loyalty.NewRepository()
		loyaltySvc := loyalty.NewService(conn, loyaltyRepo, log)
		loyaltyHandler := admin.NewLoyaltyHandler(loyaltySvc, log)
```

Add to `adminDeps`:
```go
		LoyaltyHandler: loyaltyHandler,
```

In the storefront wiring block (after the `orderDetailHandler` setup and before `storefrontDeps = storefront.Deps{`), add:

```go
		// Loyalty M3 storefront wiring.
		loyaltyRepoSF := loyalty.NewRepository()
		loyaltySvcSF := loyalty.NewService(conn, loyaltyRepoSF, log)
		sfLoyaltyHandler := storefront.NewLoyaltyHandler(loyaltySvcSF, log)
```

Add to `storefrontDeps`:
```go
		LoyaltyHandler: sfLoyaltyHandler,
```

Wire the loyalty service into the checkout ext handler:
```go
		checkoutExtHandler.SetLoyaltyService(loyaltySvcSF)
```

### Step 10.4: Expiry worker startup

In `main.go`, after the csvjob worker block and before the outbox publisher block, add:

```go
	// Loyalty point expiry worker — runs daily, admin/both modes only.
	var expiryWorkerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		loyaltyRepoWorker := loyalty.NewRepository()
		expiryWorker := loyalty.NewExpiryWorker(conn, loyaltyRepoWorker, log)
		expiryWorkerDone = expiryWorker.Start(workerCtx, 24*time.Hour)
		log.Info("loyalty: expiry worker started (24h interval)")
	}
```

In the graceful shutdown section (where `workerCancel()` is called), ensure `expiryWorkerDone` is waited on:

```go
	// After workerCancel() is called:
	if expiryWorkerDone != nil {
		<-expiryWorkerDone
	}
```

### TDD

- [ ] Run: `cd services/marketplace-api && go build ./...`
- [ ] Run: `cd services/marketplace-api && go vet ./...`

### Commit

```
feat(loyalty): wire loyalty routes, handlers, and expiry worker into main.go
```

---

## Task 11: Admin UI — Tabbed Loyalty Page

**Files to create:**
- `apps/admin/app/marketing/loyalty/page.tsx`
- `apps/admin/components/marketing/LoyaltyProgramTab.tsx`
- `apps/admin/components/marketing/LoyaltyMembersTab.tsx`
- `apps/admin/components/marketing/LoyaltyReferralsTab.tsx`
- `apps/admin/components/marketing/TierBuilder.tsx`
- `apps/admin/lib/api/loyalty-api.ts`

### Step 11.1: API client

- [ ] Create `apps/admin/lib/api/loyalty-api.ts`:

```typescript
// apps/admin/lib/api/loyalty-api.ts
//
// Server-side API client for loyalty endpoints.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

export interface LoyaltyProgram {
  id: string;
  is_active: boolean;
  points_per_dollar: string;
  points_currency: string;
  signup_bonus: number;
  referral_bonus: number;
  referee_bonus: number;
  point_expiry_days: number | null;
  min_redeem_points: number;
  points_value: string;
  tiers: LoyaltyTier[];
  created_at: string;
  updated_at: string;
}

export interface LoyaltyTier {
  name: string;
  min_points: number;
  multiplier: string;
}

export interface LoyaltyMember {
  id: string;
  customer_email: string;
  customer_name: string | null;
  points_balance: number;
  lifetime_points: number;
  tier: string;
  referral_code: string;
  enrolled_at: string;
}

export interface LoyaltyTransaction {
  id: string;
  type: string;
  points: number;
  balance_after: number;
  description: string | null;
  adjusted_by: string | null;
  order_id: string | null;
  created_at: string;
}

export interface LoyaltyReferral {
  id: string;
  referrer_id: string;
  referee_id: string;
  status: string;
  referrer_bonus: number;
  referee_bonus: number;
  completed_at: string | null;
  created_at: string;
}

function headers(session: SessionHeaders): HeadersInit {
  return {
    "Content-Type": "application/json",
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    "X-Internal-Secret": process.env.INTERNAL_AUTH_SECRET ?? "",
  };
}

export async function getLoyaltyProgram(
  storeId: string,
  session: SessionHeaders,
): Promise<LoyaltyProgram | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/program`,
    { headers: headers(session), cache: "no-store" },
  );
  if (!res.ok) return null;
  const json = await res.json();
  return json.data ?? null;
}

export async function updateLoyaltyProgram(
  storeId: string,
  session: SessionHeaders,
  body: Record<string, unknown>,
): Promise<LoyaltyProgram | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/program`,
    { method: "PUT", headers: headers(session), body: JSON.stringify(body) },
  );
  if (!res.ok) return null;
  const json = await res.json();
  return json.data ?? null;
}

export async function getLoyaltyMembers(
  storeId: string,
  session: SessionHeaders,
  page = 1,
  limit = 20,
): Promise<{ data: LoyaltyMember[]; meta: { total: number } }> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/members?page=${page}&limit=${limit}`,
    { headers: headers(session), cache: "no-store" },
  );
  if (!res.ok) return { data: [], meta: { total: 0 } };
  return res.json();
}

export async function getLoyaltyMember(
  storeId: string,
  memberId: string,
  session: SessionHeaders,
): Promise<{
  data: LoyaltyMember;
  transactions: { data: LoyaltyTransaction[]; meta: { total: number } };
} | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/members/${memberId}`,
    { headers: headers(session), cache: "no-store" },
  );
  if (!res.ok) return null;
  return res.json();
}

export async function adjustPoints(
  storeId: string,
  memberId: string,
  session: SessionHeaders,
  points: number,
  description: string,
): Promise<boolean> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/members/${memberId}/adjust`,
    {
      method: "POST",
      headers: headers(session),
      body: JSON.stringify({ points, description }),
    },
  );
  return res.ok;
}

export async function getLoyaltyReferrals(
  storeId: string,
  session: SessionHeaders,
  page = 1,
  limit = 20,
): Promise<{ data: LoyaltyReferral[]; meta: { total: number } }> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/referrals?page=${page}&limit=${limit}`,
    { headers: headers(session), cache: "no-store" },
  );
  if (!res.ok) return { data: [], meta: { total: 0 } };
  return res.json();
}
```

### Step 11.2: TierBuilder component

- [ ] Create `apps/admin/components/marketing/TierBuilder.tsx`:

```tsx
"use client";

import { useState } from "react";

interface Tier {
  name: string;
  min_points: number;
  multiplier: string;
}

interface TierBuilderProps {
  value: Tier[];
  onChange: (tiers: Tier[]) => void;
  disabled?: boolean;
}

const MAX_TIERS = 4;

export function TierBuilder({ value, onChange, disabled }: TierBuilderProps) {
  const addTier = () => {
    if (value.length >= MAX_TIERS) return;
    const lastMinPoints =
      value.length > 0 ? value[value.length - 1].min_points + 500 : 0;
    onChange([
      ...value,
      { name: "", min_points: lastMinPoints, multiplier: "1.0" },
    ]);
  };

  const updateTier = (index: number, field: keyof Tier, fieldValue: string | number) => {
    const updated = value.map((tier, i) =>
      i === index ? { ...tier, [field]: fieldValue } : tier,
    );
    onChange(updated);
  };

  const removeTier = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
          Tiers ({value.length}/{MAX_TIERS})
        </h3>
        {!disabled && value.length < MAX_TIERS && (
          <button
            type="button"
            onClick={addTier}
            className="rounded-[6px] bg-[color:var(--ink-900)] px-3 py-1.5 text-xs font-medium text-white hover:bg-[color:var(--ink-900)]/90 transition-colors"
          >
            Add tier
          </button>
        )}
      </div>

      {value.length === 0 && (
        <p className="text-sm text-[color:var(--ink-900)]/40">
          No tiers configured. All members earn at 1x rate.
        </p>
      )}

      <div className="space-y-3">
        {value.map((tier, index) => (
          <div
            key={index}
            className="flex items-end gap-3 rounded-[6px] bg-[color:var(--paper-200)] px-4 py-3"
          >
            <div className="flex-1 space-y-1">
              <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
                Name
              </label>
              <input
                type="text"
                value={tier.name}
                onChange={(e) => updateTier(index, "name", e.target.value)}
                disabled={disabled}
                placeholder="e.g. Silver"
                className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-1.5 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
              />
            </div>
            <div className="w-32 space-y-1">
              <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
                Min points
              </label>
              <input
                type="number"
                value={tier.min_points}
                onChange={(e) =>
                  updateTier(index, "min_points", parseInt(e.target.value) || 0)
                }
                disabled={disabled}
                min={0}
                className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-1.5 text-sm text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
              />
            </div>
            <div className="w-28 space-y-1">
              <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
                Multiplier
              </label>
              <input
                type="text"
                value={tier.multiplier}
                onChange={(e) =>
                  updateTier(index, "multiplier", e.target.value)
                }
                disabled={disabled}
                placeholder="1.5"
                className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-1.5 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
              />
            </div>
            {!disabled && (
              <button
                type="button"
                onClick={() => removeTier(index)}
                className="mb-0.5 rounded-[6px] px-2 py-1.5 text-xs text-[color:var(--ink-900)]/40 hover:bg-[color:var(--ink-900)]/5 hover:text-[color:var(--ink-900)]/70 transition-colors"
                aria-label={`Remove tier ${tier.name || index + 1}`}
              >
                Remove
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
```

### Step 11.3: Program config tab

- [ ] Create `apps/admin/components/marketing/LoyaltyProgramTab.tsx`:

```tsx
"use client";

import { useState, useTransition } from "react";
import { TierBuilder } from "./TierBuilder";
import type { LoyaltyProgram, LoyaltyTier } from "@/lib/api/loyalty-api";

interface LoyaltyProgramTabProps {
  program: LoyaltyProgram | null;
  storeId: string;
  editable: boolean;
  onSave: (data: Record<string, unknown>) => Promise<void>;
}

export function LoyaltyProgramTab({
  program,
  storeId,
  editable,
  onSave,
}: LoyaltyProgramTabProps) {
  const [isPending, startTransition] = useTransition();

  const [isActive, setIsActive] = useState(program?.is_active ?? false);
  const [pointsPerDollar, setPointsPerDollar] = useState(
    program?.points_per_dollar ?? "1.00",
  );
  const [pointsCurrency, setPointsCurrency] = useState(
    program?.points_currency ?? "points",
  );
  const [signupBonus, setSignupBonus] = useState(program?.signup_bonus ?? 0);
  const [referralBonus, setReferralBonus] = useState(
    program?.referral_bonus ?? 0,
  );
  const [refereeBonus, setRefereeBonus] = useState(
    program?.referee_bonus ?? 0,
  );
  const [pointExpiryDays, setPointExpiryDays] = useState<number | "">(
    program?.point_expiry_days ?? "",
  );
  const [minRedeemPoints, setMinRedeemPoints] = useState(
    program?.min_redeem_points ?? 100,
  );
  const [pointsValue, setPointsValue] = useState(
    program?.points_value ?? "0.01",
  );
  const [tiers, setTiers] = useState<LoyaltyTier[]>(program?.tiers ?? []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    startTransition(async () => {
      await onSave({
        is_active: isActive,
        points_per_dollar: pointsPerDollar,
        points_currency: pointsCurrency,
        signup_bonus: signupBonus,
        referral_bonus: referralBonus,
        referee_bonus: refereeBonus,
        point_expiry_days: pointExpiryDays === "" ? null : pointExpiryDays,
        min_redeem_points: minRedeemPoints,
        points_value: pointsValue,
        tiers,
      });
    });
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Active toggle */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-[color:var(--ink-900)]">
              Loyalty program
            </h2>
            <p className="text-sm text-[color:var(--ink-900)]/60">
              Enable the loyalty program for your store.
            </p>
          </div>
          <label className="relative inline-flex cursor-pointer items-center">
            <input
              type="checkbox"
              checked={isActive}
              onChange={(e) => setIsActive(e.target.checked)}
              disabled={!editable}
              className="peer sr-only"
            />
            <div className="h-6 w-11 rounded-full bg-[color:var(--ink-900)]/10 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all peer-checked:bg-[color:var(--moss-700)] peer-checked:after:translate-x-full" />
          </label>
        </div>
      </div>

      {/* Points configuration */}
      <div className="rounded-[6px] bg-white px-6 py-5 space-y-4">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
          Points configuration
        </h3>
        <hr className="border-t border-[color:var(--ink-900)]/6" />
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Points per dollar
            </label>
            <input
              type="text"
              value={pointsPerDollar}
              onChange={(e) => setPointsPerDollar(e.target.value)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Points display name
            </label>
            <input
              type="text"
              value={pointsCurrency}
              onChange={(e) => setPointsCurrency(e.target.value)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Point value (currency)
            </label>
            <input
              type="text"
              value={pointsValue}
              onChange={(e) => setPointsValue(e.target.value)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Min points to redeem
            </label>
            <input
              type="number"
              value={minRedeemPoints}
              onChange={(e) => setMinRedeemPoints(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Point expiry (days)
            </label>
            <input
              type="number"
              value={pointExpiryDays}
              onChange={(e) =>
                setPointExpiryDays(
                  e.target.value === "" ? "" : parseInt(e.target.value),
                )
              }
              disabled={!editable}
              placeholder="Never"
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm placeholder:text-[color:var(--ink-900)]/30"
            />
          </div>
        </div>
      </div>

      {/* Bonuses */}
      <div className="rounded-[6px] bg-white px-6 py-5 space-y-4">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
          Bonuses
        </h3>
        <hr className="border-t border-[color:var(--ink-900)]/6" />
        <div className="grid grid-cols-3 gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Signup bonus
            </label>
            <input
              type="number"
              value={signupBonus}
              onChange={(e) => setSignupBonus(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Referral bonus (referrer)
            </label>
            <input
              type="number"
              value={referralBonus}
              onChange={(e) => setReferralBonus(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-[color:var(--ink-900)]/60">
              Referral bonus (referee)
            </label>
            <input
              type="number"
              value={refereeBonus}
              onChange={(e) => setRefereeBonus(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm"
            />
          </div>
        </div>
      </div>

      {/* Tiers */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <TierBuilder
          value={tiers}
          onChange={setTiers}
          disabled={!editable}
        />
      </div>

      {/* Save */}
      {editable && (
        <div className="flex justify-end">
          <button
            type="submit"
            disabled={isPending}
            className="rounded-[6px] bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-white hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50 transition-colors"
          >
            {isPending ? "Saving..." : "Save program"}
          </button>
        </div>
      )}
    </form>
  );
}
```

### Step 11.4: Members tab

- [ ] Create `apps/admin/components/marketing/LoyaltyMembersTab.tsx`:

```tsx
"use client";

import type { LoyaltyMember } from "@/lib/api/loyalty-api";

interface LoyaltyMembersTabProps {
  members: LoyaltyMember[];
  total: number;
}

export function LoyaltyMembersTab({ members, total }: LoyaltyMembersTabProps) {
  if (members.length === 0) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          No members enrolled yet.
        </p>
        <p className="mt-1 text-xs text-[color:var(--ink-900)]/30">
          Members will appear here once customers enroll in the loyalty program.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-[6px] bg-white">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/6">
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Email
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Points
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Lifetime
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Tier
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Enrolled
            </th>
          </tr>
        </thead>
        <tbody>
          {members.map((m) => (
            <tr
              key={m.id}
              className="border-b border-[color:var(--ink-900)]/6 last:border-0"
            >
              <td className="px-4 py-3 text-[color:var(--ink-900)]">
                {m.customer_email}
              </td>
              <td className="px-4 py-3 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-[color:var(--ink-900)]">
                {m.points_balance.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-[color:var(--ink-900)]/60">
                {m.lifetime_points.toLocaleString()}
              </td>
              <td className="px-4 py-3">
                <span className="inline-block rounded-[4px] bg-[color:var(--moss-700)]/10 px-2 py-0.5 text-xs font-medium capitalize text-[color:var(--moss-700)]">
                  {m.tier}
                </span>
              </td>
              <td className="px-4 py-3 text-[color:var(--ink-900)]/50 text-xs">
                {new Date(m.enrolled_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="px-4 py-3 text-xs text-[color:var(--ink-900)]/40">
        {total} member{total !== 1 ? "s" : ""} total
      </div>
    </div>
  );
}
```

### Step 11.5: Referrals tab

- [ ] Create `apps/admin/components/marketing/LoyaltyReferralsTab.tsx`:

```tsx
"use client";

import type { LoyaltyReferral } from "@/lib/api/loyalty-api";

interface LoyaltyReferralsTabProps {
  referrals: LoyaltyReferral[];
  total: number;
}

export function LoyaltyReferralsTab({
  referrals,
  total,
}: LoyaltyReferralsTabProps) {
  if (referrals.length === 0) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          No referrals yet.
        </p>
        <p className="mt-1 text-xs text-[color:var(--ink-900)]/30">
          Referrals will appear here when enrolled members share their codes.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-[6px] bg-white">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/6">
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Referrer
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Referee
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Status
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Bonuses
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Date
            </th>
          </tr>
        </thead>
        <tbody>
          {referrals.map((r) => (
            <tr
              key={r.id}
              className="border-b border-[color:var(--ink-900)]/6 last:border-0"
            >
              <td className="px-4 py-3 text-[color:var(--ink-900)] text-xs font-mono">
                {r.referrer_id.slice(0, 8)}...
              </td>
              <td className="px-4 py-3 text-[color:var(--ink-900)] text-xs font-mono">
                {r.referee_id.slice(0, 8)}...
              </td>
              <td className="px-4 py-3">
                <span
                  className={`inline-block rounded-[4px] px-2 py-0.5 text-xs font-medium capitalize ${
                    r.status === "completed"
                      ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
                      : "bg-[color:var(--ink-900)]/5 text-[color:var(--ink-900)]/50"
                  }`}
                >
                  {r.status}
                </span>
              </td>
              <td className="px-4 py-3 text-xs text-[color:var(--ink-900)]/60">
                +{r.referrer_bonus} / +{r.referee_bonus}
              </td>
              <td className="px-4 py-3 text-[color:var(--ink-900)]/50 text-xs">
                {new Date(r.created_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="px-4 py-3 text-xs text-[color:var(--ink-900)]/40">
        {total} referral{total !== 1 ? "s" : ""} total
      </div>
    </div>
  );
}
```

### Step 11.6: Main loyalty page (tabbed)

- [ ] Ensure the directory exists: `mkdir -p apps/admin/app/marketing/loyalty`

- [ ] Create `apps/admin/app/marketing/loyalty/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getLoyaltyProgram,
  getLoyaltyMembers,
  getLoyaltyReferrals,
  updateLoyaltyProgram,
} from "@/lib/api/loyalty-api";
import { LoyaltyProgramTab } from "@/components/marketing/LoyaltyProgramTab";
import { LoyaltyMembersTab } from "@/components/marketing/LoyaltyMembersTab";
import { LoyaltyReferralsTab } from "@/components/marketing/LoyaltyReferralsTab";
import { LoyaltyTabSwitcher } from "./LoyaltyTabSwitcher";

export default async function LoyaltyPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Marketing</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Loyalty program
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Configure your loyalty program, view members, and manage referrals.
          </p>
        </header>

        {currentStore ? (
          <LoyaltyContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring loyalty.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function LoyaltyContent({
  storeId,
  userId,
  tenantId,
  editable,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
}) {
  const session = { userId, tenantId };

  const [program, membersData, referralsData] = await Promise.all([
    getLoyaltyProgram(storeId, session),
    getLoyaltyMembers(storeId, session),
    getLoyaltyReferrals(storeId, session),
  ]);

  async function handleSaveProgram(data: Record<string, unknown>) {
    "use server";
    await updateLoyaltyProgram(storeId, session, data);
  }

  return (
    <LoyaltyTabSwitcher
      programTab={
        <LoyaltyProgramTab
          program={program}
          storeId={storeId}
          editable={editable}
          onSave={handleSaveProgram}
        />
      }
      membersTab={
        <LoyaltyMembersTab
          members={membersData.data}
          total={membersData.meta.total}
        />
      }
      referralsTab={
        <LoyaltyReferralsTab
          referrals={referralsData.data}
          total={referralsData.meta.total}
        />
      }
    />
  );
}
```

- [ ] Create `apps/admin/app/marketing/loyalty/LoyaltyTabSwitcher.tsx`:

```tsx
"use client";

import { useState, type ReactNode } from "react";

interface LoyaltyTabSwitcherProps {
  programTab: ReactNode;
  membersTab: ReactNode;
  referralsTab: ReactNode;
}

const tabs = [
  { key: "program", label: "Program" },
  { key: "members", label: "Members" },
  { key: "referrals", label: "Referrals" },
] as const;

type TabKey = (typeof tabs)[number]["key"];

export function LoyaltyTabSwitcher({
  programTab,
  membersTab,
  referralsTab,
}: LoyaltyTabSwitcherProps) {
  const [activeTab, setActiveTab] = useState<TabKey>("program");

  return (
    <div className="space-y-6">
      {/* Tab bar */}
      <nav className="flex gap-0 border-b border-[color:var(--ink-900)]/6" role="tablist">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            role="tab"
            aria-selected={activeTab === tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2.5 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? "border-b-2 border-[color:var(--ink-900)] text-[color:var(--ink-900)]"
                : "text-[color:var(--ink-900)]/40 hover:text-[color:var(--ink-900)]/70"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {/* Tab content */}
      <div>
        {activeTab === "program" && programTab}
        {activeTab === "members" && membersTab}
        {activeTab === "referrals" && referralsTab}
      </div>
    </div>
  );
}
```

### TDD

- [ ] Run: `cd apps/admin && npx next build` (or `npm run build`) — verify no TypeScript errors.

### Commit

```
feat(admin): add tabbed loyalty page with program config, tier builder, members, and referrals
```

---

## Task 12: Storefront — Account Loyalty Page + Checkout Points Toggle

**Files to create:**
- `apps/storefront/app/account/loyalty/page.tsx`
- `apps/storefront/components/loyalty/LoyaltyDashboard.tsx`
- `apps/storefront/components/checkout/PointsRedemptionToggle.tsx`
- `apps/storefront/lib/api/loyalty.ts`

### Step 12.1: Storefront API client

- [ ] Create `apps/storefront/lib/api/loyalty.ts`:

```typescript
// apps/storefront/lib/api/loyalty.ts

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

interface LoyaltyProgramPublic {
  is_active: boolean;
  points_per_dollar: string;
  points_currency: string;
  signup_bonus: number;
  referral_bonus: number;
  referee_bonus: number;
  min_redeem_points: number;
  points_value: string;
  tiers: { name: string; min_points: number; multiplier: string }[];
}

interface CustomerLoyalty {
  points_balance: number;
  lifetime_points: number;
  tier: string;
  referral_code: string;
}

interface RedeemResult {
  points_redeemed: number;
  value: string;
}

export async function getProgram(
  storeSlug: string,
  storefrontKey: string,
): Promise<LoyaltyProgramPublic | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/program`,
    {
      headers: { "X-Storefront-Key": storefrontKey },
      cache: "no-store",
    },
  );
  if (!res.ok) return null;
  const json = await res.json();
  return json.data ?? null;
}

export async function enrollCustomer(
  storeSlug: string,
  storefrontKey: string,
  email: string,
  name?: string,
  referralCode?: string,
): Promise<CustomerLoyalty | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/enroll`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Storefront-Key": storefrontKey,
      },
      body: JSON.stringify({ email, name, referral_code: referralCode }),
    },
  );
  if (!res.ok) return null;
  const json = await res.json();
  return json.data ?? null;
}

export async function getMe(
  storeSlug: string,
  storefrontKey: string,
  email: string,
): Promise<CustomerLoyalty | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/me?email=${encodeURIComponent(email)}`,
    {
      headers: { "X-Storefront-Key": storefrontKey },
      cache: "no-store",
    },
  );
  if (!res.ok) return null;
  const json = await res.json();
  return json.data ?? null;
}

export async function redeemPoints(
  storeSlug: string,
  storefrontKey: string,
  email: string,
  points: number,
): Promise<RedeemResult | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/redeem`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Storefront-Key": storefrontKey,
      },
      body: JSON.stringify({ email, points }),
    },
  );
  if (!res.ok) return null;
  const json = await res.json();
  return json.data ?? null;
}

export type { LoyaltyProgramPublic, CustomerLoyalty, RedeemResult };
```

### Step 12.2: Loyalty dashboard component

- [ ] Create `apps/storefront/components/loyalty/LoyaltyDashboard.tsx`:

```tsx
"use client";

import type {
  LoyaltyProgramPublic,
  CustomerLoyalty,
} from "@/lib/api/loyalty";

interface LoyaltyDashboardProps {
  program: LoyaltyProgramPublic;
  customer: CustomerLoyalty | null;
  onEnroll?: () => void;
}

export function LoyaltyDashboard({
  program,
  customer,
  onEnroll,
}: LoyaltyDashboardProps) {
  if (!customer) {
    return (
      <div className="space-y-6">
        <div className="rounded-[6px] bg-white px-6 py-8 text-center">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
            Join our loyalty program
          </h2>
          <p className="mt-2 text-sm text-[color:var(--ink-900)]/60">
            Earn {program.points_currency} on every purchase and unlock exclusive rewards.
          </p>
          {program.signup_bonus > 0 && (
            <p className="mt-1 text-sm text-[color:var(--moss-700)] font-medium">
              Get {program.signup_bonus} {program.points_currency} just for joining!
            </p>
          )}
          {onEnroll && (
            <button
              onClick={onEnroll}
              className="mt-4 rounded-[6px] bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-white hover:bg-[color:var(--ink-900)]/90 transition-colors"
            >
              Join now
            </button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Balance card */}
      <div className="rounded-[6px] bg-white px-6 py-6">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Your {program.points_currency}
            </p>
            <p className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium text-[color:var(--ink-900)]">
              {customer.points_balance.toLocaleString()}
            </p>
            <p className="mt-0.5 text-xs text-[color:var(--ink-900)]/40">
              Lifetime: {customer.lifetime_points.toLocaleString()}
            </p>
          </div>
          <span className="inline-block rounded-[4px] bg-[color:var(--moss-700)]/10 px-3 py-1 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)]">
            {customer.tier}
          </span>
        </div>
      </div>

      {/* Referral card */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
          Your referral code
        </h3>
        <div className="mt-2 flex items-center gap-3">
          <code className="rounded-[4px] bg-[color:var(--paper-200)] px-3 py-1.5 text-sm font-mono font-medium text-[color:var(--ink-900)]">
            {customer.referral_code}
          </code>
          <button
            onClick={() => navigator.clipboard.writeText(customer.referral_code)}
            className="rounded-[6px] px-3 py-1.5 text-xs font-medium text-[color:var(--moss-700)] hover:bg-[color:var(--moss-700)]/5 transition-colors"
          >
            Copy
          </button>
        </div>
        {program.referral_bonus > 0 && (
          <p className="mt-2 text-xs text-[color:var(--ink-900)]/40">
            Share this code and earn {program.referral_bonus} {program.points_currency} for each friend who joins.
          </p>
        )}
      </div>

      {/* Tiers */}
      {program.tiers.length > 0 && (
        <div className="rounded-[6px] bg-white px-6 py-5">
          <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50 mb-3">
            Tiers
          </h3>
          <div className="space-y-2">
            {program.tiers.map((tier) => (
              <div
                key={tier.name}
                className={`flex items-center justify-between rounded-[6px] px-4 py-2.5 ${
                  customer.tier === tier.name.toLowerCase()
                    ? "bg-[color:var(--moss-700)]/5 border border-[color:var(--moss-700)]/20"
                    : "bg-[color:var(--paper-200)]"
                }`}
              >
                <span className="text-sm font-medium text-[color:var(--ink-900)]">
                  {tier.name}
                </span>
                <span className="text-xs text-[color:var(--ink-900)]/50">
                  {tier.min_points.toLocaleString()} pts &middot; {tier.multiplier}x
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
```

### Step 12.3: Points redemption toggle for checkout

- [ ] Create `apps/storefront/components/checkout/PointsRedemptionToggle.tsx`:

```tsx
"use client";

import { useState } from "react";

interface PointsRedemptionToggleProps {
  pointsBalance: number;
  pointsValue: string; // e.g. "0.01"
  pointsCurrency: string;
  minRedeemPoints: number;
  onToggle: (redeemPoints: number | null) => void;
}

export function PointsRedemptionToggle({
  pointsBalance,
  pointsValue,
  pointsCurrency,
  minRedeemPoints,
  onToggle,
}: PointsRedemptionToggleProps) {
  const [isRedeeming, setIsRedeeming] = useState(false);
  const canRedeem = pointsBalance >= minRedeemPoints;

  const monetaryValue = (
    pointsBalance * parseFloat(pointsValue)
  ).toFixed(2);

  if (!canRedeem) {
    return (
      <div className="text-xs text-[color:var(--ink-900)]/40">
        You have {pointsBalance.toLocaleString()} {pointsCurrency} (min {minRedeemPoints} to redeem)
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between py-2">
      <div>
        <p className="text-sm text-[color:var(--ink-900)]">
          Use {pointsBalance.toLocaleString()} {pointsCurrency}
        </p>
        <p className="text-xs text-[color:var(--ink-900)]/50">
          Worth ${monetaryValue}
        </p>
      </div>
      <label className="relative inline-flex cursor-pointer items-center">
        <input
          type="checkbox"
          checked={isRedeeming}
          onChange={(e) => {
            setIsRedeeming(e.target.checked);
            onToggle(e.target.checked ? pointsBalance : null);
          }}
          className="peer sr-only"
        />
        <div className="h-6 w-11 rounded-full bg-[color:var(--ink-900)]/10 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all peer-checked:bg-[color:var(--moss-700)] peer-checked:after:translate-x-full" />
      </label>
    </div>
  );
}
```

### Step 12.4: Account loyalty page

- [ ] Ensure directory: `mkdir -p apps/storefront/app/account/loyalty`

- [ ] Create `apps/storefront/app/account/loyalty/page.tsx`:

```tsx
import { LoyaltyDashboard } from "@/components/loyalty/LoyaltyDashboard";
import { getProgram, getMe } from "@/lib/api/loyalty";

// This page is accessed by logged-in customers. The store slug and
// customer email come from the storefront session/context.

export default async function LoyaltyAccountPage() {
  // These values should come from your storefront session middleware.
  // Adjust the imports to match your actual session utility.
  const storeSlug = process.env.STORE_SLUG ?? "";
  const storefrontKey = process.env.STOREFRONT_KEY ?? "";
  const customerEmail = ""; // TODO: get from session

  const program = await getProgram(storeSlug, storefrontKey);

  if (!program || !program.is_active) {
    return (
      <div className="mx-auto max-w-2xl py-12 px-4">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          Loyalty program is not available for this store.
        </p>
      </div>
    );
  }

  const customer = customerEmail
    ? await getMe(storeSlug, storefrontKey, customerEmail)
    : null;

  return (
    <div className="mx-auto max-w-2xl py-12 px-4 space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-[color:var(--ink-900)]">
          Loyalty
        </h1>
      </header>
      <LoyaltyDashboard program={program} customer={customer} />
    </div>
  );
}
```

### TDD

- [ ] Run: `cd apps/storefront && npx next build` (or `npm run build`) — verify no TypeScript errors.
- [ ] Run: `cd apps/admin && npx next build`

### Commit

```
feat(storefront): add loyalty account page and checkout points redemption toggle
```

---

## Task 13: Build Verification + Final Commit

### Step 13.1: Full build check

- [ ] Go build: `cd services/marketplace-api && go build ./...`
- [ ] Go vet: `cd services/marketplace-api && go vet ./...`
- [ ] Go test: `cd services/marketplace-api && go test ./internal/loyalty/... -v`
- [ ] Admin build: `cd apps/admin && npm run build`
- [ ] Storefront build: `cd apps/storefront && npm run build`

### Step 13.2: Verify all new files exist

```bash
# Backend
ls services/marketplace-api/migrations/000011_loyalty.up.sql
ls services/marketplace-api/migrations/000011_loyalty.down.sql
ls services/marketplace-api/internal/loyalty/models.go
ls services/marketplace-api/internal/loyalty/models_test.go
ls services/marketplace-api/internal/loyalty/repository.go
ls services/marketplace-api/internal/loyalty/repository_test.go
ls services/marketplace-api/internal/loyalty/service.go
ls services/marketplace-api/internal/loyalty/service_test.go
ls services/marketplace-api/internal/loyalty/expiry.go
ls services/marketplace-api/internal/loyalty/expiry_test.go
ls services/marketplace-api/internal/discount/applier.go
ls services/marketplace-api/internal/discount/loyalty_applier.go
ls services/marketplace-api/internal/handlers/admin/loyalty.go
ls services/marketplace-api/internal/handlers/admin/loyalty_dto.go
ls services/marketplace-api/internal/handlers/storefront/loyalty.go
ls services/marketplace-api/internal/authz/loyalty_roles.go

# Frontend
ls apps/admin/lib/api/loyalty-api.ts
ls apps/admin/app/marketing/loyalty/page.tsx
ls apps/admin/app/marketing/loyalty/LoyaltyTabSwitcher.tsx
ls apps/admin/components/marketing/TierBuilder.tsx
ls apps/admin/components/marketing/LoyaltyProgramTab.tsx
ls apps/admin/components/marketing/LoyaltyMembersTab.tsx
ls apps/admin/components/marketing/LoyaltyReferralsTab.tsx
ls apps/storefront/lib/api/loyalty.ts
ls apps/storefront/app/account/loyalty/page.tsx
ls apps/storefront/components/loyalty/LoyaltyDashboard.tsx
ls apps/storefront/components/checkout/PointsRedemptionToggle.tsx
```

### Step 13.3: Integration test checklist (manual or automated)

- [ ] POST loyalty program config (admin) and verify GET returns it
- [ ] POST enroll (storefront) and verify 200 with points_balance = signup_bonus
- [ ] GET /me (storefront) returns enrolled customer
- [ ] POST /redeem with insufficient points returns 422
- [ ] POST /adjust (admin) credits points and verify transaction record
- [ ] Tier calculation promotes customer after enough lifetime points
- [ ] Referral flow: enroll with referral_code, verify both referrer and referee get bonuses
- [ ] Concurrent redemption: two simultaneous debit requests, only one succeeds (race test)

### Commit

```
test(loyalty): verify full M3 loyalty build — backend + admin + storefront
```

---

## Summary of All Files

### New files (backend — 16 files):

| File | Purpose |
|------|---------|
| `services/marketplace-api/migrations/000011_loyalty.up.sql` | Loyalty schema |
| `services/marketplace-api/migrations/000011_loyalty.down.sql` | Rollback |
| `services/marketplace-api/internal/loyalty/models.go` | GORM models + constants |
| `services/marketplace-api/internal/loyalty/models_test.go` | Model tests |
| `services/marketplace-api/internal/loyalty/repository.go` | Data access + referral code gen |
| `services/marketplace-api/internal/loyalty/repository_test.go` | Repository tests |
| `services/marketplace-api/internal/loyalty/service.go` | Business logic |
| `services/marketplace-api/internal/loyalty/service_test.go` | Service tests |
| `services/marketplace-api/internal/loyalty/expiry.go` | Background point expiry worker |
| `services/marketplace-api/internal/loyalty/expiry_test.go` | Worker test |
| `services/marketplace-api/internal/discount/applier.go` | Discount interface |
| `services/marketplace-api/internal/discount/loyalty_applier.go` | Loyalty discount impl |
| `services/marketplace-api/internal/handlers/admin/loyalty.go` | Admin handlers |
| `services/marketplace-api/internal/handlers/admin/loyalty_dto.go` | Admin DTOs |
| `services/marketplace-api/internal/handlers/storefront/loyalty.go` | Storefront handlers |
| `services/marketplace-api/internal/authz/loyalty_roles.go` | AuthZ role constants |

### Modified files (backend — 4 files):

| File | Change |
|------|--------|
| `services/marketplace-api/migrations.go` | Bump ExpectedSchemaVersion |
| `services/marketplace-api/pkg/apperrors/errors.go` | Add loyalty error codes |
| `services/marketplace-api/internal/handlers/admin/errors.go` | Add loyalty HTTP status mappings |
| `services/marketplace-api/internal/handlers/admin/routes.go` | Add LoyaltyHandler to Deps + routes |
| `services/marketplace-api/internal/handlers/storefront/routes.go` | Add LoyaltyHandler to Deps + routes |
| `services/marketplace-api/internal/handlers/storefront/checkout_ext.go` | Add loyalty service field + post-checkout award |
| `services/marketplace-api/cmd/marketplace-api/main.go` | Wire loyalty repo/svc/handler + expiry worker |

### New files (frontend — 11 files):

| File | Purpose |
|------|---------|
| `apps/admin/lib/api/loyalty-api.ts` | Admin API client |
| `apps/admin/app/marketing/loyalty/page.tsx` | Tabbed loyalty page |
| `apps/admin/app/marketing/loyalty/LoyaltyTabSwitcher.tsx` | Client tab switcher |
| `apps/admin/components/marketing/TierBuilder.tsx` | Structured tier list editor |
| `apps/admin/components/marketing/LoyaltyProgramTab.tsx` | Program config form |
| `apps/admin/components/marketing/LoyaltyMembersTab.tsx` | Members table |
| `apps/admin/components/marketing/LoyaltyReferralsTab.tsx` | Referrals table |
| `apps/storefront/lib/api/loyalty.ts` | Storefront API client |
| `apps/storefront/app/account/loyalty/page.tsx` | Account loyalty page |
| `apps/storefront/components/loyalty/LoyaltyDashboard.tsx` | Points + referral UI |
| `apps/storefront/components/checkout/PointsRedemptionToggle.tsx` | Checkout points toggle |

### Commit sequence:

1. `feat(marketplace-api): add migration 000011 for loyalty program tables`
2. `feat(loyalty): add GORM models for loyalty program, customer loyalty, transactions, referrals`
3. `feat(loyalty): add repository with atomic point operations and referral code generation`
4. `feat(loyalty): add service layer with enroll, award, redeem, adjust, tier calculation`
5. `feat(loyalty): add point expiry background worker with batch processing`
6. `feat(loyalty): add domain error codes for insufficient points and not enrolled`
7. `feat(loyalty): add admin handlers for program config, members, referrals, and point adjustment`
8. `feat(loyalty): add storefront handlers for program info, enrollment, me, and redemption`
9. `feat(loyalty): add discount.Applier interface and loyalty checkout integration`
10. `feat(loyalty): wire loyalty routes, handlers, and expiry worker into main.go`
11. `feat(admin): add tabbed loyalty page with program config, tier builder, members, and referrals`
12. `feat(storefront): add loyalty account page and checkout points redemption toggle`
13. `test(loyalty): verify full M3 loyalty build — backend + admin + storefront`
