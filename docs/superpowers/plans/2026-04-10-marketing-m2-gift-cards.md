# Marketing M2 — Gift Cards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship gift card issuance (admin), balance check (storefront), checkout integration as payment method, with atomic balance debit using SELECT FOR UPDATE and crypto/rand code generation.

**Architecture:** New `internal/giftcard/` package (models, repository, service) + admin handler + storefront handler. Migration 000010. Implements `discount.Applier` for checkout. Gift card codes: 16-char base32, crypto/rand.

**Tech Stack:** Go 1.26, Gin, GORM, shopspring/decimal, crypto/rand. Next.js 16, React 19, Tailwind.

---

## Post-Review Amendments (2026-04-10)

> These amendments override the corresponding sections in the plan below.

### CRITICAL FIX 1: Gift card debit MUST run inside the order creation transaction

The plan acknowledges (Key Design Decisions item 3) that gift card debit happens in a separate transaction after order creation. This violates spec §8.2: "All discount steps (coupon apply, gift card debit, order creation, tax line saves) MUST be in a single `db.Transaction()` call." If payment creation fails after the gift card debit succeeds, the customer loses balance with no order.

**Required change:** `GiftCardApplier.Apply(ctx, tx, orderID, amount)` must receive the `tx` from the order creation transaction and call `repo.DebitInTx(tx, ...)` inside it. The debit rolls back if order creation fails.

### CRITICAL FIX 2: Code generation entropy must be 128 bits

The plan uses 10 bytes = 80 bits and incorrectly claims this "exceeds the 128-bit spec requirement." 10 bytes is 80 bits, not 128. **Fix:** Use 16 bytes (128 bits) of `crypto/rand`, then base32-encode and take the first 26 characters. Update `GenerateCode()` accordingly.

### HIGH FIX 3: Use atomic UPDATE WHERE instead of SELECT FOR UPDATE

Spec §8.1 prescribes: `UPDATE gift_cards SET current_balance = current_balance - $amount WHERE id = $id AND current_balance >= $amount RETURNING current_balance`. The plan uses SELECT FOR UPDATE + app-level check + separate UPDATE — two round trips, longer lock hold. **Fix:** Use the single atomic UPDATE pattern. Zero rows = insufficient balance.

### HIGH FIX 4: Add concurrent debit race test

Spec §12 requires a "concurrent debit race test." The plan's tests only cover `GenerateCode` and `FormatCodeDisplay`. **Fix:** Add an integration test that spawns N goroutines calling `DebitInTx` against real Postgres with `go test -race`. Only one should succeed when balance < 2×amount.

### HIGH FIX 5: `GiftCardApplier.Apply` must handle partial debit

When gift card balance < order total, `Apply` should compute `min(balance, amount)` and debit that partial amount, returning the actual deducted amount. Currently it passes the full `amount` to `Debit` which fails with `CodeInsufficientGiftCardBalance` instead of partially applying.

### MEDIUM FIX 6: Rate limiter needs cleanup goroutine

Same as M1 — `RateLimiter.clients` map grows unboundedly. Add periodic cleanup (every 5 min, remove entries older than 2× window).

### MEDIUM FIX 7: Issue page missing AdminShell wrapper

`IssueGiftCardPage` is `"use client"` but doesn't wrap in `AdminShell`. The list page wraps correctly but the issue page renders bare `<main>`. Wrap consistently.

### LOW FIX 8: Add `GetByCode` to Service public API

Checkout integration calls `h.giftCardSvc.GetByCode(ctx, storeID, code)` but the Service only exposes `CheckBalance` and `GetByID`. Add a public `GetByCode` method.

### LOW FIX 9: Use `errors.As` instead of manual type assertion

Storefront handler uses `err.(*apperrors.Error)` — wrapped errors won't match. Use `errors.As(&ae, err)` instead.

---

## Pre-requisites

- M1 (Coupons) must be merged — migration 000009 must exist. M2 uses migration 000010.
- Existing patterns from `internal/order/` (GORM models, repository interfaces, service layer) are the template.
- `pkg/apperrors/` error envelope pattern is the error standard.
- `internal/handlers/admin/errors.go` `RespondErr` + `codeStatus` map is the HTTP error mapping standard.
- `internal/handlers/admin/routes.go` `Deps` struct + `RegisterAdmin` is the route wiring standard.
- `internal/handlers/storefront/routes.go` `Deps` struct + `RegisterStorefront` is the storefront route standard.
- `internal/handlers/storefront/checkout_ext.go` is the checkout flow to integrate into.
- `cmd/marketplace-api/main.go` is the wiring entrypoint.
- `migrations.go` `ExpectedSchemaVersion` must be bumped.
- Admin UI uses `apps/admin/lib/api/marketplace-api.ts` for API client and server components.
- Storefront uses `apps/storefront/lib/api/checkout-api.ts` for API client and client components.
- AdminShell sidebar already has placeholder "Gift Cards" item pointing to `/dashboard` — update to `/marketing/gift-cards`.

---

## Task 1: Migration 000010 — Gift Cards Schema

**Files to create:**
- `services/marketplace-api/migrations/000010_gift_cards.up.sql`
- `services/marketplace-api/migrations/000010_gift_cards.down.sql`

**Files to edit:**
- `services/marketplace-api/migrations.go` — bump `ExpectedSchemaVersion`

### Steps

- [ ] **1.1** Create `services/marketplace-api/migrations/000010_gift_cards.up.sql`:

```sql
-- 000010_gift_cards.up.sql
-- Marketing M2: Gift cards with transaction ledger.

CREATE TABLE gift_cards (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    code            VARCHAR(50)   NOT NULL,
    initial_balance NUMERIC(12,2) NOT NULL,
    current_balance NUMERIC(12,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    status          VARCHAR(20)   NOT NULL DEFAULT 'active',
    sender_name     VARCHAR(200),
    sender_email    VARCHAR(300),
    recipient_name  VARCHAR(200),
    recipient_email VARCHAR(300),
    message         TEXT,
    purchased_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, code),
    CHECK (current_balance >= 0),
    CHECK (initial_balance > 0)
);
CREATE INDEX gift_cards_store_status_idx ON gift_cards (store_id, status);
CREATE INDEX gift_cards_tenant_idx ON gift_cards (tenant_id);

CREATE TABLE gift_card_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    gift_card_id    UUID          NOT NULL REFERENCES gift_cards(id) ON DELETE CASCADE,
    order_id        UUID,
    type            VARCHAR(20)   NOT NULL,
    amount          NUMERIC(12,2) NOT NULL,
    balance_after   NUMERIC(12,2) NOT NULL,
    note            VARCHAR(200),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CHECK (balance_after >= 0)
);
CREATE INDEX gc_txn_card_idx ON gift_card_transactions (gift_card_id);
CREATE INDEX gc_txn_tenant_idx ON gift_card_transactions (tenant_id);
```

- [ ] **1.2** Create `services/marketplace-api/migrations/000010_gift_cards.down.sql`:

```sql
-- 000010_gift_cards.down.sql
DROP TABLE IF EXISTS gift_card_transactions;
DROP TABLE IF EXISTS gift_cards;
```

- [ ] **1.3** Edit `services/marketplace-api/migrations.go` — change `ExpectedSchemaVersion` from its current value. Find the current value and increment by 1. If M1 (coupons, migration 000009) has already bumped it, this will be the next increment. The constant must match the highest migration number applied.

**Important:** If migration 000009 (coupons) does not yet exist, you are blocked. Verify `migrations/000009_*.sql` exists before proceeding.

- [ ] **1.4** Run migration and verify:

```bash
cd services/marketplace-api && make mp-migrate-up
```

**TDD:** After migration, connect to the database and verify:
```sql
SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'gift_cards' ORDER BY ordinal_position;
SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'gift_card_transactions' ORDER BY ordinal_position;
-- Verify CHECK constraint: INSERT with current_balance = -1 should fail.
```

**Commit:** `feat(marketplace-api): add migration 000010 for gift cards schema`

---

## Task 2: `internal/giftcard/` Package — Models, Repository, Service

### 2.1 Models

**File to create:** `services/marketplace-api/internal/giftcard/models.go`

- [ ] **2.1.1** Create `services/marketplace-api/internal/giftcard/models.go`:

```go
// Package giftcard implements the gift card domain: models, repository,
// service layer with crypto/rand code generation and atomic balance debit.
package giftcard

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GiftCardStatus enumerates the lifecycle states of a gift card.
type GiftCardStatus string

const (
	StatusActive   GiftCardStatus = "active"
	StatusDisabled GiftCardStatus = "disabled"
	StatusDepleted GiftCardStatus = "depleted"
)

// TransactionType enumerates the types of balance mutations.
type TransactionType string

const (
	TxnPurchase   TransactionType = "purchase"
	TxnRedeem     TransactionType = "redeem"
	TxnRefund     TransactionType = "refund"
	TxnAdjustment TransactionType = "adjustment"
)

// GiftCard is the GORM model for the gift_cards table.
type GiftCard struct {
	ID             uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	Code           string          `gorm:"column:code;type:varchar(50);not null"`
	InitialBalance decimal.Decimal `gorm:"column:initial_balance;type:numeric(12,2);not null"`
	CurrentBalance decimal.Decimal `gorm:"column:current_balance;type:numeric(12,2);not null"`
	CurrencyCode   string          `gorm:"column:currency_code;type:char(3);not null"`
	Status         GiftCardStatus  `gorm:"column:status;type:varchar(20);not null;default:active"`
	SenderName     *string         `gorm:"column:sender_name;type:varchar(200)"`
	SenderEmail    *string         `gorm:"column:sender_email;type:varchar(300)"`
	RecipientName  *string         `gorm:"column:recipient_name;type:varchar(200)"`
	RecipientEmail *string         `gorm:"column:recipient_email;type:varchar(300)"`
	Message        *string         `gorm:"column:message;type:text"`
	PurchasedAt    *time.Time      `gorm:"column:purchased_at"`
	ExpiresAt      *time.Time      `gorm:"column:expires_at"`
	CreatedAt      time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt      time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (GiftCard) TableName() string { return "gift_cards" }

// Transaction is the GORM model for the gift_card_transactions table.
type Transaction struct {
	ID           uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	GiftCardID   uuid.UUID       `gorm:"column:gift_card_id;type:uuid;not null"`
	OrderID      *uuid.UUID      `gorm:"column:order_id;type:uuid"`
	Type         TransactionType `gorm:"column:type;type:varchar(20);not null"`
	Amount       decimal.Decimal `gorm:"column:amount;type:numeric(12,2);not null"`
	BalanceAfter decimal.Decimal `gorm:"column:balance_after;type:numeric(12,2);not null"`
	Note         *string         `gorm:"column:note;type:varchar(200)"`
	CreatedAt    time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (Transaction) TableName() string { return "gift_card_transactions" }
```

### 2.2 Repository

**File to create:** `services/marketplace-api/internal/giftcard/repository.go`

- [ ] **2.2.1** Create `services/marketplace-api/internal/giftcard/repository.go`:

```go
package giftcard

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for gift cards. Every mutating
// method takes an explicit *gorm.DB so callers can thread a transaction.
type Repository interface {
	// CreateInTx inserts a gift card and its initial "purchase" transaction
	// inside the provided tx.
	CreateInTx(tx *gorm.DB, gc *GiftCard, initialTxn *Transaction) error

	// GetByID returns the gift card or apperrors.ErrNotFound.
	GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID, storeID uuid.UUID) (*GiftCard, error)

	// GetByCode returns the gift card matching (store_id, code), or
	// apperrors.ErrNotFound.
	GetByCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*GiftCard, error)

	// ListByStore returns paginated gift cards for a store, optionally
	// filtered by status.
	ListByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID, tenantID uuid.UUID, status *GiftCardStatus, page, pageSize int) ([]GiftCard, int64, error)

	// DebitInTx atomically debits the gift card balance using SELECT FOR
	// UPDATE. Returns the new balance or apperrors.ErrInsufficientGiftCardBalance
	// if the balance is insufficient. Inserts a "redeem" transaction row.
	// The caller must supply an open tx.
	DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// CreditInTx atomically credits the gift card balance (for refunds).
	// Inserts a transaction row of the given type.
	CreditInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID *uuid.UUID, txnType TransactionType, note *string, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// ListTransactions returns all transactions for a gift card, ordered
	// by created_at desc.
	ListTransactions(ctx context.Context, db *gorm.DB, giftCardID uuid.UUID) ([]Transaction, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) CreateInTx(tx *gorm.DB, gc *GiftCard, initialTxn *Transaction) error {
	if err := tx.Create(gc).Error; err != nil {
		return err
	}
	initialTxn.GiftCardID = gc.ID
	return tx.Create(initialTxn).Error
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID, storeID uuid.UUID) (*GiftCard, error) {
	var gc GiftCard
	err := db.WithContext(ctx).Where("id = ? AND store_id = ?", id, storeID).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("gift card")
	}
	return &gc, err
}

func (gormRepository) GetByCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*GiftCard, error) {
	var gc GiftCard
	err := db.WithContext(ctx).Where("store_id = ? AND code = ?", storeID, code).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("gift card")
	}
	return &gc, err
}

func (gormRepository) ListByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID, tenantID uuid.UUID, status *GiftCardStatus, page, pageSize int) ([]GiftCard, int64, error) {
	q := db.WithContext(ctx).Where("store_id = ? AND tenant_id = ?", storeID, tenantID)
	if status != nil {
		q = q.Where("status = ?", *status)
	}

	var total int64
	if err := q.Model(&GiftCard{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var cards []GiftCard
	offset := (page - 1) * pageSize
	err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&cards).Error
	return cards, total, err
}

// DebitInTx uses SELECT FOR UPDATE to lock the row, then a single atomic
// UPDATE ... WHERE current_balance >= amount. Zero rows affected means
// insufficient balance. Inserts a "redeem" transaction with the new balance.
func (gormRepository) DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (decimal.Decimal, error) {
	// Lock the row.
	var gc GiftCard
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", cardID).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, apperrors.NotFound("gift card")
	}
	if err != nil {
		return decimal.Zero, err
	}

	if gc.CurrentBalance.LessThan(amount) {
		return decimal.Zero, apperrors.New(apperrors.CodeInsufficientGiftCardBalance,
			"gift card balance is insufficient for this transaction")
	}

	newBalance := gc.CurrentBalance.Sub(amount)

	// Determine new status.
	newStatus := StatusActive
	if newBalance.IsZero() {
		newStatus = StatusDepleted
	}

	// Atomic update.
	result := tx.Model(&GiftCard{}).
		Where("id = ? AND current_balance >= ?", cardID, amount).
		Updates(map[string]interface{}{
			"current_balance": newBalance,
			"status":          newStatus,
			"updated_at":      gorm.Expr("now()"),
		})
	if result.Error != nil {
		return decimal.Zero, result.Error
	}
	if result.RowsAffected == 0 {
		// Race condition: another tx debited first.
		return decimal.Zero, apperrors.New(apperrors.CodeInsufficientGiftCardBalance,
			"gift card balance is insufficient for this transaction")
	}

	// Insert redeem transaction.
	txn := Transaction{
		TenantID:     tenantID,
		GiftCardID:   cardID,
		OrderID:      &orderID,
		Type:         TxnRedeem,
		Amount:       amount.Neg(), // negative = debit
		BalanceAfter: newBalance,
	}
	if err := tx.Create(&txn).Error; err != nil {
		return decimal.Zero, err
	}

	return newBalance, nil
}

func (gormRepository) CreditInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID *uuid.UUID, txnType TransactionType, note *string, tenantID uuid.UUID) (decimal.Decimal, error) {
	var gc GiftCard
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", cardID).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, apperrors.NotFound("gift card")
	}
	if err != nil {
		return decimal.Zero, err
	}

	newBalance := gc.CurrentBalance.Add(amount)

	// Re-activate if was depleted.
	newStatus := gc.Status
	if gc.Status == StatusDepleted && newBalance.GreaterThan(decimal.Zero) {
		newStatus = StatusActive
	}

	result := tx.Model(&GiftCard{}).
		Where("id = ?", cardID).
		Updates(map[string]interface{}{
			"current_balance": newBalance,
			"status":          newStatus,
			"updated_at":      gorm.Expr("now()"),
		})
	if result.Error != nil {
		return decimal.Zero, result.Error
	}

	txn := Transaction{
		TenantID:     tenantID,
		GiftCardID:   cardID,
		OrderID:      orderID,
		Type:         txnType,
		Amount:       amount, // positive = credit
		BalanceAfter: newBalance,
		Note:         note,
	}
	if err := tx.Create(&txn).Error; err != nil {
		return decimal.Zero, err
	}

	return newBalance, nil
}

func (gormRepository) ListTransactions(ctx context.Context, db *gorm.DB, giftCardID uuid.UUID) ([]Transaction, error) {
	var txns []Transaction
	err := db.WithContext(ctx).
		Where("gift_card_id = ?", giftCardID).
		Order("created_at DESC").
		Find(&txns).Error
	return txns, err
}
```

### 2.3 Service

**File to create:** `services/marketplace-api/internal/giftcard/service.go`

- [ ] **2.3.1** Create `services/marketplace-api/internal/giftcard/service.go`:

```go
package giftcard

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// IssueInput holds the fields needed to issue a new gift card.
type IssueInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	InitialBalance decimal.Decimal
	CurrencyCode   string
	SenderName     *string
	SenderEmail    *string
	RecipientName  *string
	RecipientEmail *string
	Message        *string
	ExpiresAt      *time.Time
}

// BalanceResult is the response for a balance check.
type BalanceResult struct {
	Code           string          `json:"code"`
	CurrentBalance decimal.Decimal `json:"current_balance"`
	CurrencyCode   string          `json:"currency_code"`
	Status         GiftCardStatus  `json:"status"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
}

// Service contains the business logic for gift cards.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a gift card Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// Unit runs fn inside a database transaction.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// Issue creates a new gift card with a cryptographically random code.
func (s *Service) Issue(ctx context.Context, in IssueInput) (*GiftCard, error) {
	if in.InitialBalance.LessThanOrEqual(decimal.Zero) {
		return nil, apperrors.ValidationFailed("initial_balance", "must be greater than zero")
	}
	if in.CurrencyCode == "" {
		return nil, apperrors.ValidationFailed("currency_code", "required")
	}

	code, err := GenerateCode()
	if err != nil {
		return nil, fmt.Errorf("generate gift card code: %w", err)
	}

	now := time.Now()
	gc := GiftCard{
		TenantID:       in.TenantID,
		StoreID:        in.StoreID,
		Code:           code,
		InitialBalance: in.InitialBalance,
		CurrentBalance: in.InitialBalance,
		CurrencyCode:   strings.ToUpper(in.CurrencyCode),
		Status:         StatusActive,
		SenderName:     in.SenderName,
		SenderEmail:    in.SenderEmail,
		RecipientName:  in.RecipientName,
		RecipientEmail: in.RecipientEmail,
		Message:        in.Message,
		PurchasedAt:    &now,
		ExpiresAt:      in.ExpiresAt,
	}

	initialTxn := Transaction{
		TenantID:     in.TenantID,
		Type:         TxnPurchase,
		Amount:       in.InitialBalance,
		BalanceAfter: in.InitialBalance,
	}

	err = s.Unit(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateInTx(tx, &gc, &initialTxn)
	})
	if err != nil {
		return nil, err
	}

	return &gc, nil
}

// GetByID returns a single gift card with transactions.
func (s *Service) GetByID(ctx context.Context, storeID, id uuid.UUID) (*GiftCard, []Transaction, error) {
	gc, err := s.repo.GetByID(ctx, s.db, id, storeID)
	if err != nil {
		return nil, nil, err
	}
	txns, err := s.repo.ListTransactions(ctx, s.db, gc.ID)
	if err != nil {
		return gc, nil, err
	}
	return gc, txns, nil
}

// CheckBalance looks up a gift card by code and returns the balance.
// Returns domain errors for not-found, expired, or disabled cards.
func (s *Service) CheckBalance(ctx context.Context, storeID uuid.UUID, code string) (*BalanceResult, error) {
	gc, err := s.repo.GetByCode(ctx, s.db, storeID, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}

	if gc.Status == StatusDisabled {
		return nil, apperrors.New(apperrors.CodeGiftCardNotFound, "gift card not found")
	}
	if gc.ExpiresAt != nil && gc.ExpiresAt.Before(time.Now()) {
		return nil, apperrors.New(apperrors.CodeGiftCardExpired, "gift card has expired")
	}

	return &BalanceResult{
		Code:           gc.Code,
		CurrentBalance: gc.CurrentBalance,
		CurrencyCode:   gc.CurrencyCode,
		Status:         gc.Status,
		ExpiresAt:      gc.ExpiresAt,
	}, nil
}

// ListByStore returns paginated gift cards.
func (s *Service) ListByStore(ctx context.Context, storeID, tenantID uuid.UUID, status *GiftCardStatus, page, pageSize int) ([]GiftCard, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListByStore(ctx, s.db, storeID, tenantID, status, page, pageSize)
}

// Debit atomically deducts amount from the gift card inside the given tx.
// This is called from checkout — the tx is owned by the checkout handler.
func (s *Service) Debit(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (decimal.Decimal, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, apperrors.ValidationFailed("amount", "must be greater than zero")
	}
	return s.repo.DebitInTx(tx, cardID, amount, orderID, tenantID)
}

// GenerateCode produces a 16-character uppercase base32 code using
// crypto/rand. Minimum 128-bit entropy (10 random bytes → 16 base32 chars).
// Format: XXXX-XXXX-XXXX-XXXX (stored without dashes, displayed with).
func GenerateCode() (string, error) {
	b := make([]byte, 10) // 10 bytes = 80 bits → 16 base32 chars
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	// Take first 16 characters, uppercase.
	if len(raw) < 16 {
		return "", fmt.Errorf("base32 output too short: %d", len(raw))
	}
	return strings.ToUpper(raw[:16]), nil
}

// FormatCodeDisplay formats a 16-char code with dashes: XXXX-XXXX-XXXX-XXXX.
func FormatCodeDisplay(code string) string {
	if len(code) != 16 {
		return code
	}
	return code[0:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:16]
}
```

### 2.4 Service Tests

**File to create:** `services/marketplace-api/internal/giftcard/service_test.go`

- [ ] **2.4.1** Create `services/marketplace-api/internal/giftcard/service_test.go`:

```go
package giftcard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCode(t *testing.T) {
	code, err := GenerateCode()
	require.NoError(t, err)
	assert.Len(t, code, 16, "code must be 16 characters")

	// All characters should be valid base32 (A-Z, 2-7).
	for _, c := range code {
		assert.True(t, (c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7'),
			"invalid base32 character: %c", c)
	}

	// Two generated codes should be different (probabilistic).
	code2, err := GenerateCode()
	require.NoError(t, err)
	assert.NotEqual(t, code, code2, "two codes should differ")
}

func TestFormatCodeDisplay(t *testing.T) {
	assert.Equal(t, "ABCD-EFGH-IJKL-MNOP", FormatCodeDisplay("ABCDEFGHIJKLMNOP"))
	assert.Equal(t, "SHORT", FormatCodeDisplay("SHORT"))
}
```

- [ ] **2.4.2** Run tests:

```bash
cd services/marketplace-api && go test ./internal/giftcard/... -v -count=1
```

**Commit:** `feat(marketplace-api): add giftcard package with models, repository, service, and code generation`

---

## Task 3: Domain Errors

**File to edit:** `services/marketplace-api/pkg/apperrors/errors.go`

- [ ] **3.1** Add gift card error codes to the `const` block, after the existing Orders slice 1 codes:

```go
	// Gift cards — Marketing M2.
	CodeInsufficientGiftCardBalance Code = "insufficient_gift_card_balance"
	CodeGiftCardExpired             Code = "gift_card_expired"
	CodeGiftCardNotFound            Code = "gift_card_not_found"
```

- [ ] **3.2** Add sentinel values to the `var` block:

```go
	// Gift card sentinels.
	ErrInsufficientGiftCardBalance = &Error{Code: CodeInsufficientGiftCardBalance}
	ErrGiftCardExpired             = &Error{Code: CodeGiftCardExpired}
	ErrGiftCardNotFound            = &Error{Code: CodeGiftCardNotFound}
```

- [ ] **3.3** Add to the `IsKnownCode` switch in `errors.go`:

Add `CodeInsufficientGiftCardBalance, CodeGiftCardExpired, CodeGiftCardNotFound` to the switch case list.

- [ ] **3.4** Edit `services/marketplace-api/internal/handlers/admin/errors.go` — add to the `codeStatus` map:

```go
	// Gift cards — Marketing M2.
	apperrors.CodeInsufficientGiftCardBalance: http.StatusUnprocessableEntity,
	apperrors.CodeGiftCardExpired:             http.StatusGone,
	apperrors.CodeGiftCardNotFound:            http.StatusNotFound,
```

- [ ] **3.5** Run existing error tests to ensure nothing broke:

```bash
cd services/marketplace-api && go test ./pkg/apperrors/... -v -count=1
cd services/marketplace-api && go test ./internal/handlers/admin/... -run TestRespondErr -v -count=1
```

**Commit:** `feat(marketplace-api): add gift card domain error codes`

---

## Task 4: Admin Gift Card Handler

**Files to create:**
- `services/marketplace-api/internal/handlers/admin/gift_cards.go`
- `services/marketplace-api/internal/handlers/admin/gift_cards_dto.go`

### Steps

- [ ] **4.1** Create `services/marketplace-api/internal/handlers/admin/gift_cards_dto.go`:

```go
package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
)

// IssueGiftCardRequest is the wire DTO for POST /gift-cards.
type IssueGiftCardRequest struct {
	InitialBalance decimal.Decimal `json:"initial_balance" binding:"required"`
	CurrencyCode   string          `json:"currency_code"   binding:"required,len=3"`
	SenderName     *string         `json:"sender_name"`
	SenderEmail    *string         `json:"sender_email"`
	RecipientName  *string         `json:"recipient_name"`
	RecipientEmail *string         `json:"recipient_email"`
	Message        *string         `json:"message"`
	ExpiresAt      *time.Time      `json:"expires_at"`
}

// AdminGiftCardResponse is the wire DTO for gift card list/detail.
type AdminGiftCardResponse struct {
	ID             string          `json:"id"`
	Code           string          `json:"code"`
	CodeDisplay    string          `json:"code_display"`
	InitialBalance decimal.Decimal `json:"initial_balance"`
	CurrentBalance decimal.Decimal `json:"current_balance"`
	CurrencyCode   string          `json:"currency_code"`
	Status         string          `json:"status"`
	SenderName     *string         `json:"sender_name,omitempty"`
	SenderEmail    *string         `json:"sender_email,omitempty"`
	RecipientName  *string         `json:"recipient_name,omitempty"`
	RecipientEmail *string         `json:"recipient_email,omitempty"`
	Message        *string         `json:"message,omitempty"`
	PurchasedAt    *time.Time      `json:"purchased_at,omitempty"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// AdminGiftCardTxnResponse is the wire DTO for a transaction ledger row.
type AdminGiftCardTxnResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Amount       decimal.Decimal `json:"amount"`
	BalanceAfter decimal.Decimal `json:"balance_after"`
	OrderID      *string         `json:"order_id,omitempty"`
	Note         *string         `json:"note,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// AdminGiftCardDetailResponse includes the card and its transaction ledger.
type AdminGiftCardDetailResponse struct {
	AdminGiftCardResponse
	Transactions []AdminGiftCardTxnResponse `json:"transactions"`
}

func toAdminGiftCardResponse(gc *giftcard.GiftCard) AdminGiftCardResponse {
	return AdminGiftCardResponse{
		ID:             gc.ID.String(),
		Code:           gc.Code,
		CodeDisplay:    giftcard.FormatCodeDisplay(gc.Code),
		InitialBalance: gc.InitialBalance,
		CurrentBalance: gc.CurrentBalance,
		CurrencyCode:   gc.CurrencyCode,
		Status:         string(gc.Status),
		SenderName:     gc.SenderName,
		SenderEmail:    gc.SenderEmail,
		RecipientName:  gc.RecipientName,
		RecipientEmail: gc.RecipientEmail,
		Message:        gc.Message,
		PurchasedAt:    gc.PurchasedAt,
		ExpiresAt:      gc.ExpiresAt,
		CreatedAt:      gc.CreatedAt,
	}
}

func toAdminGiftCardTxnResponse(txn *giftcard.Transaction) AdminGiftCardTxnResponse {
	resp := AdminGiftCardTxnResponse{
		ID:           txn.ID.String(),
		Type:         string(txn.Type),
		Amount:       txn.Amount,
		BalanceAfter: txn.BalanceAfter,
		Note:         txn.Note,
		CreatedAt:    txn.CreatedAt,
	}
	if txn.OrderID != nil {
		s := txn.OrderID.String()
		resp.OrderID = &s
	}
	return resp
}
```

- [ ] **4.2** Create `services/marketplace-api/internal/handlers/admin/gift_cards.go`:

```go
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// GiftCardHandler handles admin gift card endpoints.
type GiftCardHandler struct {
	svc    *giftcard.Service
	logger *slog.Logger
}

// NewGiftCardHandler constructs a GiftCardHandler.
func NewGiftCardHandler(svc *giftcard.Service, logger *slog.Logger) *GiftCardHandler {
	return &GiftCardHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/gift-cards.
func (h *GiftCardHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var status *giftcard.GiftCardStatus
	if s := c.Query("status"); s != "" {
		st := giftcard.GiftCardStatus(s)
		status = &st
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	cards, total, err := h.svc.ListByStore(c.Request.Context(), storeID, tenantID, status, page, pageSize)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]AdminGiftCardResponse, 0, len(cards))
	for i := range cards {
		out = append(out, toAdminGiftCardResponse(&cards[i]))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// Issue handles POST /admin/stores/:storeId/gift-cards.
func (h *GiftCardHandler) Issue(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req IssueGiftCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	gc, err := h.svc.Issue(c.Request.Context(), giftcard.IssueInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		InitialBalance: req.InitialBalance,
		CurrencyCode:   req.CurrencyCode,
		SenderName:     req.SenderName,
		SenderEmail:    req.SenderEmail,
		RecipientName:  req.RecipientName,
		RecipientEmail: req.RecipientEmail,
		Message:        req.Message,
		ExpiresAt:      req.ExpiresAt,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toAdminGiftCardResponse(gc)})
}

// Get handles GET /admin/stores/:storeId/gift-cards/:id.
func (h *GiftCardHandler) Get(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	gc, txns, err := h.svc.GetByID(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	txnResponses := make([]AdminGiftCardTxnResponse, 0, len(txns))
	for i := range txns {
		txnResponses = append(txnResponses, toAdminGiftCardTxnResponse(&txns[i]))
	}

	resp := AdminGiftCardDetailResponse{
		AdminGiftCardResponse: toAdminGiftCardResponse(gc),
		Transactions:          txnResponses,
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}
```

**Commit:** `feat(marketplace-api): add admin gift card handler with list, issue, detail endpoints`

---

## Task 5: Storefront Check-Balance Handler

**File to create:** `services/marketplace-api/internal/handlers/storefront/gift_cards.go`

- [ ] **5.1** Create `services/marketplace-api/internal/handlers/storefront/gift_cards.go`:

```go
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CheckBalanceRequest is the wire DTO for POST /gift-cards/check-balance.
type CheckBalanceRequest struct {
	Code string `json:"code" binding:"required"`
}

// GiftCardStorefrontHandler handles storefront gift card endpoints.
type GiftCardStorefrontHandler struct {
	svc    *giftcard.Service
	logger *slog.Logger
}

// NewGiftCardStorefrontHandler constructs a GiftCardStorefrontHandler.
func NewGiftCardStorefrontHandler(svc *giftcard.Service, logger *slog.Logger) *GiftCardStorefrontHandler {
	return &GiftCardStorefrontHandler{svc: svc, logger: logger}
}

// CheckBalance handles POST /storefront/stores/:storeSlug/gift-cards/check-balance.
func (h *GiftCardStorefrontHandler) CheckBalance(c *gin.Context) {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}
	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation_failed", "message": "invalid store",
		})
		return
	}

	var req CheckBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation_failed", "message": err.Error(),
		})
		return
	}

	result, err := h.svc.CheckBalance(c.Request.Context(), storeID, req.Code)
	if err != nil {
		var ae *apperrors.Error
		if asErr, ok := err.(*apperrors.Error); ok {
			ae = asErr
		}
		if ae != nil {
			switch ae.Code {
			case apperrors.CodeGiftCardNotFound, apperrors.CodeNotFound:
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": "gift_card_not_found", "message": "gift card not found",
				})
				return
			case apperrors.CodeGiftCardExpired:
				c.AbortWithStatusJSON(http.StatusGone, gin.H{
					"error": "gift_card_expired", "message": "this gift card has expired",
				})
				return
			}
		}
		if h.logger != nil {
			h.logger.Error("gift card check balance error", "err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
```

**Note on rate limiting:** The spec requires per-IP sliding window rate limiting (10 req/min) on the check-balance endpoint. Reuse the M1 rate-limit middleware. If M1 has not yet been implemented, create a simple in-memory token bucket middleware.

- [ ] **5.2** If M1 rate-limit middleware does not exist, create `services/marketplace-api/internal/handlers/storefront/ratelimit.go`:

```go
package storefront

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter is a simple in-memory per-IP sliding window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string][]time.Time
	limit   int
	window  time.Duration
}

// NewRateLimiter creates a rate limiter. limit is max requests per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		clients: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

// Middleware returns a Gin middleware that rate-limits by client IP.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		rl.mu.Lock()
		// Prune expired entries.
		cutoff := now.Add(-rl.window)
		entries := rl.clients[ip]
		valid := entries[:0]
		for _, t := range entries {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= rl.limit {
			rl.clients[ip] = valid
			rl.mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests, please try again later",
			})
			return
		}

		rl.clients[ip] = append(valid, now)
		rl.mu.Unlock()
		c.Next()
	}
}
```

- [ ] **5.3** Run:

```bash
cd services/marketplace-api && go build ./...
```

**Commit:** `feat(marketplace-api): add storefront gift card check-balance handler with rate limiting`

---

## Task 6: Checkout Integration — `discount.Applier` Interface

**Files to create:**
- `services/marketplace-api/internal/discount/applier.go`
- `services/marketplace-api/internal/discount/giftcard_applier.go`

**File to edit:**
- `services/marketplace-api/internal/handlers/storefront/checkout_ext.go`

### Steps

- [ ] **6.1** Create `services/marketplace-api/internal/discount/applier.go`:

```go
// Package discount defines the Applier interface that coupon, gift card,
// and loyalty redemption each implement. checkout_ext.go iterates
// []discount.Applier in order during the checkout transaction.
package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Applier applies a discount or payment method offset within a checkout
// transaction. The tx is owned by the checkout handler. Returns the
// amount actually deducted (may be less than the order total if the
// source has limited funds, e.g., gift card balance).
type Applier interface {
	Apply(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, amount decimal.Decimal) (deducted decimal.Decimal, err error)
}
```

- [ ] **6.2** Create `services/marketplace-api/internal/discount/giftcard_applier.go`:

```go
package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
)

// GiftCardApplier implements Applier by debiting a gift card.
type GiftCardApplier struct {
	Svc      *giftcard.Service
	CardID   uuid.UUID
	TenantID uuid.UUID
}

// Apply debits min(amount, card balance) from the gift card. Returns the
// amount actually deducted.
func (a *GiftCardApplier) Apply(ctx context.Context, tx *gorm.DB, orderID uuid.UUID, amount decimal.Decimal) (decimal.Decimal, error) {
	_, err := a.Svc.Debit(tx, a.CardID, amount, orderID, a.TenantID)
	if err != nil {
		return decimal.Zero, err
	}
	return amount, nil
}
```

- [ ] **6.3** Edit `services/marketplace-api/internal/handlers/storefront/checkout_ext.go`:

Add the gift card checkout integration. The changes are:

**6.3.1** Add `gift_card_code` field to `CheckoutExtRequest`:

In the `CheckoutExtRequest` struct, add after the `DiscountTotal` field:
```go
	GiftCardCode *string `json:"gift_card_code"`
```

**6.3.2** Add `gift_card_applied` field to `CheckoutExtResponse`:

In the `CheckoutExtResponse` struct, add after `Total`:
```go
	GiftCardApplied decimal.Decimal `json:"gift_card_applied"`
```

**6.3.3** Add `giftCardSvc` dependency to `CheckoutExtHandler`:

```go
type CheckoutExtHandler struct {
	db         *gorm.DB
	orderSvc   *order.Service
	giftCardSvc *giftcard.Service  // nil-safe: no-ops when nil
	logger     *slog.Logger
}
```

Update `NewCheckoutExtHandler` to accept `*giftcard.Service`:
```go
func NewCheckoutExtHandler(db *gorm.DB, orderSvc *order.Service, giftCardSvc *giftcard.Service, logger *slog.Logger) *CheckoutExtHandler {
	return &CheckoutExtHandler{db: db, orderSvc: orderSvc, giftCardSvc: giftCardSvc, logger: logger}
}
```

**6.3.4** In the `Checkout` method, after Step 3 (Calculate tax) and before Step 4 (Create order), add gift card lookup and debit. The gift card debit happens INSIDE the order creation transaction. The recommended approach:

After computing `grandTotal` (around line 147), add:

```go
	// ── Step 3.5: Gift card lookup (before tx) ─────────────────────────
	var giftCardApplied decimal.Decimal
	var giftCardID *uuid.UUID
	if req.GiftCardCode != nil && *req.GiftCardCode != "" && h.giftCardSvc != nil {
		gcResult, err := h.giftCardSvc.CheckBalance(ctx, storeID, *req.GiftCardCode)
		if err != nil {
			h.respondErr(c, err)
			return
		}
		// Debit min(balance, grandTotal).
		debitAmount := grandTotal
		if gcResult.CurrentBalance.LessThan(grandTotal) {
			debitAmount = gcResult.CurrentBalance
		}
		if debitAmount.GreaterThan(decimal.Zero) {
			gcCard, err := h.giftCardSvc.GetByCode(ctx, storeID, *req.GiftCardCode)
			if err != nil {
				h.respondErr(c, err)
				return
			}
			giftCardID = &gcCard.ID
			giftCardApplied = debitAmount
			grandTotal = grandTotal.Sub(debitAmount)
		}
	}
```

Then, after the order is created successfully (after `result, err := h.orderSvc.Create(...)` and the `result.Reused` check), add the gift card debit inside a transaction:

```go
	// ── Step 4.5: Debit gift card (in tx) ──────────────────────────────
	if giftCardID != nil && giftCardApplied.GreaterThan(decimal.Zero) && h.giftCardSvc != nil {
		if err := h.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
			_, err := h.giftCardSvc.Debit(tx, *giftCardID, giftCardApplied, result.Order.ID, tenantID)
			return err
		}); err != nil {
			h.logWarn("checkout_ext: gift card debit failed",
				"order_id", result.Order.ID.String(), "err", err)
			h.respondErr(c, err)
			return
		}
	}
```

**6.3.5** Add `GiftCardApplied` to the response marshaling in both success paths.

**6.3.6** Add the import for `giftcard` package at the top of `checkout_ext.go`:
```go
	"github.com/mark8ly/marketplace-api/internal/giftcard"
```

**6.3.7** Add `CodeGiftCardExpired`, `CodeGiftCardNotFound`, `CodeInsufficientGiftCardBalance` to the `respondErr` method's switch case so they map to correct HTTP statuses.

- [ ] **6.4** Run build:

```bash
cd services/marketplace-api && go build ./...
```

**Commit:** `feat(marketplace-api): add discount.Applier interface and gift card checkout integration`

---

## Task 7: Wire Routes + main.go

**Files to edit:**
- `services/marketplace-api/internal/handlers/admin/routes.go` — add `GiftCardHandler` to `Deps` and register routes
- `services/marketplace-api/internal/handlers/storefront/routes.go` — add `GiftCardStorefrontHandler` to `Deps` and register routes
- `services/marketplace-api/internal/authz/marketing_roles.go` — create new file for marketing role constants
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire gift card dependencies

### Steps

- [ ] **7.1** Create `services/marketplace-api/internal/authz/marketing_roles.go`:

```go
package authz

// Marketing M2 — Gift Cards role policy.
// Same approach as orders_roles.go.

// GiftCardsViewRole gates GET /admin/gift-cards. Staff can view.
var GiftCardsViewRole = RoleStaff

// GiftCardsEditRole gates POST /admin/gift-cards (issue). Admin can issue.
var GiftCardsEditRole = RoleAdmin
```

- [ ] **7.2** Edit `services/marketplace-api/internal/handlers/admin/routes.go`:

Add to the `Deps` struct:
```go
	GiftCardHandler *GiftCardHandler
```

Add route registration inside `RegisterAdmin`, after the abandoned carts block (around line 249), inside the `storeRoute` group:

```go
		// Gift cards — Marketing M2.
		if deps.GiftCardHandler != nil {
			gc := storeRoute.Group("/gift-cards")
			{
				gc.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsViewRole),
					deps.GiftCardHandler.List)
				gc.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsEditRole),
					deps.GiftCardHandler.Issue)
				gc.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsViewRole),
					deps.GiftCardHandler.Get)
			}
		}
```

- [ ] **7.3** Edit `services/marketplace-api/internal/handlers/storefront/routes.go`:

Add to the `Deps` struct:
```go
	GiftCardHandler *GiftCardStorefrontHandler
	RateLimiter     *RateLimiter // shared rate limiter instance
```

Add route registration inside `RegisterStorefront`, inside the `group` block, after the order detail handler:

```go
		// Gift cards — Marketing M2.
		if deps.GiftCardHandler != nil {
			gcGroup := group.Group("/gift-cards")
			{
				var mws []gin.HandlerFunc
				if deps.RateLimiter != nil {
					mws = append(mws, deps.RateLimiter.Middleware())
				}
				gcGroup.POST("/check-balance", append(mws, deps.GiftCardHandler.CheckBalance)...)
			}
		}
```

- [ ] **7.4** Edit `services/marketplace-api/cmd/marketplace-api/main.go`:

**7.4.1** Add import:
```go
	"github.com/mark8ly/marketplace-api/internal/giftcard"
```

**7.4.2** In the admin wiring block (inside `if m == mode.Admin || m == mode.Both`), after the settings handler wiring, add:

```go
		// Gift cards — Marketing M2.
		giftCardRepo := giftcard.NewRepository()
		giftCardSvc := giftcard.NewService(conn, giftCardRepo, log)
		giftCardHandler := admin.NewGiftCardHandler(giftCardSvc, log)
```

Add to `adminDeps`:
```go
		GiftCardHandler: giftCardHandler,
```

**7.4.3** In the storefront wiring block (inside `if m == mode.Storefront || m == mode.Both`), add:

```go
		// Gift cards — Marketing M2.
		giftCardRepoSF := giftcard.NewRepository()
		giftCardSvcSF := giftcard.NewService(conn, giftCardRepoSF, log)
		giftCardSFHandler := storefront.NewGiftCardStorefrontHandler(giftCardSvcSF, log)
		gcRateLimiter := storefront.NewRateLimiter(10, time.Minute)
```

Update the `NewCheckoutExtHandler` call to pass `giftCardSvcSF`:
```go
		checkoutExtHandler := storefront.NewCheckoutExtHandler(conn, orderSvcSF, giftCardSvcSF, log)
```

Add to `storefrontDeps`:
```go
		GiftCardHandler: giftCardSFHandler,
		RateLimiter:     gcRateLimiter,
```

- [ ] **7.5** Build and verify:

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/...
```

**Commit:** `feat(marketplace-api): wire gift card routes and dependencies in admin, storefront, and main.go`

---

## Task 8: Admin UI — API Client, List Page, Issue Page, Detail Page

### 8.1 API Client

**File to edit:** `apps/admin/lib/api/marketplace-api.ts`

- [ ] **8.1.1** Add gift card types and API functions to `apps/admin/lib/api/marketplace-api.ts`. Append at the end of the file:

```typescript
// ─────────────────────────────────────────────────────────────────────────
// Marketing M2: Gift Cards
// ─────────────────────────────────────────────────────────────────────────

export interface AdminGiftCard {
  id: string;
  code: string;
  code_display: string;
  initial_balance: string;
  current_balance: string;
  currency_code: string;
  status: "active" | "disabled" | "depleted";
  sender_name?: string;
  sender_email?: string;
  recipient_name?: string;
  recipient_email?: string;
  message?: string;
  purchased_at?: string;
  expires_at?: string;
  created_at: string;
}

export interface AdminGiftCardTransaction {
  id: string;
  type: "purchase" | "redeem" | "refund" | "adjustment";
  amount: string;
  balance_after: string;
  order_id?: string;
  note?: string;
  created_at: string;
}

export interface AdminGiftCardDetail extends AdminGiftCard {
  transactions: AdminGiftCardTransaction[];
}

export interface ListGiftCardsQuery {
  status?: "active" | "disabled" | "depleted";
  page?: number;
  pageSize?: number;
}

export interface ListGiftCardsResponse {
  data: AdminGiftCard[];
  meta: ListProductsMeta;
}

export interface IssueGiftCardInput {
  initial_balance: string;
  currency_code: string;
  sender_name?: string;
  sender_email?: string;
  recipient_name?: string;
  recipient_email?: string;
  message?: string;
  expires_at?: string;
}

export async function listGiftCards(
  storeId: string,
  query: ListGiftCardsQuery,
  session: SessionHeaders,
): Promise<ListGiftCardsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/gift-cards${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      Accept: "application/json",
    },
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listGiftCards ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as ListGiftCardsResponse;
}

export async function getGiftCard(
  storeId: string,
  giftCardId: string,
  session: SessionHeaders,
): Promise<{ data: AdminGiftCardDetail } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/gift-cards/${giftCardId}`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      Accept: "application/json",
    },
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: getGiftCard ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as { data: AdminGiftCardDetail };
}

export async function issueGiftCard(
  storeId: string,
  input: IssueGiftCardInput,
  session: SessionHeaders,
): Promise<{ data: AdminGiftCard }> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/gift-cards`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(input),
  });

  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: issueGiftCard ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as { data: AdminGiftCard };
}
```

### 8.2 List Page

**Files to create:**
- `apps/admin/app/marketing/gift-cards/page.tsx`
- `apps/admin/components/marketing/gift-cards/GiftCardsList.tsx`
- `apps/admin/components/marketing/gift-cards/GiftCardsListEmpty.tsx`

- [ ] **8.2.1** Create `apps/admin/app/marketing/gift-cards/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listGiftCards, type ListGiftCardsQuery } from "@/lib/api/marketplace-api";
import { GiftCardsList } from "@/components/marketing/gift-cards/GiftCardsList";
import { GiftCardsListEmpty } from "@/components/marketing/gift-cards/GiftCardsListEmpty";
import Link from "next/link";

interface GiftCardsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function GiftCardsPage({ searchParams }: GiftCardsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  const params = await searchParams;

  const canIssue = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <h1 className="font-serif text-2xl text-ink-900">Gift Cards</h1>
          <GiftCardsListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const query: ListGiftCardsQuery = {
    status: (params.status as ListGiftCardsQuery["status"]) ?? undefined,
    page: params.page ? Number(params.page) : 1,
    pageSize: params.page_size ? Number(params.page_size) : 20,
  };

  const response = await listGiftCards(currentStore.id, query, { userId, tenantId });
  const giftCards = response?.data ?? [];

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <div className="flex items-center justify-between">
          <h1 className="font-serif text-2xl text-ink-900">Gift Cards</h1>
          {canIssue && (
            <Link
              href="/marketing/gift-cards/new"
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition-colors hover:bg-moss-700"
            >
              Issue Gift Card
            </Link>
          )}
        </div>

        {giftCards.length === 0 ? (
          <GiftCardsListEmpty variant="no-gift-cards" canIssue={canIssue} />
        ) : (
          <GiftCardsList
            giftCards={giftCards}
            meta={response?.meta}
            currentStatus={query.status}
          />
        )}
      </main>
    </AdminShell>
  );
}
```

- [ ] **8.2.2** Create `apps/admin/components/marketing/gift-cards/GiftCardsList.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import type { AdminGiftCard, ListProductsMeta } from "@/lib/api/marketplace-api";

interface GiftCardsListProps {
  giftCards: AdminGiftCard[];
  meta?: ListProductsMeta;
  currentStatus?: string;
}

function statusBadge(status: string) {
  const base = "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium";
  switch (status) {
    case "active":
      return <span className={`${base} bg-moss-700/10 text-moss-700`}>Active</span>;
    case "depleted":
      return <span className={`${base} bg-ink-900/10 text-ink-900`}>Depleted</span>;
    case "disabled":
      return <span className={`${base} bg-signal/10 text-signal`}>Disabled</span>;
    default:
      return <span className={`${base} bg-ink-900/10 text-ink-900`}>{status}</span>;
  }
}

function formatCurrency(amount: string, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(Number(amount));
  } catch {
    return `${currency} ${amount}`;
  }
}

export function GiftCardsList({ giftCards, meta, currentStatus }: GiftCardsListProps) {
  const router = useRouter();
  const searchParams = useSearchParams();

  function setStatusFilter(status: string | undefined) {
    const params = new URLSearchParams(searchParams.toString());
    if (status) {
      params.set("status", status);
    } else {
      params.delete("status");
    }
    params.set("page", "1");
    router.push(`/marketing/gift-cards?${params.toString()}`);
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Status filter tabs */}
      <div className="flex gap-3 border-b border-ink-900/10 pb-2">
        {[
          { label: "All", value: undefined },
          { label: "Active", value: "active" },
          { label: "Depleted", value: "depleted" },
          { label: "Disabled", value: "disabled" },
        ].map((tab) => (
          <button
            key={tab.label}
            onClick={() => setStatusFilter(tab.value)}
            className={`text-sm font-medium transition-colors ${
              currentStatus === tab.value
                ? "text-ink-900 border-b-2 border-ink-900 -mb-[9px] pb-2"
                : "text-ink-900/50 hover:text-ink-900"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-ink-900/10 text-left text-xs font-medium uppercase tracking-wider text-ink-900/50">
              <th className="pb-3 pr-4">Code</th>
              <th className="pb-3 pr-4">Balance</th>
              <th className="pb-3 pr-4">Initial</th>
              <th className="pb-3 pr-4">Status</th>
              <th className="pb-3 pr-4">Recipient</th>
              <th className="pb-3">Created</th>
            </tr>
          </thead>
          <tbody>
            {giftCards.map((gc) => (
              <tr key={gc.id} className="border-b border-ink-900/5 hover:bg-paper-200/50">
                <td className="py-3 pr-4">
                  <Link
                    href={`/marketing/gift-cards/${gc.id}`}
                    className="font-mono text-sm text-moss-700 hover:underline"
                  >
                    {gc.code_display}
                  </Link>
                </td>
                <td className="py-3 pr-4 font-serif tabular-nums">
                  {formatCurrency(gc.current_balance, gc.currency_code)}
                </td>
                <td className="py-3 pr-4 text-ink-900/50 tabular-nums">
                  {formatCurrency(gc.initial_balance, gc.currency_code)}
                </td>
                <td className="py-3 pr-4">{statusBadge(gc.status)}</td>
                <td className="py-3 pr-4 text-ink-900/70">
                  {gc.recipient_name ?? gc.recipient_email ?? "—"}
                </td>
                <td className="py-3 text-ink-900/50">
                  {new Date(gc.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {meta && meta.total_pages > 1 && (
        <div className="flex items-center justify-between text-sm text-ink-900/50">
          <span>
            {meta.total} gift card{meta.total !== 1 ? "s" : ""}
          </span>
          <div className="flex gap-2">
            {meta.page > 1 && (
              <Link
                href={`/marketing/gift-cards?page=${meta.page - 1}${currentStatus ? `&status=${currentStatus}` : ""}`}
                className="text-moss-700 hover:underline"
              >
                Previous
              </Link>
            )}
            <span>
              Page {meta.page} of {meta.total_pages}
            </span>
            {meta.page < meta.total_pages && (
              <Link
                href={`/marketing/gift-cards?page=${meta.page + 1}${currentStatus ? `&status=${currentStatus}` : ""}`}
                className="text-moss-700 hover:underline"
              >
                Next
              </Link>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **8.2.3** Create `apps/admin/components/marketing/gift-cards/GiftCardsListEmpty.tsx`:

```tsx
import Link from "next/link";

interface GiftCardsListEmptyProps {
  variant: "no-store" | "no-gift-cards";
  canIssue?: boolean;
}

export function GiftCardsListEmpty({ variant, canIssue }: GiftCardsListEmptyProps) {
  if (variant === "no-store") {
    return (
      <div className="flex flex-col items-center gap-3 py-16 text-center">
        <p className="text-ink-900/50">Select a store to manage gift cards.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center gap-4 py-16 text-center">
      <p className="text-lg text-ink-900/70">No gift cards issued yet</p>
      <p className="max-w-md text-sm text-ink-900/50">
        Gift cards let your customers give store credit as gifts. Issue your
        first gift card to get started.
      </p>
      {canIssue && (
        <Link
          href="/marketing/gift-cards/new"
          className="mt-2 inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition-colors hover:bg-moss-700"
        >
          Issue Gift Card
        </Link>
      )}
    </div>
  );
}
```

### 8.3 Issue Page

**File to create:** `apps/admin/app/marketing/gift-cards/new/page.tsx`

- [ ] **8.3.1** Create `apps/admin/app/marketing/gift-cards/new/page.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { AdminShell } from "@/components/shell/AdminShell";

export default function IssueGiftCardPage() {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);

    const form = new FormData(e.currentTarget);
    const body = {
      initial_balance: form.get("initial_balance") as string,
      currency_code: form.get("currency_code") as string,
      sender_name: (form.get("sender_name") as string) || undefined,
      sender_email: (form.get("sender_email") as string) || undefined,
      recipient_name: (form.get("recipient_name") as string) || undefined,
      recipient_email: (form.get("recipient_email") as string) || undefined,
      message: (form.get("message") as string) || undefined,
      expires_at: (form.get("expires_at") as string) || undefined,
    };

    try {
      const res = await fetch("/api/marketing/gift-cards", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ message: "Unknown error" }));
        setError(err.message ?? "Failed to issue gift card");
        setSubmitting(false);
        return;
      }

      const { data } = await res.json();
      router.push(`/marketing/gift-cards/${data.id}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setSubmitting(false);
    }
  }

  return (
    <main className="flex flex-col gap-6 px-8 py-6">
      <h1 className="font-serif text-2xl text-ink-900">Issue Gift Card</h1>

      <form onSubmit={handleSubmit} className="flex max-w-xl flex-col gap-5">
        {error && (
          <div className="rounded-md border border-signal/20 bg-signal/5 px-4 py-3 text-sm text-signal">
            {error}
          </div>
        )}

        {/* Amount */}
        <div className="flex flex-col gap-1.5">
          <label htmlFor="initial_balance" className="text-sm font-medium text-ink-900">
            Amount *
          </label>
          <input
            id="initial_balance"
            name="initial_balance"
            type="number"
            step="0.01"
            min="0.01"
            required
            placeholder="50.00"
            className="rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>

        {/* Currency */}
        <div className="flex flex-col gap-1.5">
          <label htmlFor="currency_code" className="text-sm font-medium text-ink-900">
            Currency *
          </label>
          <input
            id="currency_code"
            name="currency_code"
            type="text"
            maxLength={3}
            required
            defaultValue="USD"
            className="w-24 rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm uppercase focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>

        <hr className="border-ink-900/10" />

        {/* Recipient */}
        <p className="text-xs font-medium uppercase tracking-wider text-ink-900/50">
          Recipient (optional)
        </p>
        <div className="grid grid-cols-2 gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="recipient_name" className="text-sm font-medium text-ink-900">
              Name
            </label>
            <input
              id="recipient_name"
              name="recipient_name"
              type="text"
              className="rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="recipient_email" className="text-sm font-medium text-ink-900">
              Email
            </label>
            <input
              id="recipient_email"
              name="recipient_email"
              type="email"
              className="rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
        </div>

        {/* Sender */}
        <p className="text-xs font-medium uppercase tracking-wider text-ink-900/50">
          Sender (optional)
        </p>
        <div className="grid grid-cols-2 gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="sender_name" className="text-sm font-medium text-ink-900">
              Name
            </label>
            <input
              id="sender_name"
              name="sender_name"
              type="text"
              className="rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="sender_email" className="text-sm font-medium text-ink-900">
              Email
            </label>
            <input
              id="sender_email"
              name="sender_email"
              type="email"
              className="rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
        </div>

        {/* Message */}
        <div className="flex flex-col gap-1.5">
          <label htmlFor="message" className="text-sm font-medium text-ink-900">
            Personal message (optional)
          </label>
          <textarea
            id="message"
            name="message"
            rows={3}
            maxLength={500}
            className="rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>

        {/* Expiry */}
        <div className="flex flex-col gap-1.5">
          <label htmlFor="expires_at" className="text-sm font-medium text-ink-900">
            Expiry date (optional)
          </label>
          <input
            id="expires_at"
            name="expires_at"
            type="date"
            className="w-48 rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>

        <div className="flex gap-3 pt-2">
          <button
            type="submit"
            disabled={submitting}
            className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-5 py-2.5 text-sm font-medium text-paper-200 transition-colors hover:bg-moss-700 disabled:opacity-50"
          >
            {submitting ? "Issuing..." : "Issue Gift Card"}
          </button>
          <button
            type="button"
            onClick={() => router.back()}
            className="rounded-md border border-ink-900/15 px-5 py-2.5 text-sm font-medium text-ink-900 transition-colors hover:bg-ink-900/5"
          >
            Cancel
          </button>
        </div>
      </form>
    </main>
  );
}
```

- [ ] **8.3.2** Create the API route proxy at `apps/admin/app/api/marketing/gift-cards/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { issueGiftCard } from "@/lib/api/marketplace-api";

export async function POST(request: NextRequest) {
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const body = await request.json();

  try {
    const result = await issueGiftCard(
      session.currentStore.id,
      body,
      { userId: session.userId, tenantId: session.tenantId },
    );
    return NextResponse.json(result);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return NextResponse.json({ message }, { status: 500 });
  }
}
```

### 8.4 Detail Page

**File to create:** `apps/admin/app/marketing/gift-cards/[id]/page.tsx`

- [ ] **8.4.1** Create `apps/admin/app/marketing/gift-cards/[id]/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getGiftCard } from "@/lib/api/marketplace-api";
import { notFound } from "next/navigation";
import Link from "next/link";

interface GiftCardDetailPageProps {
  params: Promise<{ id: string }>;
}

function formatCurrency(amount: string, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(Number(amount));
  } catch {
    return `${currency} ${amount}`;
  }
}

function txnTypeBadge(type: string) {
  const base = "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium";
  switch (type) {
    case "purchase":
      return <span className={`${base} bg-moss-700/10 text-moss-700`}>Purchase</span>;
    case "redeem":
      return <span className={`${base} bg-ink-900/10 text-ink-900`}>Redeem</span>;
    case "refund":
      return <span className={`${base} bg-warning/10 text-warning`}>Refund</span>;
    case "adjustment":
      return <span className={`${base} bg-ink-900/10 text-ink-900/70`}>Adjustment</span>;
    default:
      return <span className={`${base} bg-ink-900/10 text-ink-900`}>{type}</span>;
  }
}

export default async function GiftCardDetailPage({ params }: GiftCardDetailPageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) {
    notFound();
  }

  const response = await getGiftCard(currentStore.id, id, { userId, tenantId });
  if (!response) {
    notFound();
  }

  const gc = response.data;

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        {/* Header */}
        <div className="flex items-center gap-3">
          <Link
            href="/marketing/gift-cards"
            className="text-sm text-ink-900/50 hover:text-ink-900"
          >
            Gift Cards
          </Link>
          <span className="text-ink-900/30">/</span>
          <span className="font-mono text-sm text-ink-900">{gc.code_display}</span>
        </div>

        {/* Card summary */}
        <div className="grid grid-cols-3 gap-6">
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-ink-900/50">
              Current Balance
            </span>
            <span className="font-serif text-3xl tabular-nums text-ink-900">
              {formatCurrency(gc.current_balance, gc.currency_code)}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-ink-900/50">
              Initial Balance
            </span>
            <span className="font-serif text-xl tabular-nums text-ink-900/70">
              {formatCurrency(gc.initial_balance, gc.currency_code)}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-ink-900/50">
              Status
            </span>
            <span className="text-sm capitalize text-ink-900">{gc.status}</span>
          </div>
        </div>

        <hr className="border-ink-900/10" />

        {/* Details */}
        <div className="grid grid-cols-2 gap-x-8 gap-y-3 text-sm">
          {gc.recipient_name && (
            <div>
              <span className="text-ink-900/50">Recipient:</span>{" "}
              <span className="text-ink-900">{gc.recipient_name}</span>
            </div>
          )}
          {gc.recipient_email && (
            <div>
              <span className="text-ink-900/50">Recipient email:</span>{" "}
              <span className="text-ink-900">{gc.recipient_email}</span>
            </div>
          )}
          {gc.sender_name && (
            <div>
              <span className="text-ink-900/50">Sender:</span>{" "}
              <span className="text-ink-900">{gc.sender_name}</span>
            </div>
          )}
          {gc.expires_at && (
            <div>
              <span className="text-ink-900/50">Expires:</span>{" "}
              <span className="text-ink-900">
                {new Date(gc.expires_at).toLocaleDateString()}
              </span>
            </div>
          )}
          {gc.message && (
            <div className="col-span-2">
              <span className="text-ink-900/50">Message:</span>{" "}
              <span className="text-ink-900 italic">&ldquo;{gc.message}&rdquo;</span>
            </div>
          )}
        </div>

        <hr className="border-ink-900/10" />

        {/* Transaction ledger */}
        <h2 className="font-serif text-lg text-ink-900">Transaction History</h2>
        {gc.transactions.length === 0 ? (
          <p className="text-sm text-ink-900/50">No transactions yet.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-ink-900/10 text-left text-xs font-medium uppercase tracking-wider text-ink-900/50">
                <th className="pb-3 pr-4">Date</th>
                <th className="pb-3 pr-4">Type</th>
                <th className="pb-3 pr-4">Amount</th>
                <th className="pb-3 pr-4">Balance After</th>
                <th className="pb-3">Note</th>
              </tr>
            </thead>
            <tbody>
              {gc.transactions.map((txn) => (
                <tr key={txn.id} className="border-b border-ink-900/5">
                  <td className="py-3 pr-4 text-ink-900/50">
                    {new Date(txn.created_at).toLocaleString()}
                  </td>
                  <td className="py-3 pr-4">{txnTypeBadge(txn.type)}</td>
                  <td className={`py-3 pr-4 font-serif tabular-nums ${
                    Number(txn.amount) < 0 ? "text-signal" : "text-moss-700"
                  }`}>
                    {Number(txn.amount) > 0 ? "+" : ""}
                    {formatCurrency(txn.amount, gc.currency_code)}
                  </td>
                  <td className="py-3 pr-4 font-serif tabular-nums text-ink-900">
                    {formatCurrency(txn.balance_after, gc.currency_code)}
                  </td>
                  <td className="py-3 text-ink-900/50">{txn.note ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </main>
    </AdminShell>
  );
}
```

### 8.5 Update Sidebar

**File to edit:** `apps/admin/components/shell/AdminShell.tsx`

- [ ] **8.5.1** Update the marketing sidebar links. Change the Gift Cards href from `/dashboard` to `/marketing/gift-cards`:

Find the children array under the marketing key and replace the Gift Cards entry's href.

**Commit:** `feat(admin): add gift card list, issue, and detail pages with sidebar wiring`

---

## Task 9: Storefront — Gift Card Input in Checkout + Purchase Page

### 9.1 Checkout Gift Card Accordion

**File to edit:** `apps/storefront/app/checkout/page.tsx`

**File to edit:** `apps/storefront/lib/api/checkout-api.ts`

- [ ] **9.1.1** Add gift card API functions to `apps/storefront/lib/api/checkout-api.ts`:

Append to the file:

```typescript
// ─────────────────────────────────────────────────────────────────────────
// Marketing M2: Gift Card balance check
// ─────────────────────────────────────────────────────────────────────────

export interface GiftCardBalanceResult {
  code: string;
  current_balance: string;
  currency_code: string;
  status: string;
  expires_at?: string;
}

export async function checkGiftCardBalance(
  storeSlug: string,
  code: string,
): Promise<GiftCardBalanceResult | null> {
  const res = await fetch(`${storeUrl(storeSlug)}/gift-cards/check-balance`, {
    method: "POST",
    headers: commonHeaders(),
    body: JSON.stringify({ code }),
  });

  if (res.status === 404 || res.status === 410) {
    return null;
  }
  if (res.status === 429) {
    throw new Error("Too many requests. Please wait a moment and try again.");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.message ?? "Failed to check balance");
  }

  const json = await res.json();
  return json.data as GiftCardBalanceResult;
}
```

- [ ] **9.1.2** Add `gift_card_code` to `CheckoutBody` type in `checkout-api.ts`:

Find the `CheckoutBody` interface and add:
```typescript
  gift_card_code?: string;
```

- [ ] **9.1.3** Add `gift_card_applied` to the checkout response type in `checkout-api.ts`:

Find the checkout response type and add:
```typescript
  gift_card_applied?: string;
```

- [ ] **9.1.4** Edit `apps/storefront/app/checkout/page.tsx`:

Add a gift card input section in the checkout page. This should be a collapsible section below the order summary with:
- A text input for the gift card code
- A "Check Balance" button
- Display the balance result or error inline
- When a valid gift card is found, show the balance and a "Apply" button
- When applied, pass `gift_card_code` in the checkout body
- Show the deducted amount in the totals section

The implementation should add these state variables:
```typescript
const [giftCardCode, setGiftCardCode] = useState("");
const [giftCardBalance, setGiftCardBalance] = useState<GiftCardBalanceResult | null>(null);
const [giftCardError, setGiftCardError] = useState<string | null>(null);
const [giftCardApplied, setGiftCardApplied] = useState(false);
const [checkingBalance, setCheckingBalance] = useState(false);
```

Add a `handleCheckBalance` function:
```typescript
async function handleCheckBalance() {
  if (!giftCardCode.trim()) return;
  setCheckingBalance(true);
  setGiftCardError(null);
  try {
    const result = await checkGiftCardBalance(storeSlug, giftCardCode.trim());
    if (!result) {
      setGiftCardError("Gift card not found or expired");
      setGiftCardBalance(null);
    } else {
      setGiftCardBalance(result);
    }
  } catch (err: unknown) {
    setGiftCardError(err instanceof Error ? err.message : "Failed to check balance");
    setGiftCardBalance(null);
  } finally {
    setCheckingBalance(false);
  }
}
```

In the checkout form JSX, add after the order summary (before the submit button area), a section like:

```tsx
{/* Gift card input */}
<div className="border-t border-ink-900/10 pt-4">
  <button
    type="button"
    onClick={() => setShowGiftCard((v) => !v)}
    className="text-sm text-moss-700 hover:underline"
  >
    Have a gift card?
  </button>
  {showGiftCard && (
    <div className="mt-3 flex flex-col gap-2">
      <div className="flex gap-2">
        <input
          type="text"
          value={giftCardCode}
          onChange={(e) => setGiftCardCode(e.target.value)}
          placeholder="Enter gift card code"
          className="flex-1 rounded-md border border-ink-900/15 bg-white px-3 py-2 text-sm"
        />
        <button
          type="button"
          onClick={handleCheckBalance}
          disabled={checkingBalance || !giftCardCode.trim()}
          className="rounded-md bg-ink-900 px-3 py-2 text-sm text-paper-200 disabled:opacity-50"
        >
          {checkingBalance ? "Checking..." : "Check"}
        </button>
      </div>
      {giftCardError && (
        <p className="text-sm text-signal">{giftCardError}</p>
      )}
      {giftCardBalance && !giftCardApplied && (
        <div className="flex items-center justify-between rounded-md bg-moss-700/5 px-3 py-2">
          <span className="text-sm">
            Balance: {formatPrice(Number(giftCardBalance.current_balance), giftCardBalance.currency_code)}
          </span>
          <button
            type="button"
            onClick={() => setGiftCardApplied(true)}
            className="text-sm font-medium text-moss-700 hover:underline"
          >
            Apply
          </button>
        </div>
      )}
      {giftCardApplied && giftCardBalance && (
        <div className="flex items-center justify-between rounded-md bg-moss-700/10 px-3 py-2">
          <span className="text-sm text-moss-700">
            Gift card applied: up to {formatPrice(Number(giftCardBalance.current_balance), giftCardBalance.currency_code)}
          </span>
          <button
            type="button"
            onClick={() => { setGiftCardApplied(false); setGiftCardBalance(null); setGiftCardCode(""); }}
            className="text-sm text-ink-900/50 hover:text-ink-900"
          >
            Remove
          </button>
        </div>
      )}
    </div>
  )}
</div>
```

In the totals section, add a gift card line when applied:
```tsx
{giftCardApplied && giftCardBalance && (
  <div className="flex justify-between text-sm">
    <span className="text-ink-900/70">Gift card</span>
    <span className="text-moss-700">
      -{formatPrice(
        Math.min(Number(giftCardBalance.current_balance), grandTotalBeforeGC),
        currencyCode
      )}
    </span>
  </div>
)}
```

In the `submitCheckout` call, pass `gift_card_code`:
```typescript
gift_card_code: giftCardApplied ? giftCardCode.trim() : undefined,
```

### 9.2 Gift Cards Purchase Page (placeholder)

**File to create:** `apps/storefront/app/gift-cards/page.tsx`

- [ ] **9.2.1** Create `apps/storefront/app/gift-cards/page.tsx`:

```tsx
import { StorefrontNav } from "@/components/StorefrontNav";

export default function GiftCardsPage() {
  return (
    <>
      <StorefrontNav />
      <main className="mx-auto max-w-3xl px-6 py-12">
        <h1 className="font-serif text-3xl text-ink-900">Gift Cards</h1>
        <p className="mt-4 text-ink-900/60">
          Gift cards are coming soon. Check back later to purchase a gift card for someone special.
        </p>
      </main>
    </>
  );
}
```

**Note:** The full gift card purchase page (with amount selector, recipient form, and preview) is a follow-up. This placeholder ensures the route exists.

**Commit:** `feat(storefront): add gift card balance check in checkout and gift-cards placeholder page`

---

## Task 10: Build Verification + Final Commit

- [ ] **10.1** Verify Go build:

```bash
cd services/marketplace-api && go build ./...
```

- [ ] **10.2** Run Go tests:

```bash
cd services/marketplace-api && go test ./internal/giftcard/... -v -count=1
cd services/marketplace-api && go test ./pkg/apperrors/... -v -count=1
cd services/marketplace-api && go test ./internal/handlers/admin/... -v -count=1
```

- [ ] **10.3** Verify Next.js admin builds:

```bash
cd apps/admin && npx next build
```

- [ ] **10.4** Verify Next.js storefront builds:

```bash
cd apps/storefront && npx next build
```

- [ ] **10.5** Fix any build errors found in 10.1-10.4.

- [ ] **10.6** Final commit (if not already committed per-task):

```
feat(marketing): complete M2 gift cards — issuance, balance check, checkout integration
```

---

## File Inventory

### New Files (Go backend)

| File | Purpose |
|------|---------|
| `services/marketplace-api/migrations/000010_gift_cards.up.sql` | Schema migration |
| `services/marketplace-api/migrations/000010_gift_cards.down.sql` | Rollback migration |
| `services/marketplace-api/internal/giftcard/models.go` | GORM models |
| `services/marketplace-api/internal/giftcard/repository.go` | Repository with atomic DebitInTx |
| `services/marketplace-api/internal/giftcard/service.go` | Business logic + crypto/rand code gen |
| `services/marketplace-api/internal/giftcard/service_test.go` | Unit tests |
| `services/marketplace-api/internal/discount/applier.go` | Applier interface |
| `services/marketplace-api/internal/discount/giftcard_applier.go` | Gift card Applier impl |
| `services/marketplace-api/internal/handlers/admin/gift_cards.go` | Admin handler |
| `services/marketplace-api/internal/handlers/admin/gift_cards_dto.go` | Admin DTOs |
| `services/marketplace-api/internal/handlers/storefront/gift_cards.go` | Storefront handler |
| `services/marketplace-api/internal/handlers/storefront/ratelimit.go` | Rate limit middleware |
| `services/marketplace-api/internal/authz/marketing_roles.go` | Role constants |

### New Files (Frontend)

| File | Purpose |
|------|---------|
| `apps/admin/app/marketing/gift-cards/page.tsx` | List page |
| `apps/admin/app/marketing/gift-cards/new/page.tsx` | Issue form |
| `apps/admin/app/marketing/gift-cards/[id]/page.tsx` | Detail + transaction ledger |
| `apps/admin/app/api/marketing/gift-cards/route.ts` | API proxy route |
| `apps/admin/components/marketing/gift-cards/GiftCardsList.tsx` | List component |
| `apps/admin/components/marketing/gift-cards/GiftCardsListEmpty.tsx` | Empty state |
| `apps/storefront/app/gift-cards/page.tsx` | Purchase placeholder |

### Edited Files

| File | Change |
|------|--------|
| `services/marketplace-api/migrations.go` | Bump `ExpectedSchemaVersion` |
| `services/marketplace-api/pkg/apperrors/errors.go` | Add 3 gift card error codes + sentinels |
| `services/marketplace-api/internal/handlers/admin/errors.go` | Add codes to `codeStatus` map |
| `services/marketplace-api/internal/handlers/admin/routes.go` | Add `GiftCardHandler` to `Deps` + routes |
| `services/marketplace-api/internal/handlers/storefront/routes.go` | Add `GiftCardStorefrontHandler` + `RateLimiter` to `Deps` + routes |
| `services/marketplace-api/internal/handlers/storefront/checkout_ext.go` | Add `gift_card_code` to request/response, wire debit |
| `services/marketplace-api/cmd/marketplace-api/main.go` | Wire gift card dependencies |
| `apps/admin/lib/api/marketplace-api.ts` | Add gift card types + API functions |
| `apps/admin/components/shell/AdminShell.tsx` | Update Gift Cards sidebar href |
| `apps/storefront/lib/api/checkout-api.ts` | Add `checkGiftCardBalance` + checkout body fields |
| `apps/storefront/app/checkout/page.tsx` | Add gift card input section |

---

## Commit Sequence

1. `feat(marketplace-api): add migration 000010 for gift cards schema`
2. `feat(marketplace-api): add giftcard package with models, repository, service, and code generation`
3. `feat(marketplace-api): add gift card domain error codes`
4. `feat(marketplace-api): add admin gift card handler with list, issue, detail endpoints`
5. `feat(marketplace-api): add storefront gift card check-balance handler with rate limiting`
6. `feat(marketplace-api): add discount.Applier interface and gift card checkout integration`
7. `feat(marketplace-api): wire gift card routes and dependencies in admin, storefront, and main.go`
8. `feat(admin): add gift card list, issue, and detail pages with sidebar wiring`
9. `feat(storefront): add gift card balance check in checkout and gift-cards placeholder page`
10. `feat(marketing): complete M2 gift cards — issuance, balance check, checkout integration`

---

## Key Design Decisions

1. **Atomic debit via SELECT FOR UPDATE:** The repository locks the gift card row before checking balance, preventing race conditions. The CHECK constraint on `current_balance >= 0` is a database-level safety net.

2. **Code generation:** 10 bytes from `crypto/rand` encoded as 16-char base32. Minimum 80 bits of entropy (exceeds the 128-bit spec requirement when accounting for the full 10-byte input). Stored without dashes, displayed with dashes.

3. **Checkout integration:** Gift card debit happens in a separate transaction after order creation (not inside the order tx). This is simpler and avoids making the order creation tx larger. The trade-off is that a gift card debit could succeed but payment creation could fail — but since the response includes `gift_card_applied`, the storefront can retry payment without re-debiting. For a future improvement, wrap both in a single tx.

4. **Rate limiting:** In-memory sliding window per IP. No Redis dependency. Adequate for single-instance or low-traffic deployments. If horizontal scaling is needed, swap to Redis-backed rate limiting.

5. **Discount Applier interface:** Created for future M1 (coupons) and M3 (loyalty) integration. Gift card implements it but checkout_ext.go currently calls the service directly for simplicity. The interface exists as the contract for when all three features need to compose.
