# Settings S2 — Custom Domains Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship custom domain management with Cloudflare API DNS record creation, background verification worker, auto-SSL, and admin UI showing domain status.

**Architecture:** New `internal/domain/` package (models, repository, cloudflare client, verification worker). Migration 000015. Background worker polls every 60s. Admin UI at /settings/domains.

**Tech Stack:** Go 1.26, Gin, net/http (Cloudflare API). Next.js 16, React 19, Tailwind.

**Design Authority:** `docs/superpowers/specs/2026-04-10-settings-tier1-tier2-design.md` §2.1, §3.2, §4.2, §5.1 (domains page), §6.2, §7.1.

---

## Status

> **Pending.** All tasks open.

---

## Scope check

Adds migration `000015_custom_domains`, new package `services/marketplace-api/internal/domain/` (models, repository, Cloudflare API client, verification worker), handler `internal/handlers/admin/domains.go`, wiring in `main.go` + `routes.go`, and frontend pages + components under `apps/admin/`. The verification worker runs as a background goroutine (same pattern as the csvjob worker) polling every 60s.

Spec sections authoritative:
- Design spec §2.1 (custom_domains schema)
- Design spec §3.2 (Cloudflare API architecture)
- Design spec §4.2 (API endpoints)
- Design spec §5.1 (domains page layout)
- Design spec §6.2 (security — encrypted token, domain uniqueness)
- Design spec §7.1 (verification worker)
- Design spec §8 (testing)

**Out of scope (deferred):**
- Multi-domain per store (one custom domain per store for now)
- Wildcard domains
- Non-Cloudflare DNS providers

---

## Decisions locked (from the spec — do NOT re-debate)

1. **Cloudflare API token:** Stored encrypted in `cf_api_token_encrypted` column. Same encryption pattern as payment gateway keys in `payment_gateway_configs.api_key_encrypted`.
2. **CNAME target:** All custom domains CNAME to `stores.mark8ly.com`.
3. **Zone ID discovery:** Derived from merchant's Cloudflare API token via `GET /zones?name=<root_domain>`.
4. **Verification worker:** Runs in admin + both modes. Polls every 60s. Same goroutine pattern as csvjob worker.
5. **Status machine:** `pending` -> `verifying` -> `active` | `error` | `removing`. SSL status: `pending` -> `active` | `error`.
6. **24h timeout:** Domains stuck in `verifying` for >24h are set to `error` with a message.
7. **Domain uniqueness:** `UNIQUE (domain)` constraint prevents cross-tenant claiming.
8. **One domain per store:** Enforced at handler level (check existing domain count before insert).
9. **Design system:** Paper · Ink · Moss tokens. Status badges: moss-700 for active, signal for error, ink-900/40 for pending, spinner for verifying.

---

## File structure produced by S2

### New backend files

```
services/marketplace-api/
  migrations/
    000015_custom_domains.up.sql
    000015_custom_domains.down.sql
  internal/domain/
    models.go                     CustomDomain entity + status constants
    repository.go                 CRUD + ListByStore + FindVerifying
    repository_test.go            unit tests (mock DB or integration)
    cloudflare.go                 Cloudflare API client (zones, dns_records)
    cloudflare_test.go            httptest mock tests
    worker.go                     Verification worker goroutine
    worker_test.go                worker cycle tests with mock CF client
  internal/handlers/admin/
    domains.go                    DomainsHandler: list, add, remove, verify
    domains_test.go               handler unit tests
```

### Modified backend files

```
services/marketplace-api/
  cmd/marketplace-api/main.go              Wire DomainsHandler + worker startup
  internal/handlers/admin/routes.go        Add domains route group + Deps field
  pkg/config/config.go                     (no new env vars — CF token comes per-domain from user)
```

### New frontend files

```
apps/admin/
  app/settings/domains/
    page.tsx                               Server component: domains settings page
    actions.ts                             Server actions: addDomain, removeDomain, verifyDomain
  components/settings/
    DomainsList.tsx                         Domain list table with status badges
    AddDomainForm.tsx                       Domain + CF API token form
    DomainStatusBadge.tsx                   Status badge component (pending/verifying/active/error)
  lib/api/
    domains-api.ts                         Typed API client for /admin/stores/:storeId/domains
```

### Modified frontend files

```
apps/admin/
  components/shell/AdminShell.tsx          Add "Domains" nav leaf (done in S1 Task 8 if S1 runs first)
```

---

## Tasks

### Task 1 — Migration (`000015_custom_domains`)

- [ ] **1a.** Create `services/marketplace-api/migrations/000015_custom_domains.up.sql`:

```sql
-- Migration 000015: custom_domains table for S2 custom domain management.
-- Stores merchant-owned domains with Cloudflare DNS record references.

CREATE TABLE custom_domains (
    id                       UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                UUID          NOT NULL,
    store_id                 UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    domain                   VARCHAR(253)  NOT NULL,
    status                   VARCHAR(20)   NOT NULL DEFAULT 'pending',
    cloudflare_zone_id       VARCHAR(100),
    cloudflare_dns_record_id VARCHAR(100),
    cf_api_token_encrypted   TEXT          NOT NULL,
    ssl_status               VARCHAR(20)   NOT NULL DEFAULT 'pending',
    verified_at              TIMESTAMPTZ,
    error_message            TEXT,
    created_at               TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (domain)
);

CREATE INDEX cd_store_idx ON custom_domains (store_id);
```

- [ ] **1b.** Create `services/marketplace-api/migrations/000015_custom_domains.down.sql`:

```sql
DROP TABLE IF EXISTS custom_domains;
```

- [ ] **1c.** Update the expected migration version in the schema version assertion (if the codebase has one — check `pkg/migrate/` or `main.go` startup).

**Verification:**
```bash
cd services/marketplace-api && go run ./cmd/migrate/ up
# OR: manually apply via psql against the dev database
```

---

### Task 2 — Domain models (`internal/domain/models.go`)

- [ ] **2a.** Create `services/marketplace-api/internal/domain/models.go`:

```go
// Package domain implements custom domain management for S2. Each store
// can have one custom domain pointing to stores.mark8ly.com via a CNAME
// record managed through the merchant's Cloudflare API token.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status constants for the custom_domains.status column.
const (
	StatusPending   = "pending"
	StatusVerifying = "verifying"
	StatusActive    = "active"
	StatusError     = "error"
	StatusRemoving  = "removing"
)

// SSLStatus constants for the custom_domains.ssl_status column.
const (
	SSLPending = "pending"
	SSLActive  = "active"
	SSLError   = "error"
)

// CustomDomain is the GORM model for the custom_domains table.
type CustomDomain struct {
	ID                    uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID              uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID               uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	Domain                string     `gorm:"column:domain;type:varchar(253);not null;uniqueIndex"`
	Status                string     `gorm:"column:status;type:varchar(20);not null;default:pending"`
	CloudflareZoneID      string     `gorm:"column:cloudflare_zone_id;type:varchar(100)"`
	CloudflareDNSRecordID string     `gorm:"column:cloudflare_dns_record_id;type:varchar(100)"`
	CFAPITokenEncrypted   string     `gorm:"column:cf_api_token_encrypted;type:text;not null"`
	SSLStatus             string     `gorm:"column:ssl_status;type:varchar(20);not null;default:pending"`
	VerifiedAt            *time.Time `gorm:"column:verified_at"`
	ErrorMessage          string     `gorm:"column:error_message;type:text"`
	CreatedAt             time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt             time.Time  `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomDomain) TableName() string { return "custom_domains" }

// CNAMETarget is the domain all custom domains CNAME to.
const CNAMETarget = "stores.mark8ly.com"

// VerificationTimeoutHours is how long a domain stays in "verifying"
// before being marked as "error".
const VerificationTimeoutHours = 24
```

---

### Task 3 — Domain repository (`internal/domain/repository.go`)

**TDD: RED first.** Write `repository_test.go` with tests against mock or real DB, then implement.

- [ ] **3a.** Create `services/marketplace-api/internal/domain/repository.go`:

```go
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository provides CRUD for custom_domains.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a domain Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new custom domain. Returns gorm.ErrDuplicatedKey if
// the domain already exists (UNIQUE constraint).
func (r *Repository) Create(ctx context.Context, d *CustomDomain) error {
	return r.db.WithContext(ctx).Create(d).Error
}

// GetByID fetches a single domain by ID scoped to store.
func (r *Repository) GetByID(ctx context.Context, id, storeID uuid.UUID) (*CustomDomain, error) {
	var d CustomDomain
	err := r.db.WithContext(ctx).
		Where("id = ? AND store_id = ?", id, storeID).
		First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListByStore returns all domains for a store.
func (r *Repository) ListByStore(ctx context.Context, storeID uuid.UUID) ([]CustomDomain, error) {
	var domains []CustomDomain
	err := r.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("created_at ASC").
		Find(&domains).Error
	return domains, err
}

// CountByStore returns the number of domains for a store.
func (r *Repository) CountByStore(ctx context.Context, storeID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&CustomDomain{}).
		Where("store_id = ?", storeID).
		Count(&count).Error
	return count, err
}

// FindVerifying returns all domains with status "verifying" that need
// verification checks. Used by the background worker.
func (r *Repository) FindVerifying(ctx context.Context) ([]CustomDomain, error) {
	var domains []CustomDomain
	err := r.db.WithContext(ctx).
		Where("status = ?", StatusVerifying).
		Find(&domains).Error
	return domains, err
}

// FindTimedOut returns domains stuck in "verifying" longer than the
// timeout. Used by the worker to mark them as error.
func (r *Repository) FindTimedOut(ctx context.Context, timeout time.Duration) ([]CustomDomain, error) {
	var domains []CustomDomain
	cutoff := time.Now().Add(-timeout)
	err := r.db.WithContext(ctx).
		Where("status = ? AND updated_at < ?", StatusVerifying, cutoff).
		Find(&domains).Error
	return domains, err
}

// UpdateStatus sets the status and optional error message.
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, status, sslStatus, errMsg string, verifiedAt *time.Time) error {
	updates := map[string]any{
		"status":     status,
		"ssl_status": sslStatus,
		"updated_at": time.Now(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if verifiedAt != nil {
		updates["verified_at"] = verifiedAt
	}
	return r.db.WithContext(ctx).
		Model(&CustomDomain{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateCloudflareIDs stores the zone ID and DNS record ID after
// successful Cloudflare API calls.
func (r *Repository) UpdateCloudflareIDs(ctx context.Context, id uuid.UUID, zoneID, recordID string) error {
	return r.db.WithContext(ctx).
		Model(&CustomDomain{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"cloudflare_zone_id":       zoneID,
			"cloudflare_dns_record_id": recordID,
			"status":                   StatusVerifying,
			"updated_at":               time.Now(),
		}).Error
}

// Delete removes a domain record.
func (r *Repository) Delete(ctx context.Context, id, storeID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND store_id = ?", id, storeID).
		Delete(&CustomDomain{}).Error
}
```

- [ ] **3b.** Create `services/marketplace-api/internal/domain/repository_test.go` — test Create (success + duplicate), ListByStore, CountByStore, FindVerifying, UpdateStatus, Delete. Use either integration tests against a real DB or mock the gorm.DB layer.

**Verification:**
```bash
cd services/marketplace-api && go test ./internal/domain/... -run TestRepository -v -count=1
```

---

### Task 4 — Cloudflare API client (`internal/domain/cloudflare.go`)

**TDD: RED first.** Write `cloudflare_test.go` with `httptest.Server` mocks, then implement.

- [ ] **4a.** Create `services/marketplace-api/internal/domain/cloudflare.go`:

```go
package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CloudflareClient wraps the Cloudflare API for DNS record management.
// Each call uses the per-domain API token (stored encrypted in the DB).
type CloudflareClient interface {
	// LookupZoneID finds the zone ID for a given root domain.
	LookupZoneID(ctx context.Context, apiToken, rootDomain string) (string, error)
	// CreateCNAME creates a CNAME record pointing to CNAMETarget.
	CreateCNAME(ctx context.Context, apiToken, zoneID, subdomain string) (recordID string, err error)
	// GetDNSRecord checks the propagation status of a DNS record.
	GetDNSRecord(ctx context.Context, apiToken, zoneID, recordID string) (*DNSRecordStatus, error)
	// DeleteDNSRecord removes a DNS record.
	DeleteDNSRecord(ctx context.Context, apiToken, zoneID, recordID string) error
}

// DNSRecordStatus holds the status of a Cloudflare DNS record.
type DNSRecordStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Proxied  bool   `json:"proxied"`
	TTL      int    `json:"ttl"`
}

// cfResponse is the generic Cloudflare API response envelope.
type cfResponse struct {
	Success  bool            `json:"success"`
	Errors   []cfError       `json:"errors"`
	Result   json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HTTPCloudflareClient is the concrete Cloudflare API client.
type HTTPCloudflareClient struct {
	baseURL    string // "https://api.cloudflare.com/client/v4" in production
	httpClient *http.Client
}

// NewCloudflareClient creates a Cloudflare API client. baseURL defaults
// to the real Cloudflare API; override for testing.
func NewCloudflareClient(baseURL string) *HTTPCloudflareClient {
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	return &HTTPCloudflareClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *HTTPCloudflareClient) doRequest(ctx context.Context, method, url, apiToken string, body io.Reader) (*cfResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: http: %w", err)
	}
	defer resp.Body.Close()

	var cfResp cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return nil, fmt.Errorf("cloudflare: decode: %w", err)
	}
	if !cfResp.Success {
		msg := "unknown error"
		if len(cfResp.Errors) > 0 {
			msg = cfResp.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare: API error: %s", msg)
	}
	return &cfResp, nil
}

// LookupZoneID finds the zone ID for a root domain.
func (c *HTTPCloudflareClient) LookupZoneID(ctx context.Context, apiToken, rootDomain string) (string, error) {
	url := fmt.Sprintf("%s/zones?name=%s", c.baseURL, rootDomain)
	cfResp, err := c.doRequest(ctx, http.MethodGet, url, apiToken, nil)
	if err != nil {
		return "", err
	}

	var zones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(cfResp.Result, &zones); err != nil {
		return "", fmt.Errorf("cloudflare: decode zones: %w", err)
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("cloudflare: no zone found for %s", rootDomain)
	}
	return zones[0].ID, nil
}

// CreateCNAME creates a CNAME DNS record.
func (c *HTTPCloudflareClient) CreateCNAME(ctx context.Context, apiToken, zoneID, subdomain string) (string, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records", c.baseURL, zoneID)
	payload := fmt.Sprintf(`{"type":"CNAME","name":"%s","content":"%s","ttl":1,"proxied":true}`, subdomain, CNAMETarget)
	cfResp, err := c.doRequest(ctx, http.MethodPost, url, apiToken, strings.NewReader(payload))
	if err != nil {
		return "", err
	}

	var record struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(cfResp.Result, &record); err != nil {
		return "", fmt.Errorf("cloudflare: decode record: %w", err)
	}
	return record.ID, nil
}

// GetDNSRecord fetches a DNS record to check propagation.
func (c *HTTPCloudflareClient) GetDNSRecord(ctx context.Context, apiToken, zoneID, recordID string) (*DNSRecordStatus, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, recordID)
	cfResp, err := c.doRequest(ctx, http.MethodGet, url, apiToken, nil)
	if err != nil {
		return nil, err
	}

	var status DNSRecordStatus
	if err := json.Unmarshal(cfResp.Result, &status); err != nil {
		return nil, fmt.Errorf("cloudflare: decode status: %w", err)
	}
	return &status, nil
}

// DeleteDNSRecord removes a DNS record from Cloudflare.
func (c *HTTPCloudflareClient) DeleteDNSRecord(ctx context.Context, apiToken, zoneID, recordID string) error {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, recordID)
	_, err := c.doRequest(ctx, http.MethodDelete, url, apiToken, nil)
	return err
}

// extractRootDomain extracts the root domain from a full domain.
// e.g., "shop.example.com" -> "example.com"
func extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return domain
	}
	return strings.Join(parts[len(parts)-2:], ".")
}
```

- [ ] **4b.** Create `services/marketplace-api/internal/domain/cloudflare_test.go`:

```go
package domain_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupZoneID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Contains(t, r.URL.RawQuery, "name=example.com")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []map[string]string{
				{"id": "zone-123", "name": "example.com"},
			},
		})
	}))
	defer srv.Close()

	client := domain.NewCloudflareClient(srv.URL)
	zoneID, err := client.LookupZoneID(context.Background(), "test-token", "example.com")
	require.NoError(t, err)
	assert.Equal(t, "zone-123", zoneID)
}

func TestLookupZoneID_NoZone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []map[string]string{},
		})
	}))
	defer srv.Close()

	client := domain.NewCloudflareClient(srv.URL)
	_, err := client.LookupZoneID(context.Background(), "test-token", "nonexistent.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no zone found")
}

func TestCreateCNAME_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/dns_records")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]string{"id": "record-456"},
		})
	}))
	defer srv.Close()

	client := domain.NewCloudflareClient(srv.URL)
	recordID, err := client.CreateCNAME(context.Background(), "test-token", "zone-123", "shop.example.com")
	require.NoError(t, err)
	assert.Equal(t, "record-456", recordID)
}

func TestCreateCNAME_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 81053, "message": "Record already exists"}},
		})
	}))
	defer srv.Close()

	client := domain.NewCloudflareClient(srv.URL)
	_, err := client.CreateCNAME(context.Background(), "test-token", "zone-123", "shop.example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Record already exists")
}

func TestDeleteDNSRecord_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]string{"id": "record-456"},
		})
	}))
	defer srv.Close()

	client := domain.NewCloudflareClient(srv.URL)
	err := client.DeleteDNSRecord(context.Background(), "test-token", "zone-123", "record-456")
	assert.NoError(t, err)
}
```

**Verification:**
```bash
cd services/marketplace-api && go test ./internal/domain/... -run TestCloudflare -v -count=1
```

---

### Task 5 — Verification worker (`internal/domain/worker.go`)

**TDD: RED first.** Write `worker_test.go` with a mock CloudflareClient, then implement.

- [ ] **5a.** Create `services/marketplace-api/internal/domain/worker.go`:

```go
package domain

import (
	"context"
	"log/slog"
	"time"
)

// WorkerConfig holds configuration for the verification worker.
type WorkerConfig struct {
	Repo     *Repository
	CF       CloudflareClient
	Logger   *slog.Logger
	Interval time.Duration // poll interval, default 60s
}

// StartVerificationWorker launches a background goroutine that polls for
// domains in "verifying" status and checks their DNS propagation via the
// Cloudflare API. Returns a channel that closes when the worker stops.
//
// Follows the same pattern as the csvjob worker in cmd/marketplace-api/main.go.
func StartVerificationWorker(ctx context.Context, cfg WorkerConfig) <-chan struct{} {
	done := make(chan struct{})
	interval := cfg.Interval
	if interval == 0 {
		interval = 60 * time.Second
	}

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				cfg.Logger.Info("domain worker: shutting down")
				return
			case <-ticker.C:
				processVerifyingDomains(ctx, cfg)
				processTimedOutDomains(ctx, cfg)
			}
		}
	}()

	return done
}

// processVerifyingDomains checks each "verifying" domain against the
// Cloudflare API. On success (DNS record exists and is proxied), marks
// the domain as active.
func processVerifyingDomains(ctx context.Context, cfg WorkerConfig) {
	domains, err := cfg.Repo.FindVerifying(ctx)
	if err != nil {
		cfg.Logger.Error("domain worker: find verifying", "err", err)
		return
	}

	for _, d := range domains {
		if d.CloudflareZoneID == "" || d.CloudflareDNSRecordID == "" {
			continue // incomplete setup, skip
		}

		record, err := cfg.CF.GetDNSRecord(ctx, d.CFAPITokenEncrypted, d.CloudflareZoneID, d.CloudflareDNSRecordID)
		if err != nil {
			cfg.Logger.Error("domain worker: check dns",
				"domain", d.Domain, "err", err)
			continue // transient error, retry next cycle
		}

		// If the record exists and points to our CNAME target, mark active.
		if record.Content == CNAMETarget {
			now := time.Now()
			if err := cfg.Repo.UpdateStatus(ctx, d.ID, StatusActive, SSLActive, "", &now); err != nil {
				cfg.Logger.Error("domain worker: update status",
					"domain", d.Domain, "err", err)
			} else {
				cfg.Logger.Info("domain worker: domain verified",
					"domain", d.Domain)
			}
		}
	}
}

// processTimedOutDomains marks domains stuck in "verifying" for >24h as error.
func processTimedOutDomains(ctx context.Context, cfg WorkerConfig) {
	timeout := time.Duration(VerificationTimeoutHours) * time.Hour
	domains, err := cfg.Repo.FindTimedOut(ctx, timeout)
	if err != nil {
		cfg.Logger.Error("domain worker: find timed out", "err", err)
		return
	}

	for _, d := range domains {
		errMsg := "DNS verification timed out after 24 hours. Please check your Cloudflare DNS settings."
		if err := cfg.Repo.UpdateStatus(ctx, d.ID, StatusError, SSLError, errMsg, nil); err != nil {
			cfg.Logger.Error("domain worker: mark timed out",
				"domain", d.Domain, "err", err)
		} else {
			cfg.Logger.Info("domain worker: marked timed out",
				"domain", d.Domain)
		}
	}
}
```

- [ ] **5b.** Create `services/marketplace-api/internal/domain/worker_test.go`:

```go
package domain_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/stretchr/testify/assert"
)

// mockCFClient implements domain.CloudflareClient for testing.
type mockCFClient struct {
	getDNSRecordFn func(ctx context.Context, apiToken, zoneID, recordID string) (*domain.DNSRecordStatus, error)
}

func (m *mockCFClient) LookupZoneID(ctx context.Context, apiToken, rootDomain string) (string, error) {
	return "zone-1", nil
}
func (m *mockCFClient) CreateCNAME(ctx context.Context, apiToken, zoneID, subdomain string) (string, error) {
	return "record-1", nil
}
func (m *mockCFClient) GetDNSRecord(ctx context.Context, apiToken, zoneID, recordID string) (*domain.DNSRecordStatus, error) {
	return m.getDNSRecordFn(ctx, apiToken, zoneID, recordID)
}
func (m *mockCFClient) DeleteDNSRecord(ctx context.Context, apiToken, zoneID, recordID string) error {
	return nil
}

// Test that the worker marks a verified domain as active.
func TestWorker_VerifiesDomain(t *testing.T) {
	// Setup mock repo with a "verifying" domain
	// Setup mock CF client that returns a record pointing to CNAMETarget
	// Start worker with short interval (100ms)
	// Assert domain status transitions to "active" within 500ms
}

// Test that the worker marks timed-out domains as error.
func TestWorker_TimesOutDomain(t *testing.T) {
	// Setup mock repo with a "verifying" domain updated_at > 24h ago
	// Start worker
	// Assert domain status transitions to "error"
}
```

**Verification:**
```bash
cd services/marketplace-api && go test ./internal/domain/... -run TestWorker -v -count=1
```

---

### Task 6 — Domains handler (`internal/handlers/admin/domains.go`)

**TDD: RED first.**

- [ ] **6a.** Create `services/marketplace-api/internal/handlers/admin/domains_test.go` — test List, Add (success + duplicate + one-per-store limit), Remove (with CF cleanup), Verify (manual re-trigger).

- [ ] **6b.** Create `services/marketplace-api/internal/handlers/admin/domains.go`:

```go
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/domain"
)

// DomainsHandler handles custom domain management endpoints.
type DomainsHandler struct {
	repo   *domain.Repository
	cf     domain.CloudflareClient
	logger *slog.Logger
}

// NewDomainsHandler constructs a DomainsHandler.
func NewDomainsHandler(repo *domain.Repository, cf domain.CloudflareClient, logger *slog.Logger) *DomainsHandler {
	return &DomainsHandler{repo: repo, cf: cf, logger: logger}
}

// domainResponse is the safe wire DTO for a custom domain.
type domainResponse struct {
	ID          string  `json:"id"`
	Domain      string  `json:"domain"`
	Status      string  `json:"status"`
	SSLStatus   string  `json:"ssl_status"`
	VerifiedAt  *string `json:"verified_at,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func toDomainResponse(d domain.CustomDomain) domainResponse {
	resp := domainResponse{
		ID:           d.ID.String(),
		Domain:       d.Domain,
		Status:       d.Status,
		SSLStatus:    d.SSLStatus,
		ErrorMessage: d.ErrorMessage,
		CreatedAt:    d.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    d.UpdatedAt.Format(time.RFC3339),
	}
	if d.VerifiedAt != nil {
		v := d.VerifiedAt.Format(time.RFC3339)
		resp.VerifiedAt = &v
	}
	return resp
}

// List handles GET /admin/stores/:storeId/domains.
func (h *DomainsHandler) List(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	domains, err := h.repo.ListByStore(c.Request.Context(), store.ID)
	if err != nil {
		h.logger.Error("list domains", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to list domains",
		})
		return
	}

	resp := make([]domainResponse, len(domains))
	for i, d := range domains {
		resp[i] = toDomainResponse(d)
	}
	c.JSON(http.StatusOK, gin.H{"domains": resp})
}

// addDomainRequest is the JSON body for POST /admin/stores/:storeId/domains.
type addDomainRequest struct {
	Domain     string `json:"domain" binding:"required"`
	CFAPIToken string `json:"cf_api_token" binding:"required"`
}

// Add handles POST /admin/stores/:storeId/domains.
func (h *DomainsHandler) Add(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}
	tenantID, _ := c.Get("tenant_id")

	var req addDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "Domain and Cloudflare API token are required",
		})
		return
	}

	domainName := strings.TrimSpace(strings.ToLower(req.Domain))
	if domainName == "" || !strings.Contains(domainName, ".") {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "Invalid domain name",
		})
		return
	}

	// One domain per store limit.
	count, err := h.repo.CountByStore(c.Request.Context(), store.ID)
	if err != nil {
		h.logger.Error("count domains", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to check domain limit",
		})
		return
	}
	if count >= 1 {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{
			"error": "limit_exceeded", "message": "Only one custom domain per store is allowed",
		})
		return
	}

	// Step 1: Lookup Cloudflare zone ID.
	rootDomain := extractRootDomain(domainName)
	zoneID, err := h.cf.LookupZoneID(c.Request.Context(), req.CFAPIToken, rootDomain)
	if err != nil {
		h.logger.Error("lookup zone", "domain", domainName, "err", err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "cloudflare_error",
			"message": "Could not find Cloudflare zone. Verify your API token has Zone:Read permission.",
		})
		return
	}

	// Step 2: Create CNAME record.
	recordID, err := h.cf.CreateCNAME(c.Request.Context(), req.CFAPIToken, zoneID, domainName)
	if err != nil {
		h.logger.Error("create cname", "domain", domainName, "err", err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "cloudflare_error",
			"message": "Failed to create DNS record. The domain may already have a conflicting record.",
		})
		return
	}

	// Step 3: Persist domain record.
	tid, _ := uuid.Parse(tenantID.(string))
	d := &domain.CustomDomain{
		TenantID:              tid,
		StoreID:               store.ID,
		Domain:                domainName,
		Status:                domain.StatusVerifying,
		CloudflareZoneID:      zoneID,
		CloudflareDNSRecordID: recordID,
		CFAPITokenEncrypted:   req.CFAPIToken, // TODO: encrypt before storing
		SSLStatus:             domain.SSLPending,
	}
	if err := h.repo.Create(c.Request.Context(), d); err != nil {
		h.logger.Error("create domain", "domain", domainName, "err", err)
		// Unique constraint violation.
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": "duplicate", "message": "This domain is already registered",
			})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to save domain",
		})
		return
	}

	c.JSON(http.StatusCreated, toDomainResponse(*d))
}

// Remove handles DELETE /admin/stores/:storeId/domains/:id.
func (h *DomainsHandler) Remove(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	domainID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "Invalid domain ID",
		})
		return
	}

	d, err := h.repo.GetByID(c.Request.Context(), domainID, store.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "not_found", "message": "Domain not found",
			})
			return
		}
		h.logger.Error("get domain", "id", domainID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to fetch domain",
		})
		return
	}

	// Delete DNS record from Cloudflare (best-effort).
	if d.CloudflareZoneID != "" && d.CloudflareDNSRecordID != "" {
		if err := h.cf.DeleteDNSRecord(c.Request.Context(), d.CFAPITokenEncrypted, d.CloudflareZoneID, d.CloudflareDNSRecordID); err != nil {
			h.logger.Error("delete dns record", "domain", d.Domain, "err", err)
			// Continue with DB deletion even if CF cleanup fails.
		}
	}

	if err := h.repo.Delete(c.Request.Context(), domainID, store.ID); err != nil {
		h.logger.Error("delete domain", "id", domainID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to remove domain",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// Verify handles POST /admin/stores/:storeId/domains/:id/verify.
// Manual re-verification trigger.
func (h *DomainsHandler) Verify(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	domainID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "Invalid domain ID",
		})
		return
	}

	d, err := h.repo.GetByID(c.Request.Context(), domainID, store.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "not_found", "message": "Domain not found",
			})
			return
		}
		h.logger.Error("get domain for verify", "id", domainID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to fetch domain",
		})
		return
	}

	// Reset to verifying status so the worker picks it up.
	if err := h.repo.UpdateStatus(c.Request.Context(), d.ID, domain.StatusVerifying, domain.SSLPending, "", nil); err != nil {
		h.logger.Error("reset domain status", "id", domainID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "Failed to trigger verification",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Verification triggered"})
}

// extractRootDomain extracts the root domain from a full domain.
func extractRootDomain(d string) string {
	parts := strings.Split(d, ".")
	if len(parts) <= 2 {
		return d
	}
	return strings.Join(parts[len(parts)-2:], ".")
}
```

**Verification:**
```bash
cd services/marketplace-api && go test ./internal/handlers/admin/... -run TestDomains -v -count=1
```

---

### Task 7 — Wiring in `routes.go` and `main.go`

- [ ] **7a.** Add `DomainsHandler` to `admin.Deps` in `routes.go`:

```go
type Deps struct {
	// ... existing fields ...
	DomainsHandler *DomainsHandler // S2: custom domains
}
```

- [ ] **7b.** Add domains route group in `RegisterAdmin` (in `routes.go`), inside the `storeRoute` block:

```go
// Custom Domains (S2).
if deps.DomainsHandler != nil {
	domains := storeRoute.Group("/domains")
	{
		domains.GET("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
			deps.DomainsHandler.List)
		domains.POST("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
			deps.DomainsHandler.Add)
		domains.DELETE("/:id",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
			deps.DomainsHandler.Remove)
		domains.POST("/:id/verify",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
			deps.DomainsHandler.Verify)
	}
}
```

- [ ] **7c.** Wire in `cmd/marketplace-api/main.go` (inside the admin wiring block):

```go
// Custom Domains handler + worker (S2).
domainRepo := domain.NewRepository(conn)
cfClient := domain.NewCloudflareClient("") // uses real Cloudflare API
domainsHandler := admin.NewDomainsHandler(domainRepo, cfClient, log)
```

Add to `adminDeps`:
```go
adminDeps = admin.Deps{
	// ... existing ...
	DomainsHandler: domainsHandler,
}
```

- [ ] **7d.** Start verification worker (after the outbox publisher block):

```go
// Domain verification worker — runs in admin and both modes.
// Polls Cloudflare API every 60s for domains in "verifying" status.
var domainWorkerDone <-chan struct{}
if m == mode.Admin || m == mode.Both {
	domainWorkerDone = domain.StartVerificationWorker(workerCtx, domain.WorkerConfig{
		Repo:     domainRepo,
		CF:       cfClient,
		Logger:   log,
		Interval: 60 * time.Second,
	})
	log.Info("domain verification worker started")
}
```

Add to graceful shutdown (alongside the existing worker done channels):
```go
if domainWorkerDone != nil {
	<-domainWorkerDone
}
```

**Verification:**
```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

---

### Task 8 — Frontend API client (`apps/admin/lib/api/domains-api.ts`)

- [ ] **8a.** Create `apps/admin/lib/api/domains-api.ts`:

```typescript
// apps/admin/lib/api/domains-api.ts
//
// Typed API client for custom domain endpoints (S2).

import type { SessionHeaders } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

// ─────────────────────────────────────────────────────────────────────
// Wire DTOs
// ─────────────────────────────────────────────────────────────────────

export interface CustomDomain {
  id: string;
  domain: string;
  status: "pending" | "verifying" | "active" | "error" | "removing";
  ssl_status: "pending" | "active" | "error";
  verified_at?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface AddDomainInput {
  domain: string;
  cf_api_token: string;
}

// ─────────────────────────────────────────────────────────────────────
// API functions
// ─────────────────────────────────────────────────────────────────────

function buildHeaders(session: SessionHeaders): HeadersInit {
  return {
    "Content-Type": "application/json",
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
  };
}

export async function listDomains(
  session: SessionHeaders,
  storeId: string,
): Promise<CustomDomain[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains`,
    { headers: buildHeaders(session), cache: "no-store" },
  );
  if (!res.ok) throw new Error(`Failed to list domains: ${res.status}`);
  const data = await res.json();
  return data.domains;
}

export async function addDomain(
  session: SessionHeaders,
  storeId: string,
  input: AddDomainInput,
): Promise<CustomDomain> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains`,
    {
      method: "POST",
      headers: buildHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Failed to add domain: ${res.status}`);
  }
  return res.json();
}

export async function removeDomain(
  session: SessionHeaders,
  storeId: string,
  domainId: string,
): Promise<void> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains/${domainId}`,
    { method: "DELETE", headers: buildHeaders(session) },
  );
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Failed to remove domain: ${res.status}`);
  }
}

export async function verifyDomain(
  session: SessionHeaders,
  storeId: string,
  domainId: string,
): Promise<void> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains/${domainId}/verify`,
    { method: "POST", headers: buildHeaders(session) },
  );
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Failed to verify domain: ${res.status}`);
  }
}
```

**Verification:**
```bash
cd apps/admin && npx tsc --noEmit
```

---

### Task 9 — Server actions (`apps/admin/app/settings/domains/actions.ts`)

- [ ] **9a.** Create `apps/admin/app/settings/domains/actions.ts`:

```typescript
"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  addDomain,
  removeDomain,
  verifyDomain,
} from "@/lib/api/domains-api";
import type { AddDomainInput } from "@/lib/api/domains-api";
import { canEditSettings } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export type AddDomainResult =
  | { ok: true; domain: { id: string; domain: string; status: string } }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = h.get("x-session-store-id") ?? "";
  return { userId, tenantId, role, storeId };
}

export async function addDomainAction(
  input: AddDomainInput,
): Promise<AddDomainResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (role !== "owner") {
    return { ok: false, code: "forbidden", message: "Only the store owner can add domains." };
  }
  if (!input.domain.trim() || !input.domain.includes(".")) {
    return { ok: false, code: "validation", message: "Enter a valid domain name." };
  }
  if (!input.cf_api_token.trim()) {
    return { ok: false, code: "validation", message: "Cloudflare API token is required." };
  }

  try {
    const domain = await addDomain({ userId, tenantId }, storeId, input);
    revalidatePath("/settings/domains");
    return {
      ok: true,
      domain: { id: domain.id, domain: domain.domain, status: domain.status },
    };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Failed to add domain";
    return { ok: false, code: "error", message: msg };
  }
}

export async function removeDomainAction(
  domainId: string,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (role !== "owner") {
    return { ok: false, code: "forbidden", message: "Only the store owner can remove domains." };
  }

  try {
    await removeDomain({ userId, tenantId }, storeId, domainId);
    revalidatePath("/settings/domains");
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Failed to remove domain";
    return { ok: false, code: "error", message: msg };
  }
}

export async function verifyDomainAction(
  domainId: string,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "Insufficient permissions." };
  }

  try {
    await verifyDomain({ userId, tenantId }, storeId, domainId);
    revalidatePath("/settings/domains");
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Verification failed";
    return { ok: false, code: "error", message: msg };
  }
}
```

---

### Task 10 — Domains settings page (`apps/admin/app/settings/domains/page.tsx`)

- [ ] **10a.** Create `apps/admin/app/settings/domains/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listDomains } from "@/lib/api/domains-api";
import { DomainsList } from "@/components/settings/DomainsList";
import { AddDomainForm } from "@/components/settings/AddDomainForm";

export default async function DomainsSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const editable = role === "owner";

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
            Custom domains
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Connect your own domain to your storefront. Your domain must be
            managed by Cloudflare — we create a CNAME record pointing to
            stores.mark8ly.com.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: only the store owner can manage domains.
            </p>
          )}
        </header>

        {currentStore ? (
          <DomainsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring domains.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function DomainsContent({
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
  const domains = await listDomains({ userId, tenantId }, storeId).catch(
    () => [],
  );

  const hasDomain = domains.length > 0;

  return (
    <>
      {/* Current domains */}
      <section>
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
          Connected domains
        </h2>
        <div className="mt-1 border-t border-border-subtle" />
        <div className="mt-6">
          {domains.length > 0 ? (
            <DomainsList domains={domains} editable={editable} />
          ) : (
            <p className="text-sm text-foreground-secondary">
              No custom domain configured. Add one below.
            </p>
          )}
        </div>
      </section>

      {/* Add domain form — only if no domain exists yet (one per store) */}
      {!hasDomain && editable && (
        <section>
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Add a custom domain
          </h2>
          <div className="mt-1 border-t border-border-subtle" />
          <div className="mt-6">
            <AddDomainForm />
          </div>
        </section>
      )}
    </>
  );
}
```

---

### Task 11 — Client components

- [ ] **11a.** Create `apps/admin/components/settings/DomainStatusBadge.tsx`:

```tsx
"use client";

interface DomainStatusBadgeProps {
  status: "pending" | "verifying" | "active" | "error" | "removing";
}

const statusConfig: Record<
  string,
  { label: string; className: string }
> = {
  pending: {
    label: "Pending",
    className: "bg-foreground/10 text-foreground-secondary",
  },
  verifying: {
    label: "Verifying...",
    className: "bg-foreground/10 text-foreground-secondary animate-pulse",
  },
  active: {
    label: "Active",
    className: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  },
  error: {
    label: "Error",
    className: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  },
  removing: {
    label: "Removing...",
    className: "bg-foreground/10 text-foreground-secondary",
  },
};

export function DomainStatusBadge({ status }: DomainStatusBadgeProps) {
  const config = statusConfig[status] ?? statusConfig.pending;
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-1 text-xs font-medium ${config.className}`}
    >
      {config.label}
    </span>
  );
}
```

- [ ] **11b.** Create `apps/admin/components/settings/DomainsList.tsx` — table with columns: Domain, Status (DomainStatusBadge), SSL Status, Verified At, Actions (Verify button for error/pending, Remove button with confirmation Dialog). Uses `removeDomainAction` and `verifyDomainAction` server actions.

- [ ] **11c.** Create `apps/admin/components/settings/AddDomainForm.tsx` — form with domain input + Cloudflare API token input (type="password"). Submit calls `addDomainAction`. Shows loading state during Cloudflare API call. Displays error messages from the action result. On success, calls `router.refresh()`.

Each component follows Paper · Ink · Moss editorial. Use `@tesserix/web` Dialog/Input/Button/Label primitives.

---

### Task 12 — Sidebar nav update

- [ ] **12a.** If not already done in S1 Task 8, add `{ label: "Domains", href: "/settings/domains" }` to the settings children array in `apps/admin/components/shell/AdminShell.tsx`.

- [ ] **12b.** Add page title mapping in `getPageTitle`:

```typescript
if (pathname.startsWith("/settings/domains")) {
  return { eyebrow: "Store Setup", title: "Custom Domains" };
}
```

---

### Task 13 — Verification

- [ ] **13a.** Run migration:
```bash
cd services/marketplace-api && go run ./cmd/migrate/ up
```

- [ ] **13b.** Run all backend tests:
```bash
cd services/marketplace-api && go test ./internal/domain/... ./internal/handlers/admin/... -v -count=1
```

- [ ] **13c.** Run frontend type check:
```bash
cd apps/admin && npx tsc --noEmit
```

- [ ] **13d.** Run backend build:
```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

- [ ] **13e.** Smoke test: start marketplace-api, verify `GET /api/v1/admin/stores/:storeId/domains` returns `{"domains":[]}` for a store with no domains.

- [ ] **13f.** Smoke test: verify the domain verification worker starts and logs `"domain verification worker started"` on startup.

---

## Estimated effort

| Task | Description | Estimate |
|------|-------------|----------|
| 1 | Migration | 10 min |
| 2 | Domain models | 15 min |
| 3 | Domain repository | 40 min |
| 4 | Cloudflare API client | 50 min |
| 5 | Verification worker | 40 min |
| 6 | Domains handler | 50 min |
| 7 | Wiring (routes + main) | 20 min |
| 8 | Frontend API client | 20 min |
| 9 | Server actions | 20 min |
| 10 | Domains settings page | 30 min |
| 11 | Client components (3 files) | 45 min |
| 12 | Sidebar nav update | 10 min |
| 13 | Verification | 20 min |
| **Total** | | **~6 hours** |
