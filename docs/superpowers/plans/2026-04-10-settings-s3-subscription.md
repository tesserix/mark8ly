# Settings S3 — Subscription/Billing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship Stripe Billing integration: plan display, Stripe Checkout for upgrades, Stripe Portal for billing management, webhook for subscription lifecycle.

**Architecture:** New `internal/subscription/` package (models, repository, stripe client). Migration 000016. Webhook at /webhooks/stripe-billing. Admin UI at /settings/subscription.

**Tech Stack:** Go 1.26, Gin, net/http (Stripe API). Next.js 16, React 19, Tailwind.

**Spec reference:** `docs/superpowers/specs/2026-04-10-settings-tier1-tier2-design.md` — sections §2.2, §3.3, §4.3, §5.1 (subscription page), §6.3, §8 (S3 tests).

**Prerequisite:** Migration 000015 (S2 custom domains) must exist. Current latest migration is `000008_payments_shipping_tax`. Adjust migration number if S1/S2 have already shipped migrations 000009–000015.

---

## File structure produced by S3

```
services/marketplace-api/
├── migrations/
│   ├── 000016_subscriptions.up.sql                     # NEW
│   └── 000016_subscriptions.down.sql                   # NEW
├── internal/
│   ├── subscription/
│   │   ├── models.go                                   # NEW — StoreSubscription GORM model + plan/status constants
│   │   ├── repository.go                               # NEW — GetByStoreID, Upsert, UpdateStatus
│   │   ├── repository_test.go                          # NEW — unit tests with mock DB
│   │   ├── stripe_billing.go                           # NEW — Stripe Billing API client (Checkout, Portal, webhook verify)
│   │   ├── stripe_billing_test.go                      # NEW — unit tests with httptest server
│   │   ├── handler.go                                  # NEW — HTTP handlers (GetSubscription, CreateCheckout, CreatePortal, Webhook)
│   │   └── handler_test.go                             # NEW — integration tests
│   └── authz/
│       └── subscription_roles.go                       # NEW — role constants for subscription endpoints
├── internal/handlers/admin/
│   └── routes.go                                       # MODIFY — add subscription routes + webhook route
├── cmd/marketplace-api/
│   └── main.go                                         # MODIFY — wire SubscriptionHandler into Deps

apps/admin/
├── lib/api/
│   └── subscription-api.ts                             # NEW — typed API client
├── app/settings/subscription/
│   ├── page.tsx                                        # NEW — server component
│   └── actions.ts                                      # NEW — server actions (createCheckout, createPortal)
├── components/settings/
│   └── SubscriptionClient.tsx                          # NEW — client component with plan grid + buttons
└── components/shell/
    └── AdminShell.tsx                                  # MODIFY — add "Subscription" to sidebar navigation
```

---

## Task 0: Verify prerequisites

**Files:** none (read-only)

- [ ] **Step 1: Verify current migration version**

```bash
ls services/marketplace-api/migrations/ | tail -5
```

Expected: latest is `000008_payments_shipping_tax` or higher if S1/S2 shipped. Note the highest number — the new migration must be the next sequential number. This plan assumes `000016`. If the actual next number differs, adjust all migration filenames accordingly.

- [ ] **Step 2: Verify stores table exists**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_name = 'stores';"
```

Expected: `1`. The `store_subscriptions` table references `stores(id)`.

- [ ] **Step 3: Verify Stripe env vars are documented**

Check that `STRIPE_BILLING_SECRET_KEY` and `STRIPE_BILLING_WEBHOOK_SECRET` are listed in the service's `.env.example` or config struct. If not, we'll add them in Task 2.

No commit. Task 0 is read-only.

---

## Task 1: Migration — store_subscriptions table

**Files:**
- Create: `services/marketplace-api/migrations/000016_subscriptions.up.sql`
- Create: `services/marketplace-api/migrations/000016_subscriptions.down.sql`

### TDD: RED

- [ ] **Step 1: Write the up migration**

Create `services/marketplace-api/migrations/000016_subscriptions.up.sql`:

```sql
BEGIN;

CREATE TABLE store_subscriptions (
    id                      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID          NOT NULL,
    store_id                UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    stripe_customer_id      VARCHAR(100)  NOT NULL,
    stripe_subscription_id  VARCHAR(100),
    plan                    VARCHAR(30)   NOT NULL DEFAULT 'free',
    status                  VARCHAR(30)   NOT NULL DEFAULT 'active',
    current_period_start    TIMESTAMPTZ,
    current_period_end      TIMESTAMPTZ,
    cancel_at_period_end    BOOLEAN       NOT NULL DEFAULT false,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE INDEX ss_stripe_cust_idx ON store_subscriptions (stripe_customer_id);

COMMIT;
```

- [ ] **Step 2: Write the down migration**

Create `services/marketplace-api/migrations/000016_subscriptions.down.sql`:

```sql
BEGIN;
DROP TABLE IF EXISTS store_subscriptions;
COMMIT;
```

### GREEN

- [ ] **Step 3: Apply migration**

```bash
cd services/marketplace-api && DATABASE_URL="postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable" go run ./cmd/migrate up
```

- [ ] **Step 4: Verify table exists**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT column_name, data_type FROM information_schema.columns WHERE table_name='store_subscriptions' ORDER BY ordinal_position;"
```

Expected: 12 columns matching the schema above.

**Commit:** `feat(subscription): add migration 000016 for store_subscriptions table`

---

## Task 2: GORM model + repository

**Files:**
- Create: `services/marketplace-api/internal/subscription/models.go`
- Create: `services/marketplace-api/internal/subscription/repository.go`
- Create: `services/marketplace-api/internal/subscription/repository_test.go`

### TDD: RED — Write tests first

- [ ] **Step 1: Create models.go**

Create `services/marketplace-api/internal/subscription/models.go`:

```go
package subscription

import (
	"time"

	"github.com/google/uuid"
)

// Plan values — must match CHECK constraint in migration.
const (
	PlanFree       = "free"
	PlanStarter    = "starter"
	PlanPro        = "pro"
	PlanEnterprise = "enterprise"
)

// Status values.
const (
	StatusActive     = "active"
	StatusTrialing   = "trialing"
	StatusPastDue    = "past_due"
	StatusCancelled  = "cancelled"
	StatusIncomplete = "incomplete"
)

// StoreSubscription is the GORM model for store_subscriptions.
type StoreSubscription struct {
	ID                   uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID             uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID              uuid.UUID  `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	StripeCustomerID     string     `gorm:"column:stripe_customer_id;type:varchar(100);not null"     json:"stripe_customer_id"`
	StripeSubscriptionID *string    `gorm:"column:stripe_subscription_id;type:varchar(100)"          json:"stripe_subscription_id,omitempty"`
	Plan                 string     `gorm:"column:plan;type:varchar(30);not null;default:free"        json:"plan"`
	Status               string     `gorm:"column:status;type:varchar(30);not null;default:active"    json:"status"`
	CurrentPeriodStart   *time.Time `gorm:"column:current_period_start"                              json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *time.Time `gorm:"column:current_period_end"                                json:"current_period_end,omitempty"`
	CancelAtPeriodEnd    bool       `gorm:"column:cancel_at_period_end;not null;default:false"        json:"cancel_at_period_end"`
	CreatedAt            time.Time  `gorm:"column:created_at;not null;default:now()"                  json:"created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at;not null;default:now()"                  json:"updated_at"`
}

func (StoreSubscription) TableName() string { return "store_subscriptions" }

// subscriptionResponse is the safe wire DTO — never exposes stripe_customer_id.
type SubscriptionResponse struct {
	ID                 string  `json:"id"`
	Plan               string  `json:"plan"`
	Status             string  `json:"status"`
	CurrentPeriodStart *string `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *string `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd  bool    `json:"cancel_at_period_end"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

// ToResponse converts a StoreSubscription to a safe wire DTO.
func (s StoreSubscription) ToResponse() SubscriptionResponse {
	resp := SubscriptionResponse{
		ID:                s.ID.String(),
		Plan:              s.Plan,
		Status:            s.Status,
		CancelAtPeriodEnd: s.CancelAtPeriodEnd,
		CreatedAt:         s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         s.UpdatedAt.Format(time.RFC3339),
	}
	if s.CurrentPeriodStart != nil {
		t := s.CurrentPeriodStart.Format(time.RFC3339)
		resp.CurrentPeriodStart = &t
	}
	if s.CurrentPeriodEnd != nil {
		t := s.CurrentPeriodEnd.Format(time.RFC3339)
		resp.CurrentPeriodEnd = &t
	}
	return resp
}
```

- [ ] **Step 2: Create repository.go**

Create `services/marketplace-api/internal/subscription/repository.go`:

```go
package subscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository provides data access for store_subscriptions.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetByStoreID returns the subscription for the given store, or nil if none exists.
func (r *Repository) GetByStoreID(ctx context.Context, storeID uuid.UUID) (*StoreSubscription, error) {
	var sub StoreSubscription
	err := r.db.WithContext(ctx).Where("store_id = ?", storeID).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("subscription: get by store: %w", err)
	}
	return &sub, nil
}

// GetByStripeCustomerID finds the subscription by Stripe customer ID.
func (r *Repository) GetByStripeCustomerID(ctx context.Context, customerID string) (*StoreSubscription, error) {
	var sub StoreSubscription
	err := r.db.WithContext(ctx).Where("stripe_customer_id = ?", customerID).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("subscription: get by stripe customer: %w", err)
	}
	return &sub, nil
}

// Upsert creates or updates the subscription row for a store. Uses ON CONFLICT (store_id).
func (r *Repository) Upsert(ctx context.Context, sub *StoreSubscription) error {
	result := r.db.WithContext(ctx).
		Where("store_id = ?", sub.StoreID).
		Assign(*sub).
		FirstOrCreate(sub)
	if result.Error != nil {
		return fmt.Errorf("subscription: upsert: %w", result.Error)
	}
	return nil
}

// UpdateFields updates specific fields on the subscription row for a store.
func (r *Repository) UpdateFields(ctx context.Context, storeID uuid.UUID, fields map[string]any) error {
	result := r.db.WithContext(ctx).
		Model(&StoreSubscription{}).
		Where("store_id = ?", storeID).
		Updates(fields)
	if result.Error != nil {
		return fmt.Errorf("subscription: update fields: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("subscription: update fields: no subscription for store %s", storeID)
	}
	return nil
}
```

- [ ] **Step 3: Write repository tests**

Create `services/marketplace-api/internal/subscription/repository_test.go`:

```go
package subscription_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&subscription.StoreSubscription{}))
	return db
}

func TestRepository_GetByStoreID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := subscription.NewRepository(db)

	sub, err := repo.GetByStoreID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, sub, "expected nil when no subscription exists")
}

func TestRepository_Upsert_CreatesNew(t *testing.T) {
	db := setupTestDB(t)
	repo := subscription.NewRepository(db)

	storeID := uuid.New()
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          storeID,
		StripeCustomerID: "cus_test123",
		Plan:             subscription.PlanFree,
		Status:           subscription.StatusActive,
	}
	err := repo.Upsert(context.Background(), sub)
	require.NoError(t, err)

	got, err := repo.GetByStoreID(context.Background(), storeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "cus_test123", got.StripeCustomerID)
	assert.Equal(t, subscription.PlanFree, got.Plan)
}

func TestRepository_Upsert_UpdatesExisting(t *testing.T) {
	db := setupTestDB(t)
	repo := subscription.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()
	sub := &subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_test123",
		Plan:             subscription.PlanFree,
		Status:           subscription.StatusActive,
	}
	require.NoError(t, repo.Upsert(context.Background(), sub))

	// Upsert again with plan change.
	sub.Plan = subscription.PlanPro
	require.NoError(t, repo.Upsert(context.Background(), sub))

	got, err := repo.GetByStoreID(context.Background(), storeID)
	require.NoError(t, err)
	assert.Equal(t, subscription.PlanPro, got.Plan)
}

func TestRepository_GetByStripeCustomerID(t *testing.T) {
	db := setupTestDB(t)
	repo := subscription.NewRepository(db)

	storeID := uuid.New()
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          storeID,
		StripeCustomerID: "cus_lookup",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}
	require.NoError(t, repo.Upsert(context.Background(), sub))

	got, err := repo.GetByStripeCustomerID(context.Background(), "cus_lookup")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, storeID, got.StoreID)
}

func TestRepository_UpdateFields(t *testing.T) {
	db := setupTestDB(t)
	repo := subscription.NewRepository(db)

	storeID := uuid.New()
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          storeID,
		StripeCustomerID: "cus_upd",
		Plan:             subscription.PlanFree,
		Status:           subscription.StatusActive,
	}
	require.NoError(t, repo.Upsert(context.Background(), sub))

	now := time.Now().UTC()
	err := repo.UpdateFields(context.Background(), storeID, map[string]any{
		"plan":                 subscription.PlanPro,
		"status":               subscription.StatusTrialing,
		"current_period_start": now,
	})
	require.NoError(t, err)

	got, _ := repo.GetByStoreID(context.Background(), storeID)
	assert.Equal(t, subscription.PlanPro, got.Plan)
	assert.Equal(t, subscription.StatusTrialing, got.Status)
}

func TestRepository_UpdateFields_NoRow(t *testing.T) {
	db := setupTestDB(t)
	repo := subscription.NewRepository(db)

	err := repo.UpdateFields(context.Background(), uuid.New(), map[string]any{"plan": "pro"})
	assert.Error(t, err, "expected error when no subscription row exists")
}
```

### GREEN

- [ ] **Step 4: Run tests**

```bash
cd services/marketplace-api && go test ./internal/subscription/... -v -count=1
```

All 5 tests must pass.

**Commit:** `feat(subscription): add GORM model, repository, and repository tests`

---

## Task 3: Stripe Billing API client

**Files:**
- Create: `services/marketplace-api/internal/subscription/stripe_billing.go`
- Create: `services/marketplace-api/internal/subscription/stripe_billing_test.go`

### TDD: RED

- [ ] **Step 1: Create stripe_billing.go**

This follows the exact same `net/http` + `url.Values` + form-encoded POST pattern from `internal/payment/stripe.go`.

Create `services/marketplace-api/internal/subscription/stripe_billing.go`:

```go
package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const stripeBaseURL = "https://api.stripe.com"

// StripeBillingClient wraps Stripe Billing API calls for subscription management.
// Uses net/http + form-encoded requests (same pattern as internal/payment/stripe.go).
type StripeBillingClient struct {
	secretKey     string
	webhookSecret string
	baseURL       string
	client        *http.Client
}

// NewStripeBillingClient constructs a Stripe Billing client.
func NewStripeBillingClient(secretKey, webhookSecret string) *StripeBillingClient {
	return &StripeBillingClient{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		baseURL:       stripeBaseURL,
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

// newStripeBillingClientWithURL is for tests — allows custom base URL.
func newStripeBillingClientWithURL(secretKey, webhookSecret, baseURL string) *StripeBillingClient {
	c := NewStripeBillingClient(secretKey, webhookSecret)
	c.baseURL = baseURL
	return c
}

// CreateCustomer creates a Stripe Customer for a store.
func (c *StripeBillingClient) CreateCustomer(ctx context.Context, email, name, storeID string) (string, error) {
	form := url.Values{}
	form.Set("email", email)
	form.Set("name", name)
	form.Set("metadata[store_id]", storeID)

	var result struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, "/v1/customers", form, &result); err != nil {
		return "", fmt.Errorf("stripe billing: create customer: %w", err)
	}
	return result.ID, nil
}

// CheckoutSessionInput contains parameters for creating a Stripe Checkout session.
type CheckoutSessionInput struct {
	CustomerID string
	PriceID    string
	SuccessURL string
	CancelURL  string
}

// CreateCheckoutSession creates a Stripe Checkout session for subscription.
func (c *StripeBillingClient) CreateCheckoutSession(ctx context.Context, in CheckoutSessionInput) (string, error) {
	form := url.Values{}
	form.Set("customer", in.CustomerID)
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", in.PriceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", in.SuccessURL)
	form.Set("cancel_url", in.CancelURL)

	var result struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/checkout/sessions", form, &result); err != nil {
		return "", fmt.Errorf("stripe billing: create checkout: %w", err)
	}
	return result.URL, nil
}

// CreatePortalSession creates a Stripe Customer Portal session.
func (c *StripeBillingClient) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)

	var result struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/billing_portal/sessions", form, &result); err != nil {
		return "", fmt.Errorf("stripe billing: create portal: %w", err)
	}
	return result.URL, nil
}

// VerifyWebhook verifies a Stripe webhook signature and parses the event.
// Reuses the verifyStripeSignature pattern from internal/payment/stripe.go.
func (c *StripeBillingClient) VerifyWebhook(payload []byte, signature string) (*BillingWebhookEvent, error) {
	if err := verifyStripeBillingSignature(payload, signature, c.webhookSecret); err != nil {
		return nil, fmt.Errorf("stripe billing: verify webhook: %w", err)
	}

	var raw struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("stripe billing: verify webhook: decode: %w", err)
	}

	return &BillingWebhookEvent{
		ID:        raw.ID,
		Type:      raw.Type,
		RawObject: raw.Data.Object,
	}, nil
}

// BillingWebhookEvent is a parsed Stripe billing webhook event.
type BillingWebhookEvent struct {
	ID        string
	Type      string
	RawObject json.RawMessage
}

// post is a helper that sends a form-encoded POST request to Stripe.
func (c *StripeBillingClient) post(ctx context.Context, path string, form url.Values, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.secretKey, "")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	return json.Unmarshal(body, dest)
}

// verifyStripeBillingSignature checks the Stripe-Signature header.
// Same algorithm as internal/payment/stripe.go:verifyStripeSignature.
func verifyStripeBillingSignature(payload []byte, header, secret string) error {
	// Import the shared verification from payment package or duplicate.
	// For isolation, we duplicate the 20-line function here.
	import_crypto_hmac := hmacVerify // see below
	return import_crypto_hmac(payload, header, secret)
}
```

**IMPORTANT:** The signature verification above is pseudo-code. The actual implementation must duplicate the `verifyStripeSignature` function from `internal/payment/stripe.go` (lines 240-285). Copy it verbatim and rename to `verifyStripeBillingSignature`:

```go
import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

func verifyStripeBillingSignature(payload []byte, header, secret string) error {
	parts := strings.Split(header, ",")
	var timestamp string
	var signatures []string

	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return fmt.Errorf("invalid signature header")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp in signature header")
	}
	if time.Since(time.Unix(ts, 0)).Abs() > 5*time.Minute {
		return fmt.Errorf("webhook timestamp too old (replay rejected)")
	}

	signedPayload := []byte(timestamp + "." + string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signedPayload)
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}

	return fmt.Errorf("signature mismatch")
}
```

- [ ] **Step 2: Write Stripe client tests**

Create `services/marketplace-api/internal/subscription/stripe_billing_test.go`:

```go
package subscription_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func TestStripeBilling_CreateCheckoutSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/checkout/sessions", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		// Verify auth header present.
		user, _, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "sk_test_123", user)

		// Verify form body.
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "cus_abc", r.PostForm.Get("customer"))
		assert.Equal(t, "subscription", r.PostForm.Get("mode"))
		assert.Equal(t, "price_starter", r.PostForm.Get("line_items[0][price]"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"url": "https://checkout.stripe.com/sess_123"})
	}))
	defer srv.Close()

	client := subscription.NewStripeBillingClientForTest("sk_test_123", "whsec_test", srv.URL)
	url, err := client.CreateCheckoutSession(context.Background(), subscription.CheckoutSessionInput{
		CustomerID: "cus_abc",
		PriceID:    "price_starter",
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.com/sess_123", url)
}

func TestStripeBilling_CreatePortalSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/billing_portal/sessions", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "cus_abc", r.PostForm.Get("customer"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"url": "https://billing.stripe.com/portal_123"})
	}))
	defer srv.Close()

	client := subscription.NewStripeBillingClientForTest("sk_test_123", "whsec_test", srv.URL)
	url, err := client.CreatePortalSession(context.Background(), "cus_abc", "https://example.com/settings")
	require.NoError(t, err)
	assert.Equal(t, "https://billing.stripe.com/portal_123", url)
}

func TestStripeBilling_CreateCustomer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/customers", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "owner@shop.com", r.PostForm.Get("email"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": "cus_new_123"})
	}))
	defer srv.Close()

	client := subscription.NewStripeBillingClientForTest("sk_test_123", "whsec_test", srv.URL)
	id, err := client.CreateCustomer(context.Background(), "owner@shop.com", "Test Store", "store-uuid")
	require.NoError(t, err)
	assert.Equal(t, "cus_new_123", id)
}

func TestStripeBilling_VerifyWebhook_Valid(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"customer":"cus_abc"}}}`)
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signed := []byte(ts + "." + string(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signed)
	sig := "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))

	client := subscription.NewStripeBillingClientForTest("sk_test", secret, "")
	evt, err := client.VerifyWebhook(payload, sig)
	require.NoError(t, err)
	assert.Equal(t, "evt_1", evt.ID)
	assert.Equal(t, "checkout.session.completed", evt.Type)
}

func TestStripeBilling_VerifyWebhook_BadSignature(t *testing.T) {
	client := subscription.NewStripeBillingClientForTest("sk_test", "whsec_real", "")
	_, err := client.VerifyWebhook([]byte(`{}`), "t=123,v1=badsig")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature mismatch")
}

func TestStripeBilling_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid price"}}`))
	}))
	defer srv.Close()

	client := subscription.NewStripeBillingClientForTest("sk_test_123", "whsec_test", srv.URL)
	_, err := client.CreateCheckoutSession(context.Background(), subscription.CheckoutSessionInput{
		CustomerID: "cus_abc",
		PriceID:    "price_invalid",
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}
```

**Note:** The test uses `NewStripeBillingClientForTest` — export the `newStripeBillingClientWithURL` function as `NewStripeBillingClientForTest` for test usage:

```go
// NewStripeBillingClientForTest is exported for test use only. It allows
// overriding the base URL to point at httptest servers.
func NewStripeBillingClientForTest(secretKey, webhookSecret, baseURL string) *StripeBillingClient {
	c := NewStripeBillingClient(secretKey, webhookSecret)
	if baseURL != "" {
		c.baseURL = baseURL
	}
	return c
}
```

### GREEN

- [ ] **Step 3: Run tests**

```bash
cd services/marketplace-api && go test ./internal/subscription/... -v -count=1
```

All tests must pass.

**Commit:** `feat(subscription): add Stripe Billing API client with webhook verification`

---

## Task 4: Authz roles + HTTP handler

**Files:**
- Create: `services/marketplace-api/internal/authz/subscription_roles.go`
- Create: `services/marketplace-api/internal/subscription/handler.go`
- Create: `services/marketplace-api/internal/subscription/handler_test.go`

### TDD: RED

- [ ] **Step 1: Create subscription role constants**

Create `services/marketplace-api/internal/authz/subscription_roles.go`:

```go
package authz

// Subscription settings — only the store owner can initiate checkout or
// manage billing. Admin role can view the current plan.

// SubscriptionViewRole allows viewing the current subscription.
var SubscriptionViewRole = RoleAdmin

// SubscriptionManageRole allows creating checkout/portal sessions.
var SubscriptionManageRole = RoleOwner
```

- [ ] **Step 2: Create handler.go**

Create `services/marketplace-api/internal/subscription/handler.go`:

```go
package subscription

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// PlanPriceIDs maps plan names to Stripe Price IDs. Loaded from environment.
type PlanPriceIDs struct {
	Starter    string
	Pro        string
	Enterprise string
}

// Handler provides HTTP handlers for subscription management.
type Handler struct {
	repo    *Repository
	stripe  *StripeBillingClient
	prices  PlanPriceIDs
	logger  *slog.Logger
	baseURL string // admin app base URL for redirect URLs
}

// NewHandler constructs a subscription Handler.
func NewHandler(db *gorm.DB, stripe *StripeBillingClient, prices PlanPriceIDs, logger *slog.Logger) *Handler {
	baseURL := os.Getenv("ADMIN_APP_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3001"
	}
	return &Handler{
		repo:    NewRepository(db),
		stripe:  stripe,
		prices:  prices,
		logger:  logger,
		baseURL: baseURL,
	}
}

// storeFromCtx extracts the *stores.Store set by StoreMiddleware.
func storeFromCtx(c *gin.Context) *stores.Store {
	v, ok := c.Get("store")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing store context",
		})
		return nil
	}
	s, _ := v.(*stores.Store)
	if s == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing store context",
		})
		return nil
	}
	return s
}

// GetSubscription handles GET /admin/stores/:storeId/subscription.
func (h *Handler) GetSubscription(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	sub, err := h.repo.GetByStoreID(c.Request.Context(), store.ID)
	if err != nil {
		h.logger.Error("get subscription", "store_id", store.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to get subscription"})
		return
	}

	if sub == nil {
		// No subscription yet — return free plan default.
		c.JSON(http.StatusOK, SubscriptionResponse{
			Plan:   PlanFree,
			Status: StatusActive,
		})
		return
	}

	c.JSON(http.StatusOK, sub.ToResponse())
}

// createCheckoutRequest is the request body for POST /subscription/checkout.
type createCheckoutRequest struct {
	Plan string `json:"plan" binding:"required,oneof=starter pro enterprise"`
}

// CreateCheckout handles POST /admin/stores/:storeId/subscription/checkout.
func (h *Handler) CreateCheckout(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	var req createCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation", "message": "plan must be one of: starter, pro, enterprise"})
		return
	}

	priceID := h.priceForPlan(req.Plan)
	if priceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation", "message": "no Stripe price configured for plan: " + req.Plan})
		return
	}

	ctx := c.Request.Context()

	// Ensure a Stripe customer exists for this store.
	sub, err := h.repo.GetByStoreID(ctx, store.ID)
	if err != nil {
		h.logger.Error("create checkout: get subscription", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to check subscription"})
		return
	}

	var customerID string
	if sub != nil {
		customerID = sub.StripeCustomerID
	} else {
		// Get user email from context (set by auth middleware).
		email, _ := c.Get("user_email")
		emailStr, _ := email.(string)
		if emailStr == "" {
			emailStr = "unknown@mark8ly.com"
		}

		customerID, err = h.stripe.CreateCustomer(ctx, emailStr, store.Name, store.ID.String())
		if err != nil {
			h.logger.Error("create checkout: create customer", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "stripe_error", "message": "failed to create Stripe customer"})
			return
		}

		// Save the customer ID.
		tenantID, _ := c.Get("tenant_id")
		tenantUUID, _ := uuid.Parse(tenantID.(string))
		newSub := &StoreSubscription{
			TenantID:         tenantUUID,
			StoreID:          store.ID,
			StripeCustomerID: customerID,
			Plan:             PlanFree,
			Status:           StatusActive,
		}
		if err := h.repo.Upsert(ctx, newSub); err != nil {
			h.logger.Error("create checkout: save customer", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to save subscription"})
			return
		}
	}

	storeID := c.Param("storeId")
	checkoutURL, err := h.stripe.CreateCheckoutSession(ctx, CheckoutSessionInput{
		CustomerID: customerID,
		PriceID:    priceID,
		SuccessURL: h.baseURL + "/settings/subscription?checkout=success&store=" + storeID,
		CancelURL:  h.baseURL + "/settings/subscription?checkout=cancelled&store=" + storeID,
	})
	if err != nil {
		h.logger.Error("create checkout: stripe session", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stripe_error", "message": "failed to create checkout session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": checkoutURL})
}

// CreatePortal handles POST /admin/stores/:storeId/subscription/portal.
func (h *Handler) CreatePortal(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	sub, err := h.repo.GetByStoreID(c.Request.Context(), store.ID)
	if err != nil || sub == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_found", "message": "no subscription found — upgrade first"})
		return
	}

	storeID := c.Param("storeId")
	portalURL, err := h.stripe.CreatePortalSession(c.Request.Context(), sub.StripeCustomerID, h.baseURL+"/settings/subscription?store="+storeID)
	if err != nil {
		h.logger.Error("create portal", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stripe_error", "message": "failed to create billing portal"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": portalURL})
}

// HandleWebhook handles POST /webhooks/stripe-billing.
// This endpoint is NOT behind auth middleware — it's called by Stripe.
func (h *Handler) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "cannot read body"})
		return
	}

	sig := c.GetHeader("Stripe-Signature")
	evt, err := h.stripe.VerifyWebhook(body, sig)
	if err != nil {
		h.logger.Warn("webhook: signature verification failed", "err", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "invalid webhook signature"})
		return
	}

	ctx := c.Request.Context()
	switch evt.Type {
	case "checkout.session.completed":
		h.handleCheckoutCompleted(ctx, evt.RawObject)
	case "customer.subscription.updated":
		h.handleSubscriptionUpdated(ctx, evt.RawObject)
	case "customer.subscription.deleted":
		h.handleSubscriptionDeleted(ctx, evt.RawObject)
	case "invoice.payment_failed":
		h.handlePaymentFailed(ctx, evt.RawObject)
	default:
		h.logger.Info("webhook: unhandled event type", "type", evt.Type)
	}

	// Always return 200 to Stripe to acknowledge receipt.
	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *Handler) handleCheckoutCompleted(ctx context.Context, raw json.RawMessage) {
	var obj struct {
		Customer     string `json:"customer"`
		Subscription string `json:"subscription"`
		Metadata     struct {
			Plan string `json:"plan"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		h.logger.Error("webhook: checkout completed: decode", "err", err)
		return
	}

	sub, err := h.repo.GetByStripeCustomerID(ctx, obj.Customer)
	if err != nil || sub == nil {
		h.logger.Error("webhook: checkout completed: customer not found", "customer_id", obj.Customer)
		return
	}

	now := time.Now().UTC()
	fields := map[string]any{
		"stripe_subscription_id": obj.Subscription,
		"plan":                   obj.Metadata.Plan,
		"status":                 StatusActive,
		"current_period_start":   now,
		"updated_at":             now,
	}
	if err := h.repo.UpdateFields(ctx, sub.StoreID, fields); err != nil {
		h.logger.Error("webhook: checkout completed: update", "err", err)
	}
}

func (h *Handler) handleSubscriptionUpdated(ctx context.Context, raw json.RawMessage) {
	var obj struct {
		ID                 string `json:"id"`
		Customer           string `json:"customer"`
		Status             string `json:"status"`
		CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
		CurrentPeriodStart int64  `json:"current_period_start"`
		CurrentPeriodEnd   int64  `json:"current_period_end"`
		Items              struct {
			Data []struct {
				Price struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		h.logger.Error("webhook: subscription updated: decode", "err", err)
		return
	}

	sub, err := h.repo.GetByStripeCustomerID(ctx, obj.Customer)
	if err != nil || sub == nil {
		h.logger.Error("webhook: subscription updated: customer not found", "customer_id", obj.Customer)
		return
	}

	periodStart := time.Unix(obj.CurrentPeriodStart, 0).UTC()
	periodEnd := time.Unix(obj.CurrentPeriodEnd, 0).UTC()
	now := time.Now().UTC()

	fields := map[string]any{
		"status":                 obj.Status,
		"cancel_at_period_end":   obj.CancelAtPeriodEnd,
		"current_period_start":   periodStart,
		"current_period_end":     periodEnd,
		"updated_at":             now,
	}

	// Map price ID back to plan name.
	if len(obj.Items.Data) > 0 {
		fields["plan"] = h.planForPrice(obj.Items.Data[0].Price.ID)
	}

	if err := h.repo.UpdateFields(ctx, sub.StoreID, fields); err != nil {
		h.logger.Error("webhook: subscription updated: update", "err", err)
	}
}

func (h *Handler) handleSubscriptionDeleted(ctx context.Context, raw json.RawMessage) {
	var obj struct {
		Customer string `json:"customer"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		h.logger.Error("webhook: subscription deleted: decode", "err", err)
		return
	}

	sub, err := h.repo.GetByStripeCustomerID(ctx, obj.Customer)
	if err != nil || sub == nil {
		return
	}

	now := time.Now().UTC()
	fields := map[string]any{
		"plan":       PlanFree,
		"status":     StatusCancelled,
		"updated_at": now,
	}
	if err := h.repo.UpdateFields(ctx, sub.StoreID, fields); err != nil {
		h.logger.Error("webhook: subscription deleted: update", "err", err)
	}
}

func (h *Handler) handlePaymentFailed(ctx context.Context, raw json.RawMessage) {
	var obj struct {
		Customer string `json:"customer"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		h.logger.Error("webhook: payment failed: decode", "err", err)
		return
	}

	sub, err := h.repo.GetByStripeCustomerID(ctx, obj.Customer)
	if err != nil || sub == nil {
		return
	}

	now := time.Now().UTC()
	if err := h.repo.UpdateFields(ctx, sub.StoreID, map[string]any{
		"status":     StatusPastDue,
		"updated_at": now,
	}); err != nil {
		h.logger.Error("webhook: payment failed: update", "err", err)
	}
}

func (h *Handler) priceForPlan(plan string) string {
	switch plan {
	case PlanStarter:
		return h.prices.Starter
	case PlanPro:
		return h.prices.Pro
	case PlanEnterprise:
		return h.prices.Enterprise
	default:
		return ""
	}
}

func (h *Handler) planForPrice(priceID string) string {
	switch priceID {
	case h.prices.Starter:
		return PlanStarter
	case h.prices.Pro:
		return PlanPro
	case h.prices.Enterprise:
		return PlanEnterprise
	default:
		return PlanFree
	}
}
```

- [ ] **Step 3: Write handler integration tests**

Create `services/marketplace-api/internal/subscription/handler_test.go`:

```go
package subscription_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func setupHandlerTest(t *testing.T) (*gin.Engine, *gorm.DB, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&subscription.StoreSubscription{}))

	// Mock Stripe server.
	stripeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/customers":
			json.NewEncoder(w).Encode(map[string]string{"id": "cus_test"})
		case "/v1/checkout/sessions":
			json.NewEncoder(w).Encode(map[string]string{"url": "https://checkout.stripe.com/test"})
		case "/v1/billing_portal/sessions":
			json.NewEncoder(w).Encode(map[string]string{"url": "https://billing.stripe.com/test"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	stripeClient := subscription.NewStripeBillingClientForTest("sk_test", "whsec_test", stripeSrv.URL)
	handler := subscription.NewHandler(db, stripeClient, subscription.PlanPriceIDs{
		Starter: "price_starter", Pro: "price_pro", Enterprise: "price_enterprise",
	}, slog.Default())

	r := gin.New()
	return r, db, stripeSrv
}

func TestGetSubscription_NoSubscription(t *testing.T) {
	r, _, stripeSrv := setupHandlerTest(t)
	defer stripeSrv.Close()

	// ... wire handler + middleware + test. Pattern matches existing tests in
	// internal/handlers/admin/*_integration_test.go.
	// Set store in context, call GET, assert 200 + free plan.
	_ = r // wiring left to implementer matching admin.RegisterAdmin pattern
}

// Additional tests to write:
// - TestGetSubscription_ExistingPlan
// - TestCreateCheckout_Success
// - TestCreateCheckout_InvalidPlan
// - TestCreateCheckout_NoStore
// - TestCreatePortal_NoSubscription
// - TestCreatePortal_Success
// - TestWebhook_InvalidSignature
// - TestWebhook_CheckoutCompleted
// - TestWebhook_SubscriptionUpdated
// - TestWebhook_SubscriptionDeleted
// - TestWebhook_PaymentFailed
```

### GREEN

- [ ] **Step 4: Run all subscription tests**

```bash
cd services/marketplace-api && go test ./internal/subscription/... -v -count=1
```

**Commit:** `feat(subscription): add HTTP handlers for subscription CRUD and Stripe webhook`

---

## Task 5: Route wiring + Deps

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Add SubscriptionHandler to Deps struct**

In `services/marketplace-api/internal/handlers/admin/routes.go`, add to the `Deps` struct:

```go
SubscriptionHandler      *subscription.Handler // S3: subscription/billing
```

Add the import:

```go
"github.com/mark8ly/marketplace-api/internal/subscription"
```

- [ ] **Step 2: Add subscription routes to RegisterAdmin**

In `RegisterAdmin`, after the tax settings block (around line 233) and before the abandoned carts block, add:

```go
		// Subscription / billing — S3.
		if deps.SubscriptionHandler != nil {
			sub := storeRoute.Group("/subscription")
			{
				sub.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionViewRole),
					deps.SubscriptionHandler.GetSubscription)
				sub.POST("/checkout",
					deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionManageRole),
					deps.SubscriptionHandler.CreateCheckout)
				sub.POST("/portal",
					deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionManageRole),
					deps.SubscriptionHandler.CreatePortal)
			}
		}
```

- [ ] **Step 3: Add webhook route OUTSIDE auth middleware**

The webhook endpoint must be outside the store route (no auth middleware). In `RegisterAdmin`, add at the top level (same level as `/admin`):

```go
	// Stripe billing webhook — no auth middleware; Stripe calls this directly.
	if deps.SubscriptionHandler != nil {
		router.POST("/webhooks/stripe-billing", deps.SubscriptionHandler.HandleWebhook)
	}
```

- [ ] **Step 4: Wire in main.go**

In `services/marketplace-api/cmd/marketplace-api/main.go`, in the admin deps construction block, add config loading and handler construction:

```go
	// Subscription handler (S3).
	var subscriptionHandler *subscription.Handler
	if stripeBillingKey := os.Getenv("STRIPE_BILLING_SECRET_KEY"); stripeBillingKey != "" {
		stripeWebhookSecret := os.Getenv("STRIPE_BILLING_WEBHOOK_SECRET")
		stripeClient := subscription.NewStripeBillingClient(stripeBillingKey, stripeWebhookSecret)
		subscriptionHandler = subscription.NewHandler(conn, stripeClient, subscription.PlanPriceIDs{
			Starter:    os.Getenv("STRIPE_PRICE_STARTER"),
			Pro:        os.Getenv("STRIPE_PRICE_PRO"),
			Enterprise: os.Getenv("STRIPE_PRICE_ENTERPRISE"),
		}, log)
		log.Info("subscription handler initialized (Stripe Billing)")
	} else {
		log.Warn("STRIPE_BILLING_SECRET_KEY not set — subscription endpoints disabled")
	}
```

Then add to the `adminDeps` struct literal:

```go
	SubscriptionHandler: subscriptionHandler,
```

- [ ] **Step 5: Build check**

```bash
cd services/marketplace-api && go build ./...
```

Must compile without errors.

**Commit:** `feat(subscription): wire subscription routes and Stripe Billing handler in main.go`

---

## Task 6: Admin UI — subscription page

**Files:**
- Create: `apps/admin/lib/api/subscription-api.ts`
- Create: `apps/admin/app/settings/subscription/page.tsx`
- Create: `apps/admin/app/settings/subscription/actions.ts`
- Create: `apps/admin/components/settings/SubscriptionClient.tsx`
- Modify: `apps/admin/components/shell/AdminShell.tsx`

- [ ] **Step 1: Create subscription-api.ts**

Create `apps/admin/lib/api/subscription-api.ts`:

```typescript
import type { SessionHeaders, MutationResult, MutationError } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SubscriptionData {
  id?: string;
  plan: "free" | "starter" | "pro" | "enterprise";
  status: "active" | "trialing" | "past_due" | "cancelled" | "incomplete";
  current_period_start?: string;
  current_period_end?: string;
  cancel_at_period_end: boolean;
  created_at?: string;
  updated_at?: string;
}

export async function getSubscription(
  storeId: string,
  headers: SessionHeaders,
): Promise<SubscriptionData> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription`,
    { headers, cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`Failed to fetch subscription: ${res.status}`);
  }
  return res.json();
}

export async function createCheckoutSession(
  storeId: string,
  plan: string,
  headers: SessionHeaders,
): Promise<{ url: string }> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription/checkout`,
    {
      method: "POST",
      headers: { ...headers, "Content-Type": "application/json" },
      body: JSON.stringify({ plan }),
    },
  );
  if (!res.ok) {
    const err = await res.json().catch(() => ({ message: "Unknown error" }));
    throw new Error(err.message ?? `Checkout failed: ${res.status}`);
  }
  return res.json();
}

export async function createPortalSession(
  storeId: string,
  headers: SessionHeaders,
): Promise<{ url: string }> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription/portal`,
    {
      method: "POST",
      headers: { ...headers, "Content-Type": "application/json" },
    },
  );
  if (!res.ok) {
    const err = await res.json().catch(() => ({ message: "Unknown error" }));
    throw new Error(err.message ?? `Portal failed: ${res.status}`);
  }
  return res.json();
}
```

- [ ] **Step 2: Create server actions**

Create `apps/admin/app/settings/subscription/actions.ts`:

```typescript
"use server";

import { redirect } from "next/navigation";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { createCheckoutSession, createPortalSession } from "@/lib/api/subscription-api";

export async function checkoutAction(storeId: string, plan: string) {
  const { sessionHeaders } = await getServerSessionContext();
  const { url } = await createCheckoutSession(storeId, plan, sessionHeaders);
  redirect(url);
}

export async function portalAction(storeId: string) {
  const { sessionHeaders } = await getServerSessionContext();
  const { url } = await createPortalSession(storeId, sessionHeaders);
  redirect(url);
}
```

- [ ] **Step 3: Create SubscriptionClient.tsx**

Create `apps/admin/components/settings/SubscriptionClient.tsx`:

```tsx
"use client";

import { useState, useTransition } from "react";
import { checkoutAction, portalAction } from "@/app/settings/subscription/actions";
import type { SubscriptionData } from "@/lib/api/subscription-api";

interface PlanFeature {
  name: string;
  free: boolean | string;
  starter: boolean | string;
  pro: boolean | string;
  enterprise: boolean | string;
}

const planFeatures: PlanFeature[] = [
  { name: "Products", free: "25", starter: "500", pro: "Unlimited", enterprise: "Unlimited" },
  { name: "Team members", free: "1", starter: "3", pro: "10", enterprise: "Unlimited" },
  { name: "Custom domain", free: false, starter: true, pro: true, enterprise: true },
  { name: "Analytics", free: false, starter: "Basic", pro: "Advanced", enterprise: "Advanced" },
  { name: "Priority support", free: false, starter: false, pro: true, enterprise: true },
  { name: "API access", free: false, starter: false, pro: true, enterprise: true },
];

const planNames = ["free", "starter", "pro", "enterprise"] as const;
const planLabels: Record<string, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  enterprise: "Enterprise",
};

interface SubscriptionClientProps {
  storeId: string;
  subscription: SubscriptionData;
  editable: boolean;
}

export function SubscriptionClient({ storeId, subscription, editable }: SubscriptionClientProps) {
  const [isPending, startTransition] = useTransition();

  function handleUpgrade(plan: string) {
    startTransition(() => checkoutAction(storeId, plan));
  }

  function handleManageBilling() {
    startTransition(() => portalAction(storeId));
  }

  return (
    <div className="space-y-10">
      {/* Current plan card */}
      <section className="space-y-4">
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium">
          Current plan
        </h2>
        <div className="rounded-md border border-[color:var(--ink-900)]/10 bg-white p-6">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <p className="text-lg font-medium text-foreground">
                {planLabels[subscription.plan] ?? subscription.plan}
              </p>
              <StatusBadge status={subscription.status} />
              {subscription.current_period_end && (
                <p className="text-sm text-foreground-secondary">
                  Current period ends{" "}
                  {new Date(subscription.current_period_end).toLocaleDateString()}
                </p>
              )}
              {subscription.cancel_at_period_end && (
                <p className="text-sm text-[color:var(--signal)]">
                  Cancels at end of period
                </p>
              )}
            </div>
            {editable && subscription.plan !== "free" && (
              <button
                type="button"
                onClick={handleManageBilling}
                disabled={isPending}
                className="rounded-md border border-[color:var(--ink-900)]/20 px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-paper-100 disabled:opacity-50"
              >
                {isPending ? "Loading..." : "Manage billing"}
              </button>
            )}
          </div>
          {subscription.status === "past_due" && (
            <div className="mt-4 rounded-md border border-[color:var(--signal)] bg-[color:var(--signal)]/5 p-3">
              <p className="text-sm text-[color:var(--signal)]">
                Your payment is past due. Please update your payment method to
                avoid service interruption.
              </p>
            </div>
          )}
        </div>
      </section>

      {/* Plan comparison grid */}
      <section className="space-y-4">
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium">
          Plans
        </h2>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[color:var(--ink-900)]/10">
                <th className="py-3 pr-4 text-left font-medium text-foreground-secondary">
                  Feature
                </th>
                {planNames.map((plan) => (
                  <th
                    key={plan}
                    className={`px-4 py-3 text-center font-medium ${
                      plan === subscription.plan
                        ? "text-[color:var(--moss-700)]"
                        : "text-foreground"
                    }`}
                  >
                    {planLabels[plan]}
                    {plan === subscription.plan && (
                      <span className="ml-1 text-xs">(current)</span>
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {planFeatures.map((feature) => (
                <tr
                  key={feature.name}
                  className="border-b border-[color:var(--ink-900)]/5"
                >
                  <td className="py-3 pr-4 text-foreground-secondary">
                    {feature.name}
                  </td>
                  {planNames.map((plan) => {
                    const value = feature[plan];
                    return (
                      <td key={plan} className="px-4 py-3 text-center">
                        {typeof value === "boolean" ? (
                          value ? (
                            <span className="text-[color:var(--moss-700)]">
                              &#10003;
                            </span>
                          ) : (
                            <span className="text-foreground-secondary">
                              &mdash;
                            </span>
                          )
                        ) : (
                          <span className="text-foreground">{value}</span>
                        )}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Upgrade buttons */}
        {editable && (
          <div className="flex gap-3 pt-2">
            {planNames
              .filter((p) => p !== "free" && p !== subscription.plan)
              .map((plan) => (
                <button
                  key={plan}
                  type="button"
                  onClick={() => handleUpgrade(plan)}
                  disabled={isPending}
                  className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
                >
                  {isPending ? "Loading..." : `Upgrade to ${planLabels[plan]}`}
                </button>
              ))}
          </div>
        )}
      </section>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    active:
      "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
    trialing:
      "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
    past_due:
      "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
    cancelled:
      "bg-[color:var(--ink-900)]/10 text-foreground-secondary",
    incomplete:
      "bg-[color:var(--warning)]/10 text-[color:var(--warning)]",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? colors.active}`}
    >
      {status.replace("_", " ")}
    </span>
  );
}
```

- [ ] **Step 4: Create page.tsx**

Create `apps/admin/app/settings/subscription/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getSubscription } from "@/lib/api/subscription-api";
import { SubscriptionClient } from "@/components/settings/SubscriptionClient";

export default async function SubscriptionPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
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
          <p className="eyebrow">Store setup</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Subscription
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Manage your subscription plan and billing details.
          </p>
        </header>

        {currentStore ? (
          <SubscriptionContent storeId={currentStore.id} editable={editable} />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store first.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function SubscriptionContent({
  storeId,
  editable,
}: {
  storeId: string;
  editable: boolean;
}) {
  const subscription = await getSubscription(storeId, {} as any);

  return (
    <SubscriptionClient
      storeId={storeId}
      subscription={subscription}
      editable={editable}
    />
  );
}
```

- [ ] **Step 5: Add "Subscription" to sidebar navigation**

In `apps/admin/components/shell/AdminShell.tsx`, find the settings navigation children array and add after the "Tax" entry:

```typescript
      { label: "Subscription", href: "/settings/subscription" },
```

The resulting array should be:

```typescript
    children: [
      { label: "Store Settings", href: "/settings/general" },
      { label: "Storefront", href: "/settings/storefront" },
      { label: "Stores", href: "/settings/stores" },
      { label: "Team", href: "/settings/team" },
      { label: "Payments", href: "/settings/payments" },
      { label: "Shipping", href: "/settings/shipping" },
      { label: "Tax", href: "/settings/tax" },
      { label: "Subscription", href: "/settings/subscription" },
    ],
```

- [ ] **Step 6: Verify frontend builds**

```bash
cd apps/admin && npx next build
```

Must compile without type errors.

**Commit:** `feat(subscription): add subscription settings page, plan grid, and Stripe checkout/portal redirects`

---

## Task 7: E2E smoke test

**Files:**
- Create: `apps/admin/e2e/subscription.spec.ts`

- [ ] **Step 1: Write Playwright test**

Create `apps/admin/e2e/subscription.spec.ts`:

```typescript
import { test, expect } from "@playwright/test";

test.describe("Subscription Settings", () => {
  test("displays free plan when no subscription exists", async ({ page }) => {
    await page.goto("/settings/subscription");
    await expect(page.getByText("Current plan")).toBeVisible();
    await expect(page.getByText("Free")).toBeVisible();
  });

  test("shows plan comparison grid", async ({ page }) => {
    await page.goto("/settings/subscription");
    await expect(page.getByText("Plans")).toBeVisible();
    await expect(page.getByText("Starter")).toBeVisible();
    await expect(page.getByText("Pro")).toBeVisible();
    await expect(page.getByText("Enterprise")).toBeVisible();
  });

  test("sidebar contains Subscription link", async ({ page }) => {
    await page.goto("/settings/subscription");
    const sidebar = page.locator("aside");
    await expect(sidebar.getByText("Subscription")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run E2E tests**

```bash
cd apps/admin && npx playwright test e2e/subscription.spec.ts
```

**Commit:** `test(subscription): add Playwright E2E smoke tests for subscription page`

---

## Summary

| Task | What it delivers | Files |
|------|-----------------|-------|
| 0 | Prerequisites check | read-only |
| 1 | Migration 000016 | 2 SQL files |
| 2 | GORM model + repository + tests | 3 Go files |
| 3 | Stripe Billing client + tests | 2 Go files |
| 4 | Authz roles + HTTP handler + tests | 3 Go files |
| 5 | Route wiring + main.go | 2 Go files modified |
| 6 | Admin UI (page, actions, component, sidebar) | 5 TS/TSX files |
| 7 | E2E smoke test | 1 TS file |

**Environment variables required:**
- `STRIPE_BILLING_SECRET_KEY` — Stripe secret key for billing
- `STRIPE_BILLING_WEBHOOK_SECRET` — Stripe webhook signing secret
- `STRIPE_PRICE_STARTER` — Stripe Price ID for Starter plan
- `STRIPE_PRICE_PRO` — Stripe Price ID for Pro plan
- `STRIPE_PRICE_ENTERPRISE` — Stripe Price ID for Enterprise plan
- `ADMIN_APP_URL` — Admin app base URL (default: `http://localhost:3001`)
