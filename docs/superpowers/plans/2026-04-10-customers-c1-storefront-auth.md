# Customers C1 — Storefront Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship customer login/register on storefront via auth-bff + GIP mp-customer pool, session middleware (OptionalCustomerAuth + RequireCustomerAuth), profile auto-creation on first login, and /account shell with dashboard page.

**Architecture:** New `internal/customer/` package (models, repository, service for profile auto-creation). New middleware in storefront handlers. Migration 000013 for customer_profiles + customer_addresses + orders index for store_id+customer_email. Storefront /account pages behind RequireCustomerAuth.

**Tech Stack:** Go 1.26, Gin, GORM. Next.js 16, React 19, Tailwind.

**Spec reference:** `docs/superpowers/specs/2026-04-10-customers-reviews-design.md` -- sections 3.1, 4.1-4.3, 5.2, 7.1-7.2, 8.2, 9.1-9.2, 9.7, 10.5-10.6, 11 (C1).

**Prerequisite:** Payments P1 on main (migrations through 000008). The spec says 000013 but current latest is 000008 -- if migrations 000009-000012 have been added by the time you execute, adjust the migration number. If 000008 is still the latest, use **000009** instead of 000013.

---

## File structure produced by C1

```
services/marketplace-api/
├── migrations/
│   ├── 000013_customer_profiles.up.sql      # NEW — customer_profiles + customer_addresses + orders index
│   └── 000013_customer_profiles.down.sql    # NEW
├── internal/
│   └── customer/
│       ├── models.go                        # NEW — CustomerProfile + CustomerAddress GORM models + constants
│       ├── repository.go                    # NEW — Repository interface + GORM implementation
│       ├── service.go                       # NEW — EnsureProfile (upsert on first login)
│       └── service_test.go                  # NEW — unit tests for EnsureProfile
├── internal/handlers/storefront/
│   ├── middleware.go                        # MODIFY — add OptionalCustomerAuth + RequireCustomerAuth
│   ├── middleware_test.go                   # MODIFY or NEW — auth middleware unit tests
│   ├── customer_account.go                  # NEW — GET/PATCH profile, addresses CRUD
│   └── routes.go                           # MODIFY — add Deps fields + register /account routes
├── pkg/config/
│   └── config.go                           # MODIFY — add CustomerSessionSecret field
└── cmd/marketplace-api/main.go             # MODIFY — wire customer repo/service/handlers

apps/storefront/
├── .env.example                             # MODIFY — add AUTH_BFF_URL, MARKETPLACE_API_URL (already present)
├── lib/api/
│   └── customer-api.ts                      # NEW — storefront customer API client
├── lib/
│   └── auth.ts                              # NEW — auth helpers (login URL builder, cookie check)
├── components/
│   ├── StorefrontNav.tsx                    # MODIFY — add sign-in / account dropdown
│   ├── CustomerAuthProvider.tsx             # NEW — React context for customer auth state
│   └── AccountSidebar.tsx                   # NEW — account page sidebar navigation
├── app/account/
│   ├── layout.tsx                           # NEW — account layout with sidebar + auth gate
│   ├── page.tsx                             # NEW — dashboard
│   ├── orders/
│   │   └── page.tsx                         # NEW — order history
│   └── addresses/
│       └── page.tsx                         # NEW — saved addresses
```

---

## Task 1: Migration 000013 — customer_profiles + customer_addresses + orders index

**Files:** `services/marketplace-api/migrations/000013_customer_profiles.up.sql`, `services/marketplace-api/migrations/000013_customer_profiles.down.sql`

> **Migration number:** The spec uses 000013. Check what the latest migration number is in `services/marketplace-api/migrations/`. If the latest is still 000008, use 000009 instead of 000013. Replace all references to `000013` with the correct number.

- [ ] **Step 1: Create up migration**

Create `services/marketplace-api/migrations/000013_customer_profiles.up.sql`:

```sql
-- 000013_customer_profiles: Customer profiles, addresses, and orders performance index.
BEGIN;

-- Customer profiles — one row per (store, email). Auto-created on first login.
CREATE TABLE customer_profiles (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    gip_uid         VARCHAR(200),
    email           VARCHAR(300)  NOT NULL,
    first_name      VARCHAR(200),
    last_name       VARCHAR(200),
    phone           VARCHAR(40),
    avatar_url      TEXT,
    tags            TEXT[]        NOT NULL DEFAULT '{}',
    status          VARCHAR(20)   NOT NULL DEFAULT 'active',
    block_reason    VARCHAR(300),
    notes           TEXT,
    marketing_opt_in BOOLEAN      NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, email)
);
CREATE INDEX cp_store_status_idx ON customer_profiles (store_id, status);
CREATE INDEX cp_gip_uid_idx ON customer_profiles (gip_uid) WHERE gip_uid IS NOT NULL;
CREATE INDEX cp_tags_idx ON customer_profiles USING GIN (tags);

-- Customer saved addresses (for storefront /account/addresses).
CREATE TABLE customer_addresses (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    customer_id     UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    label           VARCHAR(100),
    is_default      BOOLEAN       NOT NULL DEFAULT false,
    name            VARCHAR(200)  NOT NULL,
    line1           VARCHAR(300)  NOT NULL,
    line2           VARCHAR(300),
    city            VARCHAR(200)  NOT NULL,
    region          VARCHAR(200),
    postal_code     VARCHAR(40),
    country_code    CHAR(2)       NOT NULL,
    phone           VARCHAR(40),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX ca_customer_idx ON customer_addresses (customer_id);

-- Performance index for customer stats aggregation (order_count, total_spent).
-- Used by admin customer list correlated subqueries per spec section 8.6.
CREATE INDEX orders_store_email_idx ON orders (store_id, customer_email);

COMMIT;
```

- [ ] **Step 2: Create down migration**

Create `services/marketplace-api/migrations/000013_customer_profiles.down.sql`:

```sql
BEGIN;
DROP INDEX IF EXISTS orders_store_email_idx;
DROP TABLE IF EXISTS customer_addresses;
DROP TABLE IF EXISTS customer_profiles;
COMMIT;
```

- [ ] **Step 3: Run migration and verify**

```bash
cd services/marketplace-api
DATABASE_URL="${DATABASE_URL}" go run ./cmd/migrate up
```

Verify tables exist:

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT table_name FROM information_schema.tables WHERE table_name IN ('customer_profiles','customer_addresses') ORDER BY 1;"
```

Expected output:
```
customer_addresses
customer_profiles
```

Verify orders index:

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT indexname FROM pg_indexes WHERE indexname = 'orders_store_email_idx';"
```

Expected: `orders_store_email_idx`

---

## Task 2: `internal/customer/` models + repository + service

**Files:** `services/marketplace-api/internal/customer/models.go`, `services/marketplace-api/internal/customer/repository.go`, `services/marketplace-api/internal/customer/service.go`, `services/marketplace-api/internal/customer/service_test.go`

### Step 1: Models

- [ ] **Step 1: Create `services/marketplace-api/internal/customer/models.go`**

```go
// Package customer provides the domain model, repository, and service
// for customer profiles and addresses. Profiles are auto-created on
// first authenticated storefront request (see Service.EnsureProfile).
package customer

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ProfileStatus constants match the CHECK on customer_profiles.status.
const (
	StatusActive  = "active"
	StatusBlocked = "blocked"
)

// CustomerProfile maps to the customer_profiles table.
type CustomerProfile struct {
	ID             uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	GipUID         *string        `gorm:"column:gip_uid;type:varchar(200)"`
	Email          string         `gorm:"column:email;type:varchar(300);not null"`
	FirstName      *string        `gorm:"column:first_name;type:varchar(200)"`
	LastName       *string        `gorm:"column:last_name;type:varchar(200)"`
	Phone          *string        `gorm:"column:phone;type:varchar(40)"`
	AvatarURL      *string        `gorm:"column:avatar_url;type:text"`
	Tags           pq.StringArray `gorm:"column:tags;type:text[];not null;default:'{}'"`
	Status         string         `gorm:"column:status;type:varchar(20);not null;default:active"`
	BlockReason    *string        `gorm:"column:block_reason;type:varchar(300)"`
	Notes          *string        `gorm:"column:notes;type:text"`
	MarketingOptIn bool           `gorm:"column:marketing_opt_in;type:boolean;not null;default:false"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomerProfile) TableName() string { return "customer_profiles" }

// CustomerAddress maps to the customer_addresses table.
type CustomerAddress struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	CustomerID  uuid.UUID `gorm:"column:customer_id;type:uuid;not null"`
	Label       *string   `gorm:"column:label;type:varchar(100)"`
	IsDefault   bool      `gorm:"column:is_default;type:boolean;not null;default:false"`
	Name        string    `gorm:"column:name;type:varchar(200);not null"`
	Line1       string    `gorm:"column:line1;type:varchar(300);not null"`
	Line2       *string   `gorm:"column:line2;type:varchar(300)"`
	City        string    `gorm:"column:city;type:varchar(200);not null"`
	Region      *string   `gorm:"column:region;type:varchar(200)"`
	PostalCode  *string   `gorm:"column:postal_code;type:varchar(40)"`
	CountryCode string    `gorm:"column:country_code;type:char(2);not null"`
	Phone       *string   `gorm:"column:phone;type:varchar(40)"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomerAddress) TableName() string { return "customer_addresses" }

// StorefrontProfileResponse is the customer-facing DTO. Excludes admin-only
// fields: notes, block_reason, status, tags (per spec section 9.7).
type StorefrontProfileResponse struct {
	ID             string  `json:"id"`
	Email          string  `json:"email"`
	FirstName      string  `json:"first_name,omitempty"`
	LastName       string  `json:"last_name,omitempty"`
	Phone          string  `json:"phone,omitempty"`
	AvatarURL      string  `json:"avatar_url,omitempty"`
	MarketingOptIn bool    `json:"marketing_opt_in"`
	CreatedAt      string  `json:"created_at"`
}

// AddressResponse is the DTO returned for customer addresses.
type AddressResponse struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	IsDefault   bool   `json:"is_default"`
	Name        string `json:"name"`
	Line1       string `json:"line1"`
	Line2       string `json:"line2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code"`
	Phone       string `json:"phone,omitempty"`
}

// UpdateProfileRequest is the storefront PATCH /account payload.
type UpdateProfileRequest struct {
	FirstName      *string `json:"first_name" binding:"omitempty,max=200"`
	LastName       *string `json:"last_name" binding:"omitempty,max=200"`
	Phone          *string `json:"phone" binding:"omitempty,max=40"`
	AvatarURL      *string `json:"avatar_url" binding:"omitempty,url,max=2048"`
	MarketingOptIn *bool   `json:"marketing_opt_in"`
}

// CreateAddressRequest is the storefront POST /account/addresses payload.
type CreateAddressRequest struct {
	Label       *string `json:"label" binding:"omitempty,max=100"`
	IsDefault   bool    `json:"is_default"`
	Name        string  `json:"name" binding:"required,max=200"`
	Line1       string  `json:"line1" binding:"required,max=300"`
	Line2       *string `json:"line2" binding:"omitempty,max=300"`
	City        string  `json:"city" binding:"required,max=200"`
	Region      *string `json:"region" binding:"omitempty,max=200"`
	PostalCode  *string `json:"postal_code" binding:"omitempty,max=40"`
	CountryCode string  `json:"country_code" binding:"required,len=2"`
	Phone       *string `json:"phone" binding:"omitempty,max=40"`
}

// UpdateAddressRequest is the storefront PATCH /account/addresses/:id payload.
type UpdateAddressRequest struct {
	Label       *string `json:"label" binding:"omitempty,max=100"`
	IsDefault   *bool   `json:"is_default"`
	Name        *string `json:"name" binding:"omitempty,max=200"`
	Line1       *string `json:"line1" binding:"omitempty,max=300"`
	Line2       *string `json:"line2" binding:"omitempty,max=300"`
	City        *string `json:"city" binding:"omitempty,max=200"`
	Region      *string `json:"region" binding:"omitempty,max=200"`
	PostalCode  *string `json:"postal_code" binding:"omitempty,max=40"`
	CountryCode *string `json:"country_code" binding:"omitempty,len=2"`
	Phone       *string `json:"phone" binding:"omitempty,max=40"`
}
```

### Step 2: Repository

- [ ] **Step 2: Create `services/marketplace-api/internal/customer/repository.go`**

```go
package customer

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when a profile or address does not exist.
var ErrNotFound = errors.New("customer: not found")

// Repository is the data-access interface for customer profiles and addresses.
type Repository interface {
	// UpsertProfile inserts a profile or updates gip_uid+updated_at on conflict(store_id, email).
	UpsertProfile(ctx context.Context, p *CustomerProfile) (*CustomerProfile, error)

	// GetProfileByGipUID returns the profile for (store_id, gip_uid). ErrNotFound on miss.
	GetProfileByGipUID(ctx context.Context, storeID uuid.UUID, gipUID string) (*CustomerProfile, error)

	// GetProfileByID returns a profile by primary key. ErrNotFound on miss.
	GetProfileByID(ctx context.Context, profileID uuid.UUID) (*CustomerProfile, error)

	// UpdateProfile patches non-nil fields. Returns updated profile.
	UpdateProfile(ctx context.Context, profileID uuid.UUID, updates map[string]any) (*CustomerProfile, error)

	// ListAddresses returns all addresses for a customer, ordered by is_default DESC, created_at ASC.
	ListAddresses(ctx context.Context, customerID uuid.UUID) ([]CustomerAddress, error)

	// CreateAddress inserts a new address. If is_default, clears other defaults first.
	CreateAddress(ctx context.Context, tx *gorm.DB, addr *CustomerAddress) error

	// GetAddress returns an address by ID scoped to customer. ErrNotFound on miss.
	GetAddress(ctx context.Context, addressID, customerID uuid.UUID) (*CustomerAddress, error)

	// UpdateAddress patches non-nil fields. Returns updated address.
	UpdateAddress(ctx context.Context, tx *gorm.DB, addressID uuid.UUID, updates map[string]any) (*CustomerAddress, error)

	// DeleteAddress removes an address by ID scoped to customer.
	DeleteAddress(ctx context.Context, addressID, customerID uuid.UUID) error

	// ClearDefaultAddresses sets is_default=false for all addresses of a customer (within tx).
	ClearDefaultAddresses(ctx context.Context, tx *gorm.DB, customerID uuid.UUID) error
}

type gormRepo struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by GORM.
func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) UpsertProfile(ctx context.Context, p *CustomerProfile) (*CustomerProfile, error) {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "store_id"}, {Name: "email"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"gip_uid", "first_name", "last_name", "updated_at",
			}),
		}).
		Create(p).Error
	if err != nil {
		return nil, fmt.Errorf("customer: upsert profile: %w", err)
	}
	return p, nil
}

func (r *gormRepo) GetProfileByGipUID(ctx context.Context, storeID uuid.UUID, gipUID string) (*CustomerProfile, error) {
	var p CustomerProfile
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND gip_uid = ?", storeID, gipUID).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: get by gip_uid: %w", err)
	}
	return &p, nil
}

func (r *gormRepo) GetProfileByID(ctx context.Context, profileID uuid.UUID) (*CustomerProfile, error) {
	var p CustomerProfile
	err := r.db.WithContext(ctx).
		Where("id = ?", profileID).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: get by id: %w", err)
	}
	return &p, nil
}

func (r *gormRepo) UpdateProfile(ctx context.Context, profileID uuid.UUID, updates map[string]any) (*CustomerProfile, error) {
	updates["updated_at"] = "now()"
	err := r.db.WithContext(ctx).
		Model(&CustomerProfile{}).
		Where("id = ?", profileID).
		Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("customer: update profile: %w", err)
	}
	return r.GetProfileByID(ctx, profileID)
}

func (r *gormRepo) ListAddresses(ctx context.Context, customerID uuid.UUID) ([]CustomerAddress, error) {
	var out []CustomerAddress
	err := r.db.WithContext(ctx).
		Where("customer_id = ?", customerID).
		Order("is_default DESC, created_at ASC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("customer: list addresses: %w", err)
	}
	if out == nil {
		out = []CustomerAddress{}
	}
	return out, nil
}

func (r *gormRepo) CreateAddress(ctx context.Context, tx *gorm.DB, addr *CustomerAddress) error {
	if err := tx.WithContext(ctx).Create(addr).Error; err != nil {
		return fmt.Errorf("customer: create address: %w", err)
	}
	return nil
}

func (r *gormRepo) GetAddress(ctx context.Context, addressID, customerID uuid.UUID) (*CustomerAddress, error) {
	var a CustomerAddress
	err := r.db.WithContext(ctx).
		Where("id = ? AND customer_id = ?", addressID, customerID).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: get address: %w", err)
	}
	return &a, nil
}

func (r *gormRepo) UpdateAddress(ctx context.Context, tx *gorm.DB, addressID uuid.UUID, updates map[string]any) (*CustomerAddress, error) {
	updates["updated_at"] = "now()"
	err := tx.WithContext(ctx).
		Model(&CustomerAddress{}).
		Where("id = ?", addressID).
		Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("customer: update address: %w", err)
	}
	var a CustomerAddress
	if err := tx.WithContext(ctx).Where("id = ?", addressID).First(&a).Error; err != nil {
		return nil, fmt.Errorf("customer: reload address: %w", err)
	}
	return &a, nil
}

func (r *gormRepo) DeleteAddress(ctx context.Context, addressID, customerID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND customer_id = ?", addressID, customerID).
		Delete(&CustomerAddress{})
	if result.Error != nil {
		return fmt.Errorf("customer: delete address: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepo) ClearDefaultAddresses(ctx context.Context, tx *gorm.DB, customerID uuid.UUID) error {
	err := tx.WithContext(ctx).
		Model(&CustomerAddress{}).
		Where("customer_id = ? AND is_default = true", customerID).
		Update("is_default", false).Error
	if err != nil {
		return fmt.Errorf("customer: clear defaults: %w", err)
	}
	return nil
}
```

### Step 3: Service (EnsureProfile)

- [ ] **Step 3: Create `services/marketplace-api/internal/customer/service.go`**

```go
package customer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EnsureProfileInput is the input for profile auto-creation.
type EnsureProfileInput struct {
	StoreID   uuid.UUID
	TenantID  uuid.UUID
	GipUID    string
	Email     string
	FirstName string
	LastName  string
}

// Service provides customer business logic.
type Service struct {
	repo   Repository
	db     *gorm.DB
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// EnsureProfile upserts a customer profile on first authenticated request.
// Uses INSERT ON CONFLICT (store_id, email) DO UPDATE SET gip_uid = EXCLUDED.gip_uid
// so concurrent first-logins are safe (spec section 8.2).
//
// Returns the existing or newly created profile.
func (s *Service) EnsureProfile(ctx context.Context, input EnsureProfileInput) (*CustomerProfile, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" {
		return nil, fmt.Errorf("customer: email is required for profile auto-creation")
	}

	profile := &CustomerProfile{
		TenantID:  input.TenantID,
		StoreID:   input.StoreID,
		GipUID:    &input.GipUID,
		Email:     email,
		FirstName: nilIfEmpty(input.FirstName),
		LastName:  nilIfEmpty(input.LastName),
		Status:    StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := s.repo.UpsertProfile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("customer: ensure profile: %w", err)
	}

	s.logger.Info("customer profile ensured",
		"store_id", input.StoreID,
		"email", email,
		"profile_id", result.ID,
	)
	return result, nil
}

func nilIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
```

### Step 4: Service unit tests

- [ ] **Step 4: Create `services/marketplace-api/internal/customer/service_test.go`**

```go
package customer_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/customer"
)

// fakeRepo is a minimal in-memory Repository for unit-testing Service.
type fakeRepo struct {
	profiles  map[string]*customer.CustomerProfile // keyed by "storeID|email"
	addresses []customer.CustomerAddress
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{profiles: make(map[string]*customer.CustomerProfile)}
}

func (r *fakeRepo) UpsertProfile(_ context.Context, p *customer.CustomerProfile) (*customer.CustomerProfile, error) {
	key := p.StoreID.String() + "|" + p.Email
	if existing, ok := r.profiles[key]; ok {
		existing.GipUID = p.GipUID
		existing.FirstName = p.FirstName
		existing.LastName = p.LastName
		return existing, nil
	}
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	r.profiles[key] = p
	return p, nil
}

func (r *fakeRepo) GetProfileByGipUID(_ context.Context, storeID uuid.UUID, gipUID string) (*customer.CustomerProfile, error) {
	for _, p := range r.profiles {
		if p.StoreID == storeID && p.GipUID != nil && *p.GipUID == gipUID {
			return p, nil
		}
	}
	return nil, customer.ErrNotFound
}

func (r *fakeRepo) GetProfileByID(_ context.Context, id uuid.UUID) (*customer.CustomerProfile, error) {
	for _, p := range r.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, customer.ErrNotFound
}

func (r *fakeRepo) UpdateProfile(_ context.Context, id uuid.UUID, updates map[string]any) (*customer.CustomerProfile, error) {
	return nil, nil // stub
}
func (r *fakeRepo) ListAddresses(_ context.Context, _ uuid.UUID) ([]customer.CustomerAddress, error) {
	return r.addresses, nil
}
func (r *fakeRepo) CreateAddress(_ context.Context, _ interface{ WithContext(context.Context) interface{} }, _ *customer.CustomerAddress) error {
	return nil // stub -- will not compile as-is; the real test uses *gorm.DB
}
// NOTE: For the fakeRepo to satisfy the Repository interface, all methods
// must have matching signatures including *gorm.DB params. In practice,
// either (a) define a narrower ServiceRepository interface that Service
// depends on, or (b) pass nil *gorm.DB in tests. Option (b) is simpler
// for now -- the fake methods ignore the *gorm.DB param.

// The actual test file should implement ALL Repository methods with the
// correct *gorm.DB signatures. The below tests focus on EnsureProfile.

func TestEnsureProfile_CreatesNewProfile(t *testing.T) {
	// This is a conceptual test. The real implementation should use
	// the integration test pattern (testdb) matching the project's
	// existing test patterns, or define a narrow interface for Service.
	//
	// If the project uses integration tests with a real DB, write
	// those instead. If it uses interface-based mocks, ensure the
	// fake fully satisfies Repository.
	t.Skip("Implement with project-appropriate test pattern (integration or interface mock)")
}

func TestEnsureProfile_IdempotentOnSecondCall(t *testing.T) {
	t.Skip("Implement with project-appropriate test pattern")
}

func TestEnsureProfile_EmptyEmailReturnsError(t *testing.T) {
	// This one is pure logic, no DB needed.
	svc := customer.NewService(nil, nil, slogDiscard())
	_, err := svc.EnsureProfile(context.Background(), customer.EnsureProfileInput{
		StoreID:  uuid.New(),
		TenantID: uuid.New(),
		GipUID:   "gip-123",
		Email:    "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email is required")
}

func slogDiscard() *slog.Logger {
	// Return a no-op logger for tests.
	return slog.Default() // Replace with slog.New(slog.NewTextHandler(io.Discard, nil)) if needed
}
```

> **IMPORTANT:** The test file above is a scaffold. The agent implementing this should check whether the project uses integration tests with a real test DB or interface-based mocks, and adapt accordingly. The fakeRepo pattern is illustrative -- make it compile by matching all `Repository` method signatures exactly. Alternatively, define a narrower `ServiceDeps` interface that `Service` depends on (containing only `UpsertProfile`), which is easier to mock.

- [ ] **Step 5: Verify Go compilation**

```bash
cd services/marketplace-api && go build ./internal/customer/...
```

Must exit 0 with no errors.

---

## Task 3: `OptionalCustomerAuth` middleware

**Files:** `services/marketplace-api/internal/handlers/storefront/middleware.go`, `services/marketplace-api/pkg/config/config.go`

- [ ] **Step 1: Add `CustomerSessionSecret` to config**

Edit `services/marketplace-api/pkg/config/config.go`. Add after the `StorefrontKey` field:

```go
	// CustomerSessionSecret is the HMAC key used to validate auth-bff
	// session cookies. When empty, OptionalCustomerAuth always yields
	// guest context — fine for local dev without auth-bff.
	CustomerSessionSecret string `envconfig:"CUSTOMER_SESSION_SECRET" default:""`
```

- [ ] **Step 2: Add `OptionalCustomerAuth` to middleware.go**

Append to `services/marketplace-api/internal/handlers/storefront/middleware.go`:

```go
import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)
```

> **NOTE:** Merge the import block with the existing one at the top of middleware.go. Do NOT duplicate imports. The existing file already imports `context`, `errors`, `net/http`, `gin`, `stores`, and `apperrors`. Add only the new ones: `crypto/hmac`, `crypto/sha256`, `encoding/base64`, `encoding/json`, `strings`, `time`, `log/slog`, `uuid`, and `customer`.

Add these types and functions after the existing `respondNotFound` function:

```go
// customerContextKey values set by OptionalCustomerAuth.
const (
	CustomerProfileIDKey = "customer_profile_id"
	CustomerEmailKey     = "customer_email"
	CustomerGipUIDKey    = "customer_gip_uid"
	CustomerProfileKey   = "customer_profile"
)

// sessionClaims represents the decoded auth-bff session cookie payload.
type sessionClaims struct {
	GipUID    string `json:"gip_uid"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Exp       int64  `json:"exp"`
}

// OptionalCustomerAuth reads the auth-bff session cookie, validates its
// HMAC signature, and if valid, upserts a customer_profiles row via
// Service.EnsureProfile and sets customer context on gin.
//
// If the cookie is missing, invalid, or expired, the request continues
// as a guest (no customer context). This middleware never aborts.
//
// Cookie format: base64(JSON payload) + "." + base64(HMAC-SHA256 signature).
// This mirrors auth-bff's cookie.go signing convention.
func OptionalCustomerAuth(secret string, customerSvc *customer.Service, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			// No secret configured — skip auth entirely (dev mode).
			c.Next()
			return
		}

		cookieVal, err := c.Cookie("mp_customer_session")
		if err != nil || cookieVal == "" {
			c.Next()
			return
		}

		claims, err := validateSessionCookie(cookieVal, secret)
		if err != nil {
			logger.Debug("invalid customer session cookie",
				"error", err,
				"remote_addr", c.ClientIP(),
			)
			c.Next()
			return
		}

		// Check expiry.
		if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
			logger.Debug("expired customer session cookie",
				"email", claims.Email,
			)
			c.Next()
			return
		}

		// Resolve the store from existing StoreContext middleware.
		storeVal, exists := c.Get("store")
		if !exists {
			c.Next()
			return
		}
		store, ok := storeVal.(*stores.Store)
		if !ok || store == nil {
			c.Next()
			return
		}

		storeID, err := uuid.Parse(store.ID)
		if err != nil {
			c.Next()
			return
		}
		tenantID, err := uuid.Parse(store.TenantID)
		if err != nil {
			c.Next()
			return
		}

		// Auto-create profile on first login (spec section 4.3).
		profile, err := customerSvc.EnsureProfile(c.Request.Context(), customer.EnsureProfileInput{
			StoreID:   storeID,
			TenantID:  tenantID,
			GipUID:    claims.GipUID,
			Email:     claims.Email,
			FirstName: claims.FirstName,
			LastName:  claims.LastName,
		})
		if err != nil {
			logger.Error("failed to ensure customer profile",
				"error", err,
				"email", claims.Email,
				"store_id", store.ID,
			)
			// Don't abort — degrade to guest.
			c.Next()
			return
		}

		// Set customer context on gin for downstream handlers.
		c.Set(CustomerProfileIDKey, profile.ID.String())
		c.Set(CustomerEmailKey, profile.Email)
		c.Set(CustomerGipUIDKey, claims.GipUID)
		c.Set(CustomerProfileKey, profile)

		c.Next()
	}
}

// validateSessionCookie validates the HMAC-SHA256 signature of the cookie.
// Cookie format: base64url(payload) + "." + base64url(signature).
func validateSessionCookie(cookie, secret string) (*sessionClaims, error) {
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed cookie: missing separator")
	}

	payloadB64 := parts[0]
	sigB64 := parts[1]

	// Verify HMAC.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, errors.New("malformed cookie: invalid signature encoding")
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("invalid cookie signature")
	}

	// Decode payload.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("malformed cookie: invalid payload encoding")
	}

	var claims sessionClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("malformed cookie: %w", err)
	}

	if claims.Email == "" {
		return nil, errors.New("malformed cookie: missing email")
	}

	return &claims, nil
}

// RequireCustomerAuth wraps OptionalCustomerAuth and returns 401 if no
// customer context was set. Use on /account/* routes.
func RequireCustomerAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		profileID, exists := c.Get(CustomerProfileIDKey)
		if !exists || profileID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
				"error":   "unauthorized",
				"message": "Authentication required. Please sign in.",
			})
			return
		}

		// Check if customer is blocked (spec section 9.2).
		profileVal, _ := c.Get(CustomerProfileKey)
		if profile, ok := profileVal.(*customer.CustomerProfile); ok && profile != nil {
			if profile.Status == customer.StatusBlocked {
				c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
					"error":   "forbidden",
					"message": "Your account has been suspended. Please contact the store.",
				})
				return
			}
		}

		c.Next()
	}
}
```

> **IMPORTANT:** The existing `middleware.go` has its own import block. Merge imports carefully -- do NOT create a second `package` declaration or duplicate imports. Add the new imports to the existing import block.

- [ ] **Step 3: Verify compilation**

```bash
cd services/marketplace-api && go build ./internal/handlers/storefront/...
```

---

## Task 4: `RequireCustomerAuth` middleware

This was implemented as part of Task 3 (the `RequireCustomerAuth` function at the bottom of the middleware additions). No separate task needed.

Verify it exists and compiles as part of Task 3 Step 3.

---

## Task 5: Storefront account handlers

**Files:** `services/marketplace-api/internal/handlers/storefront/customer_account.go`

- [ ] **Step 1: Create `services/marketplace-api/internal/handlers/storefront/customer_account.go`**

```go
// Package storefront — customer_account.go: Storefront account handlers.
// GET/PATCH profile, addresses CRUD. All routes require RequireCustomerAuth.
package storefront

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// CustomerAccountHandler serves the storefront /account/* routes.
type CustomerAccountHandler struct {
	db          *gorm.DB
	repo        customer.Repository
	customerSvc *customer.Service
	logger      *slog.Logger
}

// NewCustomerAccountHandler constructs a CustomerAccountHandler.
func NewCustomerAccountHandler(
	db *gorm.DB,
	repo customer.Repository,
	customerSvc *customer.Service,
	logger *slog.Logger,
) *CustomerAccountHandler {
	return &CustomerAccountHandler{
		db:          db,
		repo:        repo,
		customerSvc: customerSvc,
		logger:      logger,
	}
}

// GetProfile handles GET /storefront/stores/:storeSlug/account.
// Returns the customer-facing profile (excludes admin-only fields per spec 9.7).
func (h *CustomerAccountHandler) GetProfile(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(profile)})
}

// UpdateProfile handles PATCH /storefront/stores/:storeSlug/account.
func (h *CustomerAccountHandler) UpdateProfile(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	var req customer.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// Build update map — only non-nil fields.
	updates := make(map[string]any)
	if req.FirstName != nil {
		updates["first_name"] = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		updates["last_name"] = strings.TrimSpace(*req.LastName)
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.AvatarURL != nil {
		// Validate avatar URL — only GCS-originated URLs (spec section 9.3).
		av := strings.TrimSpace(*req.AvatarURL)
		if av != "" && !strings.HasPrefix(av, "https://storage.googleapis.com/") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "validation_error",
				"message": "avatar_url must be a Google Cloud Storage URL",
			})
			return
		}
		updates["avatar_url"] = av
	}
	if req.MarketingOptIn != nil {
		updates["marketing_opt_in"] = *req.MarketingOptIn
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(profile)})
		return
	}

	updated, err := h.repo.UpdateProfile(c.Request.Context(), profile.ID, updates)
	if err != nil {
		h.logger.Error("failed to update customer profile",
			"error", err,
			"profile_id", profile.ID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to update profile",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(updated)})
}

// ListAddresses handles GET /storefront/stores/:storeSlug/account/addresses.
func (h *CustomerAccountHandler) ListAddresses(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	addrs, err := h.repo.ListAddresses(c.Request.Context(), profile.ID)
	if err != nil {
		h.logger.Error("failed to list addresses", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to list addresses",
		})
		return
	}

	resp := make([]customer.AddressResponse, 0, len(addrs))
	for _, a := range addrs {
		resp = append(resp, mapAddressToResponse(&a))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// CreateAddress handles POST /storefront/stores/:storeSlug/account/addresses.
func (h *CustomerAccountHandler) CreateAddress(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	var req customer.CreateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	addr := &customer.CustomerAddress{
		TenantID:    profile.TenantID,
		CustomerID:  profile.ID,
		Label:       req.Label,
		IsDefault:   req.IsDefault,
		Name:        strings.TrimSpace(req.Name),
		Line1:       strings.TrimSpace(req.Line1),
		Line2:       req.Line2,
		City:        strings.TrimSpace(req.City),
		Region:      req.Region,
		PostalCode:  req.PostalCode,
		CountryCode: strings.ToUpper(strings.TrimSpace(req.CountryCode)),
		Phone:       req.Phone,
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if addr.IsDefault {
			if err := h.repo.ClearDefaultAddresses(c.Request.Context(), tx, profile.ID); err != nil {
				return err
			}
		}
		return h.repo.CreateAddress(c.Request.Context(), tx, addr)
	})
	if err != nil {
		h.logger.Error("failed to create address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to create address",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": mapAddressToResponse(addr)})
}

// UpdateAddress handles PATCH /storefront/stores/:storeSlug/account/addresses/:id.
func (h *CustomerAccountHandler) UpdateAddress(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	addrID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "Invalid address ID",
		})
		return
	}

	// Verify ownership.
	if _, err := h.repo.GetAddress(c.Request.Context(), addrID, profile.ID); err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Address not found",
			})
			return
		}
		h.logger.Error("failed to get address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal error",
		})
		return
	}

	var req customer.UpdateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	updates := make(map[string]any)
	if req.Label != nil {
		updates["label"] = strings.TrimSpace(*req.Label)
	}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Line1 != nil {
		updates["line1"] = strings.TrimSpace(*req.Line1)
	}
	if req.Line2 != nil {
		updates["line2"] = strings.TrimSpace(*req.Line2)
	}
	if req.City != nil {
		updates["city"] = strings.TrimSpace(*req.City)
	}
	if req.Region != nil {
		updates["region"] = strings.TrimSpace(*req.Region)
	}
	if req.PostalCode != nil {
		updates["postal_code"] = strings.TrimSpace(*req.PostalCode)
	}
	if req.CountryCode != nil {
		updates["country_code"] = strings.ToUpper(strings.TrimSpace(*req.CountryCode))
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}

	var updated *customer.CustomerAddress
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if req.IsDefault != nil && *req.IsDefault {
			if clearErr := h.repo.ClearDefaultAddresses(c.Request.Context(), tx, profile.ID); clearErr != nil {
				return clearErr
			}
			updates["is_default"] = true
		} else if req.IsDefault != nil {
			updates["is_default"] = false
		}
		var updateErr error
		updated, updateErr = h.repo.UpdateAddress(c.Request.Context(), tx, addrID, updates)
		return updateErr
	})
	if err != nil {
		h.logger.Error("failed to update address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to update address",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapAddressToResponse(updated)})
}

// DeleteAddress handles DELETE /storefront/stores/:storeSlug/account/addresses/:id.
func (h *CustomerAccountHandler) DeleteAddress(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	addrID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "Invalid address ID",
		})
		return
	}

	if err := h.repo.DeleteAddress(c.Request.Context(), addrID, profile.ID); err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Address not found",
			})
			return
		}
		h.logger.Error("failed to delete address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to delete address",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Address deleted"})
}

// --- helpers ---

// mustGetProfile extracts the customer profile from gin context (set by
// OptionalCustomerAuth). Returns nil and writes 401 if missing.
func (h *CustomerAccountHandler) mustGetProfile(c *gin.Context) *customer.CustomerProfile {
	val, exists := c.Get(CustomerProfileKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return nil
	}
	profile, ok := val.(*customer.CustomerProfile)
	if !ok || profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return nil
	}
	return profile
}

func mapProfileToResponse(p *customer.CustomerProfile) customer.StorefrontProfileResponse {
	resp := customer.StorefrontProfileResponse{
		ID:             p.ID.String(),
		Email:          p.Email,
		MarketingOptIn: p.MarketingOptIn,
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
	}
	if p.FirstName != nil {
		resp.FirstName = *p.FirstName
	}
	if p.LastName != nil {
		resp.LastName = *p.LastName
	}
	if p.Phone != nil {
		resp.Phone = *p.Phone
	}
	if p.AvatarURL != nil {
		resp.AvatarURL = *p.AvatarURL
	}
	return resp
}

func mapAddressToResponse(a *customer.CustomerAddress) customer.AddressResponse {
	resp := customer.AddressResponse{
		ID:          a.ID.String(),
		IsDefault:   a.IsDefault,
		Name:        a.Name,
		Line1:       a.Line1,
		City:        a.City,
		CountryCode: a.CountryCode,
	}
	if a.Label != nil {
		resp.Label = *a.Label
	}
	if a.Line2 != nil {
		resp.Line2 = *a.Line2
	}
	if a.Region != nil {
		resp.Region = *a.Region
	}
	if a.PostalCode != nil {
		resp.PostalCode = *a.PostalCode
	}
	if a.Phone != nil {
		resp.Phone = *a.Phone
	}
	return resp
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd services/marketplace-api && go build ./internal/handlers/storefront/...
```

---

## Task 6: Wire into storefront Deps + routes + main.go

**Files:** `services/marketplace-api/internal/handlers/storefront/routes.go`, `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Update Deps struct in routes.go**

Add to the `Deps` struct in `services/marketplace-api/internal/handlers/storefront/routes.go`:

```go
	// C1 customer auth.
	CustomerAccountHandler *CustomerAccountHandler
	CustomerService        *customer.Service
	CustomerSessionSecret  string
```

Add the import for customer:

```go
	"github.com/mark8ly/marketplace-api/internal/customer"
```

> **NOTE:** Add `"log/slog"` to imports as well since we need a logger reference for the middleware.

Add a `Logger` field to Deps:

```go
	Logger *slog.Logger
```

- [ ] **Step 2: Add account routes to RegisterStorefront**

In `RegisterStorefront` function in `routes.go`, after the existing `group` block (after the `}` that closes the initial route registrations), add:

```go
	// C1 — Customer account routes (auth required).
	if deps.CustomerAccountHandler != nil {
		optionalAuth := OptionalCustomerAuth(deps.CustomerSessionSecret, deps.CustomerService, deps.Logger)
		requireAuth := RequireCustomerAuth()

		// Apply optional auth to ALL storefront routes so customer context
		// is available even on product pages (for future review submission).
		group.Use(optionalAuth)

		account := group.Group("/account", requireAuth)
		{
			account.GET("", deps.CustomerAccountHandler.GetProfile)
			account.PATCH("", deps.CustomerAccountHandler.UpdateProfile)
			account.GET("/addresses", deps.CustomerAccountHandler.ListAddresses)
			account.POST("/addresses", deps.CustomerAccountHandler.CreateAddress)
			account.PATCH("/addresses/:id", deps.CustomerAccountHandler.UpdateAddress)
			account.DELETE("/addresses/:id", deps.CustomerAccountHandler.DeleteAddress)
		}
	}
```

> **IMPORTANT:** The `group.Use(optionalAuth)` line adds OptionalCustomerAuth to all routes in the storefront group. This means product listing and detail routes also get customer context when a session cookie is present. This is intentional -- C3 (Reviews) will use it for review submission. If you want to apply OptionalCustomerAuth more surgically (only to /account and future /reviews routes), create a separate sub-group instead.

- [ ] **Step 3: Wire customer dependencies in main.go**

In `services/marketplace-api/cmd/marketplace-api/main.go`, inside the `if m == mode.Storefront || m == mode.Both {` block (around line 230), add after the existing storefront wiring:

```go
		// C1 — Customer profiles and account.
		customerRepo := customer.NewRepository(conn)
		customerSvc := customer.NewService(conn, customerRepo, log)
		customerAccountHandler := storefront.NewCustomerAccountHandler(conn, customerRepo, customerSvc, log)
```

Add the import at the top of main.go:

```go
	"github.com/mark8ly/marketplace-api/internal/customer"
```

Update the `storefrontDeps` assignment to include the new fields:

```go
		storefrontDeps = storefront.Deps{
			Handler:               storefrontHandler,
			CheckoutHandler:       checkoutHandler,
			CheckoutExtHandler:    checkoutExtHandler,
			PaymentMethodsHandler: paymentMethodsHandler,
			ShippingRatesHandler:  shippingRatesHandler,
			WebhookHandler:        webhookHandler,
			OrderDetailHandler:    orderDetailHandler,
			SlugCache:             slugCache,
			StorefrontKey:         cfg.StorefrontKey,
			CountryHandler:        countryHandler,
			// C1 customer auth.
			CustomerAccountHandler: customerAccountHandler,
			CustomerService:        customerSvc,
			CustomerSessionSecret:  cfg.CustomerSessionSecret,
			Logger:                 log,
		}
```

- [ ] **Step 4: Verify full build**

```bash
cd services/marketplace-api && go build ./...
```

Must exit 0.

- [ ] **Step 5: Run existing tests**

```bash
cd services/marketplace-api && go test ./... -count=1 -short
```

All existing tests must still pass. Fix any compilation or test failures before proceeding.

---

## Task 7: Storefront UI — update StorefrontNav with sign-in / account dropdown

**Files:** `apps/storefront/.env.example`, `apps/storefront/lib/auth.ts`, `apps/storefront/lib/api/customer-api.ts`, `apps/storefront/components/CustomerAuthProvider.tsx`, `apps/storefront/components/StorefrontNav.tsx`

- [ ] **Step 1: Update `.env.example`**

Add to `apps/storefront/.env.example`:

```
# Auth-bff URL for customer login redirect. Points to the auth-bff
# instance that handles the GIP mp-customer OIDC flow.
AUTH_BFF_URL=http://localhost:8085
# Marketplace API URL (already should be present if not, add it)
MARKETPLACE_API_URL=http://localhost:8088
```

- [ ] **Step 2: Create `apps/storefront/lib/auth.ts`**

```typescript
// apps/storefront/lib/auth.ts
//
// Auth helpers for the storefront. Builds login/logout URLs that redirect
// to auth-bff, and reads customer session state from cookies.

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8085";

/**
 * Builds the auth-bff login URL for the mp-customer GIP pool.
 * After successful login, auth-bff redirects back to `redirectUri`.
 *
 * Spec section 4.1: /login?product=mp-customer&redirect_uri=<storefront-url>/account
 */
export function buildLoginUrl(redirectUri: string): string {
  const params = new URLSearchParams({
    product: "mp-customer",
    redirect_uri: redirectUri,
  });
  return `${AUTH_BFF_URL}/login?${params.toString()}`;
}

/**
 * Builds the auth-bff logout URL.
 */
export function buildLogoutUrl(redirectUri: string): string {
  const params = new URLSearchParams({
    product: "mp-customer",
    redirect_uri: redirectUri,
  });
  return `${AUTH_BFF_URL}/logout?${params.toString()}`;
}

/**
 * Checks whether the mp_customer_session cookie is present in
 * the request headers. Server-side only (reads from headers()).
 *
 * This does NOT validate the cookie signature — the Go middleware
 * does that. This is purely for UI purposes (show sign-in vs account).
 */
export function hasSessionCookie(cookieHeader: string | null): boolean {
  if (!cookieHeader) return false;
  return cookieHeader.includes("mp_customer_session=");
}
```

- [ ] **Step 3: Create `apps/storefront/lib/api/customer-api.ts`**

```typescript
// apps/storefront/lib/api/customer-api.ts
//
// Storefront client for customer account endpoints. Server-side only —
// forwards the customer session cookie to marketplace-api.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function accountHeaders(cookieHeader: string): HeadersInit {
  const headers: Record<string, string> = {
    Accept: "application/json",
    Cookie: cookieHeader,
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;
  return headers;
}

export interface CustomerProfile {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone: string;
  avatar_url: string;
  marketing_opt_in: boolean;
  created_at: string;
}

export interface CustomerAddress {
  id: string;
  label: string;
  is_default: boolean;
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal_code: string;
  country_code: string;
  phone: string;
}

/**
 * Fetches the authenticated customer's profile. Returns null on auth failure.
 */
export async function fetchProfile(
  storeSlug: string,
  cookieHeader: string,
): Promise<CustomerProfile | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account`;
  try {
    const res = await fetch(url, {
      headers: accountHeaders(cookieHeader),
      cache: "no-store",
    });
    if (!res.ok) return null;
    const body = (await res.json()) as { data: CustomerProfile };
    return body.data ?? null;
  } catch {
    return null;
  }
}

/**
 * Fetches the authenticated customer's saved addresses.
 */
export async function fetchAddresses(
  storeSlug: string,
  cookieHeader: string,
): Promise<CustomerAddress[]> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account/addresses`;
  try {
    const res = await fetch(url, {
      headers: accountHeaders(cookieHeader),
      cache: "no-store",
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { data: CustomerAddress[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}
```

- [ ] **Step 4: Create `apps/storefront/components/CustomerAuthProvider.tsx`**

```tsx
// apps/storefront/components/CustomerAuthProvider.tsx
//
// React context that provides customer auth state to client components.
// Populated server-side by reading the cookie and passing profile data down.
"use client";

import { createContext, useContext, type ReactNode } from "react";

interface CustomerAuthState {
  /** Whether the customer is authenticated (has a valid session). */
  isAuthenticated: boolean;
  /** Customer display name (first_name or email). Null if not authenticated. */
  displayName: string | null;
  /** Customer email. Null if not authenticated. */
  email: string | null;
  /** Login URL (auth-bff redirect). */
  loginUrl: string;
  /** Logout URL (auth-bff redirect). */
  logoutUrl: string;
}

const CustomerAuthContext = createContext<CustomerAuthState>({
  isAuthenticated: false,
  displayName: null,
  email: null,
  loginUrl: "#",
  logoutUrl: "#",
});

export function useCustomerAuth(): CustomerAuthState {
  return useContext(CustomerAuthContext);
}

interface CustomerAuthProviderProps {
  children: ReactNode;
  value: CustomerAuthState;
}

export function CustomerAuthProvider({
  children,
  value,
}: CustomerAuthProviderProps) {
  return (
    <CustomerAuthContext.Provider value={value}>
      {children}
    </CustomerAuthContext.Provider>
  );
}
```

- [ ] **Step 5: Update `apps/storefront/components/StorefrontNav.tsx`**

Replace the entire file with:

```tsx
// apps/storefront/components/StorefrontNav.tsx
//
// Storefront navigation bar. Home / Shop / Cart / Sign in (or Account).
// Uses Paper * Ink * Moss tokens.

"use client";

import { Suspense, useState, useRef, useEffect } from "react";
import Link from "next/link";
import { CartCountBadge } from "./CartCountBadge";
import { useCustomerAuth } from "./CustomerAuthProvider";

export interface StorefrontNavProps {
  /** Optional store name shown as the left-hand brand slot. */
  storeName?: string;
}

export function StorefrontNav({ storeName }: StorefrontNavProps) {
  const { isAuthenticated, displayName, loginUrl, logoutUrl } =
    useCustomerAuth();

  return (
    <nav
      aria-label="Store"
      className="mb-10 flex items-center justify-between gap-4 border-b border-[color:var(--ink-900)] border-opacity-10 pb-4"
    >
      <Link
        href="/"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        {storeName ?? "Store"}
      </Link>
      <ul className="flex items-center gap-6 text-sm">
        <li>
          <Link
            href="/"
            className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Home
          </Link>
        </li>
        <li>
          <Link
            href="/products"
            className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Shop
          </Link>
        </li>
        <li>
          <Link
            href="/cart"
            className="inline-flex items-center gap-1.5 text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Cart
            <Suspense fallback={null}>
              <CartCountBadge />
            </Suspense>
          </Link>
        </li>
        <li>
          {isAuthenticated ? (
            <AccountDropdown
              displayName={displayName}
              logoutUrl={logoutUrl}
            />
          ) : (
            <a
              href={loginUrl}
              className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Sign in
            </a>
          )}
        </li>
      </ul>
    </nav>
  );
}

interface AccountDropdownProps {
  displayName: string | null;
  logoutUrl: string;
}

function AccountDropdown({ displayName, logoutUrl }: AccountDropdownProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click.
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside);
    }
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  // Close on Escape.
  useEffect(() => {
    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    if (open) {
      document.addEventListener("keydown", handleEscape);
    }
    return () => document.removeEventListener("keydown", handleEscape);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        aria-haspopup="true"
        className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        {displayName ?? "Account"}
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 top-full z-50 mt-2 min-w-[180px] rounded-md border border-[color:var(--ink-900)]/10 bg-white py-1 shadow-md"
        >
          <Link
            href="/account"
            role="menuitem"
            className="block px-4 py-2 text-sm text-[color:var(--ink-900)] hover:bg-[color:var(--paper-200)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
            onClick={() => setOpen(false)}
          >
            Dashboard
          </Link>
          <Link
            href="/account/orders"
            role="menuitem"
            className="block px-4 py-2 text-sm text-[color:var(--ink-900)] hover:bg-[color:var(--paper-200)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
            onClick={() => setOpen(false)}
          >
            Orders
          </Link>
          <Link
            href="/account/addresses"
            role="menuitem"
            className="block px-4 py-2 text-sm text-[color:var(--ink-900)] hover:bg-[color:var(--paper-200)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
            onClick={() => setOpen(false)}
          >
            Addresses
          </Link>
          <div className="my-1 border-t border-[color:var(--ink-900)]/10" />
          <a
            href={logoutUrl}
            role="menuitem"
            className="block px-4 py-2 text-sm text-[color:var(--ink-900)] opacity-60 hover:bg-[color:var(--paper-200)] hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
          >
            Sign out
          </a>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 6: Wire CustomerAuthProvider into root layout**

Edit `apps/storefront/app/layout.tsx`. Add imports at the top:

```typescript
import { cookies } from "next/headers";
import { CustomerAuthProvider } from "@/components/CustomerAuthProvider";
import { buildLoginUrl, buildLogoutUrl, hasSessionCookie } from "@/lib/auth";
import { fetchProfile } from "@/lib/api/customer-api";
```

Inside `RootLayout`, after the existing `const storeSlug = ...` line, add:

```typescript
  // Customer auth state — read cookie, optionally fetch profile for display name.
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  const isAuthenticated = hasSessionCookie(cookieHeader);

  let displayName: string | null = null;
  let email: string | null = null;
  if (isAuthenticated && storeSlug) {
    const profile = await fetchProfile(storeSlug, cookieHeader).catch(
      () => null,
    );
    if (profile) {
      displayName = profile.first_name || profile.email.split("@")[0] || null;
      email = profile.email;
    }
  }

  const origin =
    h.get("x-forwarded-proto") && h.get("host")
      ? `${h.get("x-forwarded-proto")}://${h.get("host")}`
      : `http://${h.get("host") ?? "localhost:4203"}`;
  const loginUrl = buildLoginUrl(`${origin}/account`);
  const logoutUrl = buildLogoutUrl(origin);

  const authState = {
    isAuthenticated: isAuthenticated && displayName !== null,
    displayName,
    email,
    loginUrl,
    logoutUrl,
  };
```

Update the JSX return to wrap children with `CustomerAuthProvider`:

```tsx
  return (
    <html lang="en" className={fontVars}>
      <body>
        <SkipLink />
        <CustomerAuthProvider value={authState}>
          <CartProvider storeSlug={storeSlug}>{children}</CartProvider>
        </CustomerAuthProvider>
      </body>
    </html>
  );
```

> **NOTE:** `CustomerAuthProvider` wraps `CartProvider` so both contexts are available to all children. The `"use client"` on `CustomerAuthProvider` is necessary because it uses `createContext`.

- [ ] **Step 7: Verify TypeScript compilation**

```bash
cd apps/storefront && npx tsc --noEmit
```

Must pass with no errors. Fix any import issues.

---

## Task 8: Storefront UI — /account pages

**Files:** `apps/storefront/components/AccountSidebar.tsx`, `apps/storefront/app/account/layout.tsx`, `apps/storefront/app/account/page.tsx`, `apps/storefront/app/account/orders/page.tsx`, `apps/storefront/app/account/addresses/page.tsx`

- [ ] **Step 1: Create `apps/storefront/components/AccountSidebar.tsx`**

```tsx
// apps/storefront/components/AccountSidebar.tsx
//
// Sidebar navigation for /account/* pages. Highlights the active route.

"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const NAV_ITEMS = [
  { href: "/account", label: "Dashboard", exact: true },
  { href: "/account/orders", label: "Orders", exact: false },
  { href: "/account/addresses", label: "Addresses", exact: false },
] as const;

export function AccountSidebar() {
  const pathname = usePathname();

  return (
    <nav aria-label="Account" className="space-y-1">
      {NAV_ITEMS.map((item) => {
        const active = item.exact
          ? pathname === item.href
          : pathname.startsWith(item.href);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={`block rounded-md px-3 py-2 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] ${
              active
                ? "bg-[color:var(--ink-900)]/5 font-medium text-[color:var(--ink-900)]"
                : "text-[color:var(--ink-900)] opacity-60 hover:opacity-100"
            }`}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
```

- [ ] **Step 2: Create `apps/storefront/app/account/layout.tsx`**

```tsx
// apps/storefront/app/account/layout.tsx
//
// Account layout — auth gate + sidebar. Server component.
// If not authenticated, redirects to login.

import type { ReactNode } from "react";
import { headers, cookies } from "next/headers";
import { redirect } from "next/navigation";

import { slugFromHost } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { hasSessionCookie, buildLoginUrl } from "@/lib/auth";
import { StorefrontNav } from "@/components/StorefrontNav";
import { AccountSidebar } from "@/components/AccountSidebar";

export const dynamic = "force-dynamic";

interface AccountLayoutProps {
  children: ReactNode;
}

export default async function AccountLayout({ children }: AccountLayoutProps) {
  const h = await headers();
  const host = h.get("host");
  const slug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "";

  // Auth gate — redirect to login if no session cookie.
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  if (!hasSessionCookie(cookieHeader)) {
    const origin =
      h.get("x-forwarded-proto") && host
        ? `${h.get("x-forwarded-proto")}://${host}`
        : `http://${host ?? "localhost:4203"}`;
    redirect(buildLoginUrl(`${origin}/account`));
  }

  const store = slug ? await fetchStoreBySlug(slug).catch(() => null) : null;

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-5xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store?.name} />
        <div className="mt-8 grid grid-cols-1 gap-10 md:grid-cols-[200px_1fr]">
          <aside>
            <AccountSidebar />
          </aside>
          <section>{children}</section>
        </div>
      </div>
    </main>
  );
}
```

- [ ] **Step 3: Create `apps/storefront/app/account/page.tsx`**

```tsx
// apps/storefront/app/account/page.tsx
//
// Account dashboard — welcome message, profile summary, recent orders quick links.

import type { Metadata } from "next";
import Link from "next/link";
import { headers, cookies } from "next/headers";

import { slugFromHost } from "@/lib/slug";
import { fetchProfile } from "@/lib/api/customer-api";

export const metadata: Metadata = {
  title: "My Account",
};

export default async function AccountDashboardPage() {
  const h = await headers();
  const host = h.get("host");
  const slug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "";
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const profile = slug
    ? await fetchProfile(slug, cookieHeader).catch(() => null)
    : null;

  if (!profile) {
    return (
      <div>
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]">
          My Account
        </h1>
        <p className="mt-4 text-sm text-[color:var(--ink-900)] opacity-60">
          Unable to load your profile. Please try refreshing the page.
        </p>
      </div>
    );
  }

  const displayName =
    [profile.first_name, profile.last_name].filter(Boolean).join(" ") ||
    profile.email;

  return (
    <div>
      <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--moss-700)]">
        Welcome back
      </p>
      <h1 className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]">
        {displayName}
      </h1>
      <p className="mt-1 text-sm text-[color:var(--ink-900)] opacity-60">
        {profile.email}
      </p>

      {/* Quick links */}
      <div className="mt-10 grid grid-cols-1 gap-4 sm:grid-cols-3">
        <QuickLink
          href="/account/orders"
          title="Orders"
          description="View your order history"
        />
        <QuickLink
          href="/account/addresses"
          title="Addresses"
          description="Manage saved addresses"
        />
        <QuickLink
          href="/products"
          title="Continue shopping"
          description="Browse the latest products"
        />
      </div>
    </div>
  );
}

function QuickLink({
  href,
  title,
  description,
}: {
  href: string;
  title: string;
  description: string;
}) {
  return (
    <Link
      href={href}
      className="group rounded-md border border-[color:var(--ink-900)]/10 bg-white p-5 transition-shadow hover:shadow-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
    >
      <p className="text-sm font-medium text-[color:var(--ink-900)] group-hover:text-[color:var(--moss-700)]">
        {title}
      </p>
      <p className="mt-1 text-xs text-[color:var(--ink-900)] opacity-50">
        {description}
      </p>
    </Link>
  );
}
```

- [ ] **Step 4: Create `apps/storefront/app/account/orders/page.tsx`**

```tsx
// apps/storefront/app/account/orders/page.tsx
//
// Order history page. Placeholder — C1 ships the shell, C2/C3 will
// add full order list fetching. For now shows empty state.

import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "My Orders",
};

export default function AccountOrdersPage() {
  return (
    <div>
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]">
        Orders
      </h1>

      {/* Empty state per spec section 10.6 */}
      <div className="mt-8 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-6 py-12 text-center">
        <p className="text-sm text-[color:var(--ink-900)] opacity-60">
          You haven&apos;t placed any orders yet.
        </p>
        <Link
          href="/products"
          className="mt-4 inline-block rounded-md bg-[color:var(--ink-900)] px-5 py-2.5 text-sm font-medium text-[color:var(--paper-200)] transition-all duration-150 hover:opacity-90 active:scale-[0.99] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Start shopping
        </Link>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Create `apps/storefront/app/account/addresses/page.tsx`**

```tsx
// apps/storefront/app/account/addresses/page.tsx
//
// Saved addresses page. Fetches from marketplace-api and renders
// the address list with add/edit/delete capabilities.

import type { Metadata } from "next";
import { headers, cookies } from "next/headers";

import { slugFromHost } from "@/lib/slug";
import { fetchAddresses, type CustomerAddress } from "@/lib/api/customer-api";

export const metadata: Metadata = {
  title: "My Addresses",
};

export const dynamic = "force-dynamic";

export default async function AccountAddressesPage() {
  const h = await headers();
  const host = h.get("host");
  const slug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "";
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const addresses = slug
    ? await fetchAddresses(slug, cookieHeader).catch(() => [])
    : [];

  return (
    <div>
      <div className="flex items-center justify-between">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]">
          Addresses
        </h1>
        {/* TODO: Add address button — client component form (follow-up in C1 polish) */}
      </div>

      {addresses.length === 0 ? (
        <div className="mt-8 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-6 py-12 text-center">
          <p className="text-sm text-[color:var(--ink-900)] opacity-60">
            No saved addresses. Add one for faster checkout.
          </p>
        </div>
      ) : (
        <ul className="mt-6 space-y-4">
          {addresses.map((addr) => (
            <AddressCard key={addr.id} address={addr} />
          ))}
        </ul>
      )}
    </div>
  );
}

function AddressCard({ address }: { address: CustomerAddress }) {
  return (
    <li className="rounded-md border border-[color:var(--ink-900)]/10 bg-white p-5">
      <div className="flex items-start justify-between">
        <div>
          {address.label && (
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-[color:var(--moss-700)]">
              {address.label}
            </p>
          )}
          <p className="mt-1 text-sm font-medium text-[color:var(--ink-900)]">
            {address.name}
          </p>
          <address className="mt-1 text-sm not-italic leading-relaxed text-[color:var(--ink-900)] opacity-70">
            {address.line1}
            {address.line2 && (
              <>
                <br />
                {address.line2}
              </>
            )}
            <br />
            {address.city}
            {address.region && `, ${address.region}`}
            {address.postal_code && ` ${address.postal_code}`}
            <br />
            {address.country_code}
          </address>
        </div>
        {address.is_default && (
          <span className="rounded-full border border-[color:var(--moss-700)]/20 px-2 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
            Default
          </span>
        )}
      </div>
    </li>
  );
}
```

- [ ] **Step 6: Verify TypeScript compilation**

```bash
cd apps/storefront && npx tsc --noEmit
```

---

## Task 9: Build verification

- [ ] **Step 1: Full Go build**

```bash
cd services/marketplace-api && go build ./...
```

- [ ] **Step 2: Go tests**

```bash
cd services/marketplace-api && go test ./... -count=1 -short
```

All tests must pass.

- [ ] **Step 3: Storefront TypeScript check**

```bash
cd apps/storefront && npx tsc --noEmit
```

- [ ] **Step 4: Storefront dev build**

```bash
cd apps/storefront && npx next build
```

Must succeed. Fix any build errors.

- [ ] **Step 5: Manual smoke test checklist**

Run both services locally:

```bash
# Terminal 1 — marketplace-api
cd services/marketplace-api
MODE=both DATABASE_URL="postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable" \
  MARKETPLACE_FGA_API_URL="http://localhost:8080" \
  go run ./cmd/marketplace-api

# Terminal 2 — storefront
cd apps/storefront
MARKETPLACE_API_URL=http://localhost:8088 \
  AUTH_BFF_URL=http://localhost:8085 \
  DEFAULT_STORE_SLUG=demo \
  npm run dev
```

Verify:

1. Storefront home page loads without errors
2. "Sign in" link appears in nav (when not authenticated)
3. Clicking "Sign in" redirects to auth-bff login URL with `product=mp-customer`
4. `/account` redirects to login if no cookie present
5. API endpoint `GET /api/v1/storefront/stores/demo/account` returns 401 without cookie
6. API endpoint `GET /api/v1/storefront/stores/demo/account/addresses` returns 401 without cookie

- [ ] **Step 6: Verify migration is reversible**

```bash
cd services/marketplace-api
DATABASE_URL="${DATABASE_URL}" go run ./cmd/migrate down 1
DATABASE_URL="${DATABASE_URL}" go run ./cmd/migrate up
```

Both directions must succeed without errors.

---

## Notes for executing agent

1. **Import merging:** When modifying `middleware.go`, do NOT create a second import block. Merge new imports into the existing one.

2. **Migration number:** The spec says 000013 but the repo's latest is 000008. Use the next available number. Check with `ls services/marketplace-api/migrations/ | tail -2` before creating.

3. **Cookie format assumption:** The plan assumes auth-bff uses `base64url(payload).base64url(hmac-sha256)` cookie format. If auth-bff uses a different format (e.g., encrypted JWE, gorilla/securecookie), adapt `validateSessionCookie` accordingly. Check `auth-bff/internal/session/cookie.go` if accessible.

4. **`pq.StringArray`:** The `Tags` field uses `github.com/lib/pq` for the `text[]` type. Verify this dependency exists in `go.mod`. If not, run `go get github.com/lib/pq`.

5. **`fmt` import in middleware.go:** The `validateSessionCookie` function uses `fmt.Errorf`. The existing middleware.go does not import `fmt`. Add it to the merged import block.

6. **`cookies()` in layout.tsx:** Next.js 16 with React 19 uses `await cookies()` (async). The existing layout already uses `await headers()` so this pattern is established.

7. **No Loyalty tab:** Per spec section 10.5, the Loyalty tab is hidden until M3 ships. The account sidebar intentionally omits it.
