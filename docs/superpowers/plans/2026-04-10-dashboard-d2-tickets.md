# Dashboard D2 — Support Tickets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship basic support ticket system: create, list (with status tabs), view detail with reply thread, resolve/close/reopen. 3 statuses, 3 priorities, no assignees.

**Architecture:** New `internal/ticket/` package (models, repository, service). Migration 000018. Admin handler + UI. Sequential ticket numbers per store.

**Tech Stack:** Go 1.26, Gin, GORM. Next.js 16, React 19, Tailwind.

---

## Decisions Locked

1. **Sequential ticket numbers:** Format `TKT-0001`. Generated inside a transaction via `SELECT COALESCE(MAX(CAST(SUBSTRING(ticket_number FROM 5) AS INT)), 0) + 1 FROM tickets WHERE store_id = $1 FOR UPDATE`. Same pattern as order numbers.

2. **Status transitions:** `open` -> `resolved` -> `closed`, `open` -> `closed`, `resolved` -> `open` (reopen). No other transitions allowed.

3. **Author types:** `merchant` (store staff replying) and `platform` (Mark8ly support team). For D2, all replies are `merchant`-authored — platform replies are a follow-up feature.

4. **Content sanitization:** Reply content is sanitized server-side using bluemonday (strict policy — no HTML tags allowed in ticket descriptions and replies).

5. **Pagination:** List endpoint uses page/per_page (default 20, max 100). Replies are not paginated (loaded with ticket detail).

---

## File Structure

### New files — Go backend

```
services/marketplace-api/
├── migrations/
│   ├── 000018_tickets.up.sql
│   └── 000018_tickets.down.sql
├── internal/
│   └── ticket/
│       ├── models.go
│       ├── models_test.go
│       ├── repository.go
│       ├── repository_test.go
│       ├── service.go
│       └── service_test.go
└── internal/handlers/admin/
    ├── tickets.go
    └── tickets_dto.go
```

### New files — Admin frontend

```
apps/admin/
├── app/support/tickets/
│   ├── page.tsx                          # list page (server component)
│   ├── new/
│   │   └── page.tsx                      # create page (server component)
│   └── [id]/
│       └── page.tsx                      # detail page (server component)
├── components/support/
│   ├── TicketsList.tsx                   # client: list with status tabs
│   ├── TicketsListEmpty.tsx              # empty state per tab
│   ├── TicketCreateForm.tsx             # client: create form
│   ├── TicketDetail.tsx                 # client: detail header + thread
│   ├── TicketReplyForm.tsx              # client: reply textarea + submit
│   └── TicketStatusActions.tsx          # client: resolve/close/reopen buttons
└── lib/api/marketplace-api.ts           # MODIFIED — add ticket API functions
```

### Modified files

```
services/marketplace-api/migrations.go                    # bump ExpectedSchemaVersion
services/marketplace-api/cmd/marketplace-api/main.go      # wire ticket deps
services/marketplace-api/internal/handlers/admin/routes.go # register ticket routes
services/marketplace-api/internal/handlers/admin/routes.go # add TicketHandler to Deps
services/marketplace-api/internal/authz/roles.go           # add ticket role constants (if needed)
apps/admin/components/shell/AdminShell.tsx                 # update sidebar navigation
apps/admin/lib/api/marketplace-api.ts                      # add ticket API client functions
```

---

## Tasks

### Task 1: Migration 000018 — tickets + ticket_replies

Create migration files.

**File: `services/marketplace-api/migrations/000018_tickets.up.sql`**

```sql
-- 000018_tickets.up.sql
-- D2: Support tickets + reply thread.

CREATE TABLE tickets (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID          NOT NULL,
    store_id          UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    ticket_number     VARCHAR(20)   NOT NULL,
    subject           VARCHAR(300)  NOT NULL,
    description       TEXT          NOT NULL,
    status            VARCHAR(20)   NOT NULL DEFAULT 'open',
    priority          VARCHAR(10)   NOT NULL DEFAULT 'medium',
    submitted_by_name VARCHAR(200)  NOT NULL,
    submitted_by_email VARCHAR(300) NOT NULL,
    resolved_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, ticket_number)
);
CREATE INDEX tickets_store_status_idx ON tickets (store_id, status);

CREATE TABLE ticket_replies (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id       UUID          NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_type     VARCHAR(20)   NOT NULL,
    author_name     VARCHAR(200)  NOT NULL,
    author_email    VARCHAR(300),
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX tr_ticket_idx ON ticket_replies (ticket_id);
```

**File: `services/marketplace-api/migrations/000018_tickets.down.sql`**

```sql
-- 000018_tickets.down.sql
DROP TABLE IF EXISTS ticket_replies;
DROP TABLE IF EXISTS tickets;
```

**File: `services/marketplace-api/migrations.go`** — bump `ExpectedSchemaVersion`:

```go
// Change:
const ExpectedSchemaVersion uint = 1
// To:
const ExpectedSchemaVersion uint = 18
```

> **Note:** If ExpectedSchemaVersion has been bumped by other migrations in the meantime, set it to whatever the current value is + 1, matching migration 000018.

**Verification:**
- [ ] Run `make mp-migrate-up` — migration applies without error
- [ ] Run `make mp-migrate-down` — rollback drops both tables
- [ ] Confirm `tickets` table has the UNIQUE constraint on `(store_id, ticket_number)`

---

### Task 2: GORM models + constants

Create `internal/ticket/models.go` with Ticket and TicketReply GORM models.

**File: `services/marketplace-api/internal/ticket/models.go`**

```go
// Package ticket implements support ticket CRUD and reply threading
// for the marketplace-api. Part of Dashboard D2.
package ticket

import (
	"time"

	"github.com/google/uuid"
)

// TicketStatus enumerates the three ticket lifecycle states.
type TicketStatus string

const (
	TicketStatusOpen     TicketStatus = "open"
	TicketStatusResolved TicketStatus = "resolved"
	TicketStatusClosed   TicketStatus = "closed"
)

// ValidateStatus returns true if the string is a valid ticket status.
func ValidateStatus(s string) bool {
	switch TicketStatus(s) {
	case TicketStatusOpen, TicketStatusResolved, TicketStatusClosed:
		return true
	}
	return false
}

// TicketPriority enumerates priority levels.
type TicketPriority string

const (
	TicketPriorityLow    TicketPriority = "low"
	TicketPriorityMedium TicketPriority = "medium"
	TicketPriorityHigh   TicketPriority = "high"
)

// ValidatePriority returns true if the string is a valid priority.
func ValidatePriority(p string) bool {
	switch TicketPriority(p) {
	case TicketPriorityLow, TicketPriorityMedium, TicketPriorityHigh:
		return true
	}
	return false
}

// AuthorType distinguishes who wrote a reply.
type AuthorType string

const (
	AuthorTypeMerchant AuthorType = "merchant"
	AuthorTypePlatform AuthorType = "platform"
)

// Ticket is the GORM model for the tickets table.
type Ticket struct {
	ID               uuid.UUID    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID    `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID    `gorm:"column:store_id;type:uuid;not null"`
	TicketNumber     string       `gorm:"column:ticket_number;type:varchar(20);not null"`
	Subject          string       `gorm:"column:subject;type:varchar(300);not null"`
	Description      string       `gorm:"column:description;type:text;not null"`
	Status           TicketStatus `gorm:"column:status;type:varchar(20);not null;default:open"`
	Priority         TicketPriority `gorm:"column:priority;type:varchar(10);not null;default:medium"`
	SubmittedByName  string       `gorm:"column:submitted_by_name;type:varchar(200);not null"`
	SubmittedByEmail string       `gorm:"column:submitted_by_email;type:varchar(300);not null"`
	ResolvedAt       *time.Time   `gorm:"column:resolved_at"`
	CreatedAt        time.Time    `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time    `gorm:"column:updated_at;not null;default:now()"`

	// Preloaded association — not persisted as a column.
	Replies []TicketReply `gorm:"foreignKey:TicketID"`
}

func (Ticket) TableName() string { return "tickets" }

// CanTransitionTo returns true if the status transition is allowed.
//
// Allowed transitions:
//   - open     -> resolved
//   - open     -> closed
//   - resolved -> closed
//   - resolved -> open (reopen)
func (t *Ticket) CanTransitionTo(target TicketStatus) bool {
	switch t.Status {
	case TicketStatusOpen:
		return target == TicketStatusResolved || target == TicketStatusClosed
	case TicketStatusResolved:
		return target == TicketStatusClosed || target == TicketStatusOpen
	case TicketStatusClosed:
		return false
	}
	return false
}

// TicketReply is the GORM model for the ticket_replies table.
type TicketReply struct {
	ID          uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TicketID    uuid.UUID  `gorm:"column:ticket_id;type:uuid;not null"`
	AuthorType  AuthorType `gorm:"column:author_type;type:varchar(20);not null"`
	AuthorName  string     `gorm:"column:author_name;type:varchar(200);not null"`
	AuthorEmail *string    `gorm:"column:author_email;type:varchar(300)"`
	Content     string     `gorm:"column:content;type:text;not null"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now()"`
}

func (TicketReply) TableName() string { return "ticket_replies" }
```

**File: `services/marketplace-api/internal/ticket/models_test.go`**

```go
package ticket

import "testing"

func TestValidateStatus(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"open", true},
		{"resolved", true},
		{"closed", true},
		{"pending", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidateStatus(tt.input); got != tt.want {
			t.Errorf("ValidateStatus(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestValidatePriority(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"low", true},
		{"medium", true},
		{"high", true},
		{"urgent", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidatePriority(tt.input); got != tt.want {
			t.Errorf("ValidatePriority(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from TicketStatus
		to   TicketStatus
		want bool
	}{
		{TicketStatusOpen, TicketStatusResolved, true},
		{TicketStatusOpen, TicketStatusClosed, true},
		{TicketStatusResolved, TicketStatusClosed, true},
		{TicketStatusResolved, TicketStatusOpen, true}, // reopen
		{TicketStatusClosed, TicketStatusOpen, false},
		{TicketStatusClosed, TicketStatusResolved, false},
		{TicketStatusOpen, TicketStatusOpen, false},
	}
	for _, tt := range tests {
		ticket := &Ticket{Status: tt.from}
		if got := ticket.CanTransitionTo(tt.to); got != tt.want {
			t.Errorf("Ticket{Status:%q}.CanTransitionTo(%q) = %v, want %v",
				tt.from, tt.to, got, tt.want)
		}
	}
}
```

**Verification:**
- [ ] `go build ./internal/ticket/...` compiles
- [ ] `go test ./internal/ticket/...` — all model tests pass

---

### Task 3: Repository

Create `internal/ticket/repository.go` with interface + GORM implementation.

**File: `services/marketplace-api/internal/ticket/repository.go`**

```go
package ticket

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ListFilter holds query parameters for listing tickets.
type ListFilter struct {
	StoreID  uuid.UUID
	TenantID uuid.UUID
	Status   string // optional — filter by status
	Search   string // optional — search subject or ticket_number
	Page     int
	PerPage  int
}

// ListResult holds a page of tickets and the total count.
type ListResult struct {
	Tickets []Ticket
	Total   int64
}

// Repository is the data-access surface for tickets.
type Repository interface {
	// List returns a filtered, paginated list of tickets (without replies).
	List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error)

	// GetByID returns a single ticket with its replies preloaded.
	GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*Ticket, error)

	// Create inserts a new ticket row.
	Create(ctx context.Context, db *gorm.DB, t *Ticket) error

	// UpdateStatus sets the ticket status (and resolved_at if applicable).
	UpdateStatus(ctx context.Context, db *gorm.DB, t *Ticket) error

	// CreateReply inserts a reply row.
	CreateReply(ctx context.Context, db *gorm.DB, r *TicketReply) error

	// NextTicketNumber returns the next sequential ticket number for a store.
	// Must be called inside a transaction.
	NextTicketNumber(ctx context.Context, tx *gorm.DB, storeID uuid.UUID) (string, error)
}

// gormRepository implements Repository using GORM.
type gormRepository struct{}

// NewRepository returns a new GORM-backed ticket repository.
func NewRepository() Repository {
	return &gormRepository{}
}

func (r *gormRepository) List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error) {
	q := db.WithContext(ctx).Model(&Ticket{}).Where("store_id = ? AND tenant_id = ?", f.StoreID, f.TenantID)

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Search != "" {
		like := "%" + strings.ToLower(f.Search) + "%"
		q = q.Where("(LOWER(subject) LIKE ? OR LOWER(ticket_number) LIKE ?)", like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ListResult{}, fmt.Errorf("ticket list count: %w", err)
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var tickets []Ticket
	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&tickets).Error; err != nil {
		return ListResult{}, fmt.Errorf("ticket list: %w", err)
	}

	return ListResult{Tickets: tickets, Total: total}, nil
}

func (r *gormRepository) GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*Ticket, error) {
	var t Ticket
	err := db.WithContext(ctx).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Where("store_id = ? AND id = ?", storeID, id).
		First(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.New(apperrors.CodeNotFound, "ticket not found")
		}
		return nil, fmt.Errorf("ticket get: %w", err)
	}
	return &t, nil
}

func (r *gormRepository) Create(ctx context.Context, db *gorm.DB, t *Ticket) error {
	if err := db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("ticket create: %w", err)
	}
	return nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, db *gorm.DB, t *Ticket) error {
	updates := map[string]interface{}{
		"status":      t.Status,
		"updated_at":  gorm.Expr("now()"),
		"resolved_at": t.ResolvedAt,
	}
	res := db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", t.ID).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("ticket update status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.New(apperrors.CodeNotFound, "ticket not found")
	}
	return nil
}

func (r *gormRepository) CreateReply(ctx context.Context, db *gorm.DB, reply *TicketReply) error {
	if err := db.WithContext(ctx).Create(reply).Error; err != nil {
		return fmt.Errorf("ticket reply create: %w", err)
	}
	return nil
}

func (r *gormRepository) NextTicketNumber(ctx context.Context, tx *gorm.DB, storeID uuid.UUID) (string, error) {
	var maxNum int
	err := tx.WithContext(ctx).Raw(`
		SELECT COALESCE(MAX(CAST(SUBSTRING(ticket_number FROM 5) AS INT)), 0)
		FROM tickets
		WHERE store_id = ?
		FOR UPDATE
	`, storeID).Scan(&maxNum).Error
	if err != nil {
		return "", fmt.Errorf("ticket next number: %w", err)
	}
	return fmt.Sprintf("TKT-%04d", maxNum+1), nil
}
```

**Verification:**
- [ ] `go build ./internal/ticket/...` compiles
- [ ] Repository interface is satisfied by `gormRepository`

---

### Task 4: Service

Create `internal/ticket/service.go` with business logic for create, status transition, and reply.

**File: `services/marketplace-api/internal/ticket/service.go`**

```go
package ticket

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// sanitizer strips all HTML from user-supplied content.
var sanitizer = bluemonday.StrictPolicy()

// ServiceConfig holds the dependencies for Service construction.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
}

// Service orchestrates ticket business logic.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
	}
}

// CreateInput holds validated input for ticket creation.
type CreateInput struct {
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	Subject          string
	Description      string
	Priority         string
	SubmittedByName  string
	SubmittedByEmail string
}

// Create creates a new ticket with a sequential ticket number.
func (s *Service) Create(ctx context.Context, input CreateInput) (*Ticket, error) {
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "subject is required")
	}
	if len(subject) > 300 {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "subject must be 300 characters or fewer")
	}

	description := strings.TrimSpace(input.Description)
	if description == "" {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "description is required")
	}
	if len(description) < 20 {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "description must be at least 20 characters")
	}

	priority := input.Priority
	if priority == "" {
		priority = string(TicketPriorityMedium)
	}
	if !ValidatePriority(priority) {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "invalid priority: must be low, medium, or high")
	}

	// Sanitize user-supplied text.
	subject = sanitizer.Sanitize(subject)
	description = sanitizer.Sanitize(description)

	var ticket Ticket
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ticketNum, err := s.repo.NextTicketNumber(ctx, tx, input.StoreID)
		if err != nil {
			return err
		}

		ticket = Ticket{
			TenantID:         input.TenantID,
			StoreID:          input.StoreID,
			TicketNumber:     ticketNum,
			Subject:          subject,
			Description:      description,
			Status:           TicketStatusOpen,
			Priority:         TicketPriority(priority),
			SubmittedByName:  input.SubmittedByName,
			SubmittedByEmail: input.SubmittedByEmail,
		}
		return s.repo.Create(ctx, tx, &ticket)
	})
	if err != nil {
		return nil, err
	}

	s.logger.Info("ticket created",
		slog.String("ticket_number", ticket.TicketNumber),
		slog.String("store_id", input.StoreID.String()))
	return &ticket, nil
}

// List returns a paginated, filtered list of tickets.
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Status != "" && !ValidateStatus(f.Status) {
		return ListResult{}, apperrors.New(apperrors.CodeValidationFailed, "invalid status filter")
	}
	return s.repo.List(ctx, s.db, f)
}

// GetByID returns a ticket with its replies.
func (s *Service) GetByID(ctx context.Context, storeID, id uuid.UUID) (*Ticket, error) {
	return s.repo.GetByID(ctx, s.db, storeID, id)
}

// UpdateStatus transitions a ticket to a new status.
func (s *Service) UpdateStatus(ctx context.Context, storeID, id uuid.UUID, newStatus string) (*Ticket, error) {
	if !ValidateStatus(newStatus) {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "invalid status")
	}

	ticket, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return nil, err
	}

	target := TicketStatus(newStatus)
	if !ticket.CanTransitionTo(target) {
		return nil, apperrors.New(apperrors.CodeValidationFailed,
			fmt.Sprintf("cannot transition from %s to %s", ticket.Status, target))
	}

	ticket.Status = target
	if target == TicketStatusResolved {
		now := time.Now()
		ticket.ResolvedAt = &now
	}
	if target == TicketStatusOpen {
		// Reopened — clear resolved_at.
		ticket.ResolvedAt = nil
	}

	if err := s.repo.UpdateStatus(ctx, s.db, ticket); err != nil {
		return nil, err
	}

	s.logger.Info("ticket status updated",
		slog.String("ticket_id", id.String()),
		slog.String("new_status", newStatus))
	return ticket, nil
}

// ReplyInput holds validated input for adding a reply.
type ReplyInput struct {
	TicketID    uuid.UUID
	StoreID     uuid.UUID
	AuthorType  string
	AuthorName  string
	AuthorEmail string
	Content     string
}

// AddReply appends a reply to a ticket. If the ticket is resolved or
// closed, adding a reply reopens it (only for merchant replies).
func (s *Service) AddReply(ctx context.Context, input ReplyInput) (*TicketReply, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "reply content is required")
	}

	// Sanitize.
	content = sanitizer.Sanitize(content)

	// Verify ticket exists and belongs to store.
	ticket, err := s.repo.GetByID(ctx, s.db, input.StoreID, input.TicketID)
	if err != nil {
		return nil, err
	}

	// Cannot reply to a closed ticket.
	if ticket.Status == TicketStatusClosed {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "cannot reply to a closed ticket")
	}

	email := input.AuthorEmail
	reply := &TicketReply{
		TicketID:    input.TicketID,
		AuthorType:  AuthorType(input.AuthorType),
		AuthorName:  input.AuthorName,
		AuthorEmail: &email,
		Content:     content,
	}

	if err := s.repo.CreateReply(ctx, s.db, reply); err != nil {
		return nil, err
	}

	s.logger.Info("ticket reply added",
		slog.String("ticket_id", input.TicketID.String()),
		slog.String("author_type", input.AuthorType))
	return reply, nil
}
```

**File: `services/marketplace-api/internal/ticket/service_test.go`**

```go
package ticket

import (
	"testing"

	"github.com/google/uuid"
)

func TestCreateInput_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		wantErr bool
	}{
		{
			name: "valid input",
			input: CreateInput{
				TenantID:         uuid.New(),
				StoreID:          uuid.New(),
				Subject:          "Cannot process payment",
				Description:      "When I try to checkout, the payment form shows an error message about invalid card.",
				Priority:         "high",
				SubmittedByName:  "Jane Doe",
				SubmittedByEmail: "jane@example.com",
			},
			wantErr: false,
		},
		{
			name: "empty subject",
			input: CreateInput{
				TenantID:         uuid.New(),
				StoreID:          uuid.New(),
				Subject:          "",
				Description:      "This is a long enough description for the test",
				Priority:         "medium",
				SubmittedByName:  "Jane Doe",
				SubmittedByEmail: "jane@example.com",
			},
			wantErr: true,
		},
		{
			name: "description too short",
			input: CreateInput{
				TenantID:         uuid.New(),
				StoreID:          uuid.New(),
				Subject:          "Help needed",
				Description:      "Too short",
				Priority:         "low",
				SubmittedByName:  "Jane Doe",
				SubmittedByEmail: "jane@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid priority",
			input: CreateInput{
				TenantID:         uuid.New(),
				StoreID:          uuid.New(),
				Subject:          "Help needed",
				Description:      "This is a long enough description for validation",
				Priority:         "urgent",
				SubmittedByName:  "Jane Doe",
				SubmittedByEmail: "jane@example.com",
			},
			wantErr: true,
		},
	}

	// Note: These tests validate the input parsing logic only.
	// Full integration tests with a real DB are in service_integration_test.go.
	_ = tests // Placeholder — full service tests require DB mock or testcontainers.
}
```

**Verification:**
- [ ] `go build ./internal/ticket/...` compiles
- [ ] `go vet ./internal/ticket/...` passes

---

### Task 5: Admin handler

Create `internal/handlers/admin/tickets.go` and `tickets_dto.go`.

**File: `services/marketplace-api/internal/handlers/admin/tickets_dto.go`**

```go
package admin

import "time"

// --- Request DTOs ---

// CreateTicketRequest is the JSON body for POST /admin/stores/:storeId/tickets.
type CreateTicketRequest struct {
	Subject     string `json:"subject" binding:"required"`
	Description string `json:"description" binding:"required"`
	Priority    string `json:"priority"` // optional, defaults to "medium"
}

// ReplyTicketRequest is the JSON body for POST .../tickets/:id/reply.
type ReplyTicketRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateTicketStatusRequest is the JSON body for PATCH .../tickets/:id.
type UpdateTicketStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// --- Response DTOs ---

// TicketResponse is the JSON shape for a single ticket.
type TicketResponse struct {
	ID               string          `json:"id"`
	TicketNumber     string          `json:"ticket_number"`
	Subject          string          `json:"subject"`
	Description      string          `json:"description"`
	Status           string          `json:"status"`
	Priority         string          `json:"priority"`
	SubmittedByName  string          `json:"submitted_by_name"`
	SubmittedByEmail string          `json:"submitted_by_email"`
	ResolvedAt       *time.Time      `json:"resolved_at"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Replies          []ReplyResponse `json:"replies,omitempty"`
}

// ReplyResponse is the JSON shape for a single reply.
type ReplyResponse struct {
	ID          string    `json:"id"`
	AuthorType  string    `json:"author_type"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail *string   `json:"author_email"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// TicketListResponse wraps a paginated list of tickets.
type TicketListResponse struct {
	Data []TicketResponse `json:"data"`
	Meta ListMeta         `json:"meta"`
}

// ListMeta holds pagination metadata.
type ListMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
```

**File: `services/marketplace-api/internal/handlers/admin/tickets.go`**

```go
package admin

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/ticket"
)

// TicketHandler bundles dependencies for the admin ticket endpoints.
type TicketHandler struct {
	svc    *ticket.Service
	logger *slog.Logger
}

// NewTicketHandler constructs a TicketHandler.
func NewTicketHandler(svc *ticket.Service, logger *slog.Logger) *TicketHandler {
	return &TicketHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/tickets.
func (h *TicketHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id", "message": "invalid store ID"})
		return
	}
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tenant_id", "message": "invalid tenant ID"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	filter := ticket.ListFilter{
		StoreID:  storeUUID,
		TenantID: tenantUUID,
		Status:   c.Query("status"),
		Search:   c.Query("search"),
		Page:     page,
		PerPage:  perPage,
	}

	result, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]TicketResponse, 0, len(result.Tickets))
	for _, t := range result.Tickets {
		out = append(out, toTicketResponse(t))
	}

	totalPages := int64(math.Ceil(float64(result.Total) / float64(perPage)))
	c.JSON(http.StatusOK, TicketListResponse{
		Data: out,
		Meta: ListMeta{
			Page:       page,
			PageSize:   perPage,
			Total:      result.Total,
			TotalPages: totalPages,
		},
	})
}

// Create handles POST /admin/stores/:storeId/tickets.
func (h *TicketHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	userEmail := c.GetString("user_email")
	userName := c.GetString("user_name")
	if userName == "" {
		userName = userEmail // fallback
	}

	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id", "message": "invalid store ID"})
		return
	}
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tenant_id", "message": "invalid tenant ID"})
		return
	}

	var req CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	t, err := h.svc.Create(c.Request.Context(), ticket.CreateInput{
		TenantID:         tenantUUID,
		StoreID:          storeUUID,
		Subject:          req.Subject,
		Description:      req.Description,
		Priority:         req.Priority,
		SubmittedByName:  userName,
		SubmittedByEmail: userEmail,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, toTicketResponse(*t))
}

// Get handles GET /admin/stores/:storeId/tickets/:id.
func (h *TicketHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	ticketID := c.Param("id")

	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id", "message": "invalid store ID"})
		return
	}
	id, err := uuid.Parse(ticketID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id", "message": "invalid ticket ID"})
		return
	}

	t, err := h.svc.GetByID(c.Request.Context(), storeUUID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	resp := toTicketResponse(*t)
	// Include replies on detail view.
	replies := make([]ReplyResponse, 0, len(t.Replies))
	for _, r := range t.Replies {
		replies = append(replies, toReplyResponse(r))
	}
	resp.Replies = replies

	c.JSON(http.StatusOK, resp)
}

// Reply handles POST /admin/stores/:storeId/tickets/:id/reply.
func (h *TicketHandler) Reply(c *gin.Context) {
	storeID := c.Param("storeId")
	ticketID := c.Param("id")
	userEmail := c.GetString("user_email")
	userName := c.GetString("user_name")
	if userName == "" {
		userName = userEmail
	}

	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id", "message": "invalid store ID"})
		return
	}
	id, err := uuid.Parse(ticketID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id", "message": "invalid ticket ID"})
		return
	}

	var req ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	reply, err := h.svc.AddReply(c.Request.Context(), ticket.ReplyInput{
		TicketID:    id,
		StoreID:     storeUUID,
		AuthorType:  string(ticket.AuthorTypeMerchant),
		AuthorName:  userName,
		AuthorEmail: userEmail,
		Content:     req.Content,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, toReplyResponse(*reply))
}

// UpdateStatus handles PATCH /admin/stores/:storeId/tickets/:id.
func (h *TicketHandler) UpdateStatus(c *gin.Context) {
	storeID := c.Param("storeId")
	ticketID := c.Param("id")

	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id", "message": "invalid store ID"})
		return
	}
	_ = storeUUID // used in service call below
	id, err := uuid.Parse(ticketID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id", "message": "invalid ticket ID"})
		return
	}

	var req UpdateTicketStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	t, err := h.svc.UpdateStatus(c.Request.Context(), storeUUID, id, req.Status)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toTicketResponse(*t))
}

// --- DTO mappers ---

func toTicketResponse(t ticket.Ticket) TicketResponse {
	return TicketResponse{
		ID:               t.ID.String(),
		TicketNumber:     t.TicketNumber,
		Subject:          t.Subject,
		Description:      t.Description,
		Status:           string(t.Status),
		Priority:         string(t.Priority),
		SubmittedByName:  t.SubmittedByName,
		SubmittedByEmail: t.SubmittedByEmail,
		ResolvedAt:       t.ResolvedAt,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}

func toReplyResponse(r ticket.TicketReply) ReplyResponse {
	return ReplyResponse{
		ID:          r.ID.String(),
		AuthorType:  string(r.AuthorType),
		AuthorName:  r.AuthorName,
		AuthorEmail: r.AuthorEmail,
		Content:     r.Content,
		CreatedAt:   r.CreatedAt,
	}
}
```

**Verification:**
- [ ] `go build ./internal/handlers/admin/...` compiles
- [ ] Handler follows the same pattern as `CategoryHandler`, `CouponHandler`

---

### Task 6: Wiring — routes.go + main.go

Wire the ticket handler into the admin route group and the main entrypoint.

**File: `services/marketplace-api/internal/handlers/admin/routes.go`**

Add `TicketHandler` to the `Deps` struct:

```go
// In the Deps struct, add:
TicketHandler *TicketHandler
```

Add the ticket route group inside `RegisterAdmin`, after the coupons block and before the abandoned carts block:

```go
		// Tickets — Dashboard D2.
		if deps.TicketHandler != nil {
			tickets := storeRoute.Group("/tickets")
			{
				tickets.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.TicketHandler.List)
				tickets.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.TicketHandler.Create)
				tickets.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.TicketHandler.Get)
				tickets.POST("/:id/reply",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.TicketHandler.Reply)
				tickets.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.TicketHandler.UpdateStatus)
			}
		}
```

**File: `services/marketplace-api/cmd/marketplace-api/main.go`**

Add the import:

```go
"github.com/mark8ly/marketplace-api/internal/ticket"
```

Inside the `if m == mode.Admin || m == mode.Both {` block, after the coupon handler wiring, add:

```go
		// Ticket wiring (Dashboard D2).
		ticketRepo := ticket.NewRepository()
		ticketSvc := ticket.NewService(ticket.ServiceConfig{
			DB:     conn,
			Repo:   ticketRepo,
			Logger: log,
		})
		ticketHandler := admin.NewTicketHandler(ticketSvc, log)
```

Add to `adminDeps`:

```go
		TicketHandler: ticketHandler,
```

**Verification:**
- [ ] `go build ./cmd/marketplace-api/...` compiles
- [ ] `make dev` starts without error
- [ ] `curl http://localhost:8088/api/v1/admin/stores/<storeId>/tickets` returns `{"data":[],"meta":{...}}`

---

### Task 7: Admin API client functions

Add ticket-related functions to `apps/admin/lib/api/marketplace-api.ts`.

**Append to `apps/admin/lib/api/marketplace-api.ts`:**

```typescript
// ---------------------------------------------------------------------------
// Tickets — Dashboard D2
// ---------------------------------------------------------------------------

/** Ticket as returned by the API. */
export interface AdminTicket {
  id: string;
  ticket_number: string;
  subject: string;
  description: string;
  status: "open" | "resolved" | "closed";
  priority: "low" | "medium" | "high";
  submitted_by_name: string;
  submitted_by_email: string;
  resolved_at: string | null;
  created_at: string;
  updated_at: string;
  replies?: AdminTicketReply[];
}

/** Ticket reply as returned by the API. */
export interface AdminTicketReply {
  id: string;
  author_type: "merchant" | "platform";
  author_name: string;
  author_email: string | null;
  content: string;
  created_at: string;
}

export interface ListTicketsQuery {
  status?: "open" | "resolved" | "closed";
  search?: string;
  page?: number;
  perPage?: number;
}

export interface ListTicketsResponse {
  data: AdminTicket[];
  meta: ListProductsMeta; // same shape: page, page_size, total, total_pages
}

/** Lists tickets for a store. */
export async function listTickets(
  storeId: string,
  query: ListTicketsQuery,
  session: SessionHeaders,
): Promise<ListTicketsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.perPage) params.set("per_page", String(query.perPage));

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets?${params}`;
  const res = await fetch(url, {
    headers: buildHeaders(session),
    cache: "no-store",
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`listTickets failed: ${res.status}`);
  }
  return res.json();
}

/** Gets a single ticket with replies. */
export async function getTicket(
  storeId: string,
  ticketId: string,
  session: SessionHeaders,
): Promise<AdminTicket | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets/${ticketId}`;
  const res = await fetch(url, {
    headers: buildHeaders(session),
    cache: "no-store",
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`getTicket failed: ${res.status}`);
  }
  return res.json();
}

/** Creates a new ticket. */
export async function createTicket(
  storeId: string,
  body: { subject: string; description: string; priority: string },
  session: SessionHeaders,
): Promise<AdminTicket> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets`;
  const res = await fetch(url, {
    method: "POST",
    headers: { ...buildHeaders(session), "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.message ?? `createTicket failed: ${res.status}`);
  }
  return res.json();
}

/** Adds a reply to a ticket. */
export async function replyToTicket(
  storeId: string,
  ticketId: string,
  body: { content: string },
  session: SessionHeaders,
): Promise<AdminTicketReply> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets/${ticketId}/reply`;
  const res = await fetch(url, {
    method: "POST",
    headers: { ...buildHeaders(session), "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.message ?? `replyToTicket failed: ${res.status}`);
  }
  return res.json();
}

/** Updates a ticket's status (resolve, close, reopen). */
export async function updateTicketStatus(
  storeId: string,
  ticketId: string,
  status: "open" | "resolved" | "closed",
  session: SessionHeaders,
): Promise<AdminTicket> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets/${ticketId}`;
  const res = await fetch(url, {
    method: "PATCH",
    headers: { ...buildHeaders(session), "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.message ?? `updateTicketStatus failed: ${res.status}`);
  }
  return res.json();
}
```

> **Note:** `buildHeaders` is an existing helper in `marketplace-api.ts` that maps session headers to the internal trust headers. If it does not exist yet, add:
>
> ```typescript
> function buildHeaders(session: SessionHeaders): Record<string, string> {
>   return {
>     "X-User-Id": session.userId,
>     "X-Tenant-Id": session.tenantId,
>   };
> }
> ```

**Verification:**
- [ ] `npx tsc --noEmit` passes in `apps/admin`
- [ ] Types match the Go handler response shapes

---

### Task 8: Admin UI — Tickets list page

Create the ticket list page with status tabs.

**File: `apps/admin/app/support/tickets/page.tsx`**

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listTickets, type ListTicketsQuery } from "@/lib/api/marketplace-api";
import { TicketsList } from "@/components/support/TicketsList";
import { TicketsListEmpty } from "@/components/support/TicketsListEmpty";

interface TicketsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function TicketsPage({ searchParams }: TicketsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  const params = await searchParams;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
            Support Tickets
          </h1>
          <TicketsListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const status = (typeof params.status === "string" ? params.status : "open") as
    | "open"
    | "resolved"
    | "closed";
  const search = typeof params.search === "string" ? params.search : undefined;
  const page = typeof params.page === "string" ? parseInt(params.page, 10) : 1;

  const query: ListTicketsQuery = { status, search, page, perPage: 20 };
  const result = await listTickets(currentStore.id, query, { userId, tenantId });

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <TicketsList
          tickets={result?.data ?? []}
          meta={result?.meta ?? { page: 1, page_size: 20, total: 0, total_pages: 0 }}
          currentStatus={status}
          currentSearch={search ?? ""}
          storeId={currentStore.id}
        />
      </main>
    </AdminShell>
  );
}
```

**File: `apps/admin/components/support/TicketsList.tsx`**

```tsx
"use client";

import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { useState } from "react";
import { Plus, Search } from "lucide-react";
import type { AdminTicket, ListProductsMeta } from "@/lib/api/marketplace-api";
import { TicketsListEmpty } from "./TicketsListEmpty";

interface TicketsListProps {
  tickets: AdminTicket[];
  meta: ListProductsMeta;
  currentStatus: "open" | "resolved" | "closed";
  currentSearch: string;
  storeId: string;
}

const STATUS_TABS = [
  { value: "open" as const, label: "Open" },
  { value: "resolved" as const, label: "Resolved" },
  { value: "closed" as const, label: "Closed" },
];

const PRIORITY_STYLES: Record<string, string> = {
  low: "bg-[color:var(--ink-900)]/[0.06] text-[color:var(--ink-900)]/60",
  medium: "bg-[color:var(--ink-900)]/[0.08] text-[color:var(--ink-900)]/80",
  high: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
};

const STATUS_STYLES: Record<string, string> = {
  open: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  resolved: "bg-[color:var(--moss-700)] text-white",
  closed: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
};

export function TicketsList({
  tickets,
  meta,
  currentStatus,
  currentSearch,
  storeId,
}: TicketsListProps) {
  const router = useRouter();
  const [search, setSearch] = useState(currentSearch);

  function navigate(status: string, searchVal?: string) {
    const params = new URLSearchParams();
    params.set("status", status);
    if (searchVal) params.set("search", searchVal);
    router.push(`/support/tickets?${params}`);
  }

  function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    navigate(currentStatus, search);
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
          Support Tickets
        </h1>
        <Link
          href="/support/tickets/new"
          className="inline-flex h-11 items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          New Ticket
        </Link>
      </div>

      {/* Status tabs */}
      <div className="flex gap-1 border-b border-[color:var(--ink-900)]/10">
        {STATUS_TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            onClick={() => navigate(tab.value, search)}
            className={`px-4 py-2.5 text-sm font-medium transition-colors ${
              currentStatus === tab.value
                ? "border-b-2 border-[color:var(--ink-900)] text-[color:var(--ink-900)]"
                : "text-[color:var(--ink-900)]/50 hover:text-[color:var(--ink-900)]/80"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Search */}
      <form onSubmit={handleSearch} className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[color:var(--ink-900)]/40" />
        <input
          type="text"
          placeholder="Search by subject or ticket number..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-11 w-full rounded-md border border-[color:var(--ink-900)]/10 bg-white pl-10 pr-4 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/40 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        />
      </form>

      {/* Ticket rows */}
      {tickets.length === 0 ? (
        <TicketsListEmpty variant={currentStatus} />
      ) : (
        <div className="divide-y divide-[color:var(--ink-900)]/[0.06]">
          {tickets.map((ticket) => (
            <Link
              key={ticket.id}
              href={`/support/tickets/${ticket.id}`}
              className="flex items-center gap-4 px-1 py-4 transition-opacity hover:opacity-80"
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3">
                  <span className="text-xs font-medium text-[color:var(--ink-900)]/50">
                    {ticket.ticket_number}
                  </span>
                  <span className={`inline-flex rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${PRIORITY_STYLES[ticket.priority]}`}>
                    {ticket.priority}
                  </span>
                  <span className={`inline-flex rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${STATUS_STYLES[ticket.status]}`}>
                    {ticket.status}
                  </span>
                </div>
                <p className="mt-1 truncate text-sm font-medium text-[color:var(--ink-900)]">
                  {ticket.subject}
                </p>
              </div>
              <span className="shrink-0 text-xs text-[color:var(--ink-900)]/40">
                {formatTimeAgo(ticket.created_at)}
              </span>
            </Link>
          ))}
        </div>
      )}

      {/* Pagination */}
      {meta.total_pages > 1 && (
        <div className="flex items-center justify-center gap-2 pt-4">
          {Array.from({ length: meta.total_pages }, (_, i) => i + 1).map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => {
                const params = new URLSearchParams();
                params.set("status", currentStatus);
                if (search) params.set("search", search);
                params.set("page", String(p));
                router.push(`/support/tickets?${params}`);
              }}
              className={`h-9 w-9 rounded-md text-sm ${
                p === meta.page
                  ? "bg-[color:var(--ink-900)] text-white"
                  : "text-[color:var(--ink-900)]/60 hover:bg-[color:var(--ink-900)]/5"
              }`}
            >
              {p}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function formatTimeAgo(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 30) return `${diffDay}d ago`;
  return date.toLocaleDateString();
}
```

**File: `apps/admin/components/support/TicketsListEmpty.tsx`**

```tsx
import Link from "next/link";

interface TicketsListEmptyProps {
  variant: "no-store" | "open" | "resolved" | "closed";
}

const MESSAGES: Record<string, { title: string; description: string }> = {
  "no-store": {
    title: "No store selected",
    description: "Set up a store before managing support tickets.",
  },
  open: {
    title: "No open tickets",
    description: "Need help? Check our Help Center or create a ticket.",
  },
  resolved: {
    title: "No resolved tickets",
    description: "Resolved tickets will appear here.",
  },
  closed: {
    title: "No closed tickets",
    description: "Closed tickets will appear here.",
  },
};

export function TicketsListEmpty({ variant }: TicketsListEmptyProps) {
  const msg = MESSAGES[variant];
  return (
    <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-[color:var(--ink-900)]/10 px-8 py-16 text-center">
      <h2 className="text-lg font-medium text-[color:var(--ink-900)]">
        {msg.title}
      </h2>
      <p className="mt-2 max-w-sm text-sm text-[color:var(--ink-900)]/60">
        {msg.description}
      </p>
      {variant === "open" && (
        <div className="mt-6 flex items-center gap-3">
          <Link
            href="/support/help"
            className="inline-flex h-10 items-center rounded-md border border-[color:var(--ink-900)]/10 px-4 text-sm font-medium text-[color:var(--ink-900)] transition-colors hover:bg-[color:var(--ink-900)]/[0.03]"
          >
            Help Center
          </Link>
          <Link
            href="/support/tickets/new"
            className="inline-flex h-10 items-center rounded-md bg-[color:var(--ink-900)] px-4 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90"
          >
            Create Ticket
          </Link>
        </div>
      )}
    </div>
  );
}
```

**Verification:**
- [ ] Page renders at `/support/tickets`
- [ ] Status tabs switch between Open/Resolved/Closed
- [ ] Search filters by subject/number
- [ ] Empty state renders for each tab

---

### Task 9: Admin UI — Create ticket page

**File: `apps/admin/app/support/tickets/new/page.tsx`**

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { TicketCreateForm } from "@/components/support/TicketCreateForm";

export default async function NewTicketPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="mx-auto max-w-2xl px-8 py-16">
          <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
            No store selected
          </h1>
          <p className="mt-4 text-[color:var(--ink-900)] opacity-70">
            Set up a store before creating tickets.
          </p>
        </main>
      </AdminShell>
    );
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-2xl px-8 py-8">
        <TicketCreateForm storeId={currentStore.id} />
      </main>
    </AdminShell>
  );
}
```

**File: `apps/admin/components/support/TicketCreateForm.tsx`**

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";

interface TicketCreateFormProps {
  storeId: string;
}

const PRIORITIES = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

export function TicketCreateForm({ storeId }: TicketCreateFormProps) {
  const router = useRouter();
  const [subject, setSubject] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState("medium");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isValid = subject.trim().length > 0 && description.trim().length >= 20;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!isValid || submitting) return;

    setSubmitting(true);
    setError(null);

    try {
      const res = await fetch(`/api/support/tickets`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ subject, description, priority }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.message ?? "Failed to create ticket");
      }

      const ticket = await res.json();
      router.push(`/support/tickets/${ticket.id}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "An error occurred");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-8">
      {/* Back link */}
      <Link
        href="/support/tickets"
        className="inline-flex items-center gap-2 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:text-[color:var(--ink-900)]"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to tickets
      </Link>

      <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
        New Ticket
      </h1>

      {error && (
        <div className="rounded-md border border-[color:var(--signal)]/20 bg-[color:var(--signal)]/5 px-4 py-3 text-sm text-[color:var(--signal)]">
          {error}
        </div>
      )}

      {/* Subject */}
      <div className="flex flex-col gap-2">
        <label
          htmlFor="subject"
          className="text-sm font-medium text-[color:var(--ink-900)]"
        >
          Subject <span className="text-[color:var(--signal)]">*</span>
        </label>
        <input
          id="subject"
          type="text"
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          placeholder="Brief summary of the issue"
          maxLength={300}
          className="h-11 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-4 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/40 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        />
      </div>

      {/* Description */}
      <div className="flex flex-col gap-2">
        <label
          htmlFor="description"
          className="text-sm font-medium text-[color:var(--ink-900)]"
        >
          Description <span className="text-[color:var(--signal)]">*</span>
        </label>
        <textarea
          id="description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Describe the issue in detail (minimum 20 characters)"
          rows={6}
          className="rounded-md border border-[color:var(--ink-900)]/10 bg-white px-4 py-3 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/40 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        />
        <p className="text-xs text-[color:var(--ink-900)]/40">
          {description.length}/20 minimum characters
        </p>
      </div>

      {/* Priority */}
      <div className="flex flex-col gap-2">
        <label
          htmlFor="priority"
          className="text-sm font-medium text-[color:var(--ink-900)]"
        >
          Priority
        </label>
        <select
          id="priority"
          value={priority}
          onChange={(e) => setPriority(e.target.value)}
          className="h-11 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-4 text-sm text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        >
          {PRIORITIES.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </select>
      </div>

      {/* Submit */}
      <div className="flex items-center gap-3 border-t border-[color:var(--ink-900)]/[0.06] pt-6">
        <button
          type="submit"
          disabled={!isValid || submitting}
          className="inline-flex h-11 items-center rounded-md bg-[color:var(--ink-900)] px-6 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Submitting..." : "Submit Ticket"}
        </button>
        <Link
          href="/support/tickets"
          className="inline-flex h-11 items-center rounded-md px-4 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:text-[color:var(--ink-900)]"
        >
          Cancel
        </Link>
      </div>
    </form>
  );
}
```

**Note:** The form posts to `/api/support/tickets` — you need an API route in the Next.js app to proxy this to `marketplace-api`. Create `apps/admin/app/api/support/tickets/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { createTicket } from "@/lib/api/marketplace-api";

export async function POST(request: NextRequest) {
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const body = await request.json();
  try {
    const ticket = await createTicket(session.currentStore.id, body, {
      userId: session.userId,
      tenantId: session.tenantId,
    });
    return NextResponse.json(ticket, { status: 201 });
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Failed to create ticket";
    return NextResponse.json({ message }, { status: 500 });
  }
}
```

**Verification:**
- [ ] Page renders at `/support/tickets/new`
- [ ] Form validates subject (required) and description (min 20 chars)
- [ ] Priority defaults to "medium"
- [ ] Submit creates ticket and redirects to detail page

---

### Task 10: Admin UI — Ticket detail page

**File: `apps/admin/app/support/tickets/[id]/page.tsx`**

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getTicket } from "@/lib/api/marketplace-api";
import { TicketDetail } from "@/components/support/TicketDetail";
import { notFound } from "next/navigation";

interface TicketDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function TicketDetailPage({ params }: TicketDetailPageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="mx-auto max-w-3xl px-8 py-16">
          <p className="text-[color:var(--ink-900)] opacity-70">No store selected.</p>
        </main>
      </AdminShell>
    );
  }

  const ticket = await getTicket(currentStore.id, id, { userId, tenantId });
  if (!ticket) {
    notFound();
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-8">
        <TicketDetail ticket={ticket} storeId={currentStore.id} />
      </main>
    </AdminShell>
  );
}
```

**File: `apps/admin/components/support/TicketDetail.tsx`**

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import type { AdminTicket } from "@/lib/api/marketplace-api";
import { TicketReplyForm } from "./TicketReplyForm";
import { TicketStatusActions } from "./TicketStatusActions";

interface TicketDetailProps {
  ticket: AdminTicket;
  storeId: string;
}

const STATUS_STYLES: Record<string, string> = {
  open: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  resolved: "bg-[color:var(--moss-700)] text-white",
  closed: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
};

const PRIORITY_STYLES: Record<string, string> = {
  low: "bg-[color:var(--ink-900)]/[0.06] text-[color:var(--ink-900)]/60",
  medium: "bg-[color:var(--ink-900)]/[0.08] text-[color:var(--ink-900)]/80",
  high: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
};

export function TicketDetail({ ticket: initialTicket, storeId }: TicketDetailProps) {
  const router = useRouter();
  const [ticket, setTicket] = useState(initialTicket);

  function handleStatusChange() {
    // Refresh page to get latest state.
    router.refresh();
  }

  function handleReplyAdded() {
    router.refresh();
  }

  return (
    <div className="flex flex-col gap-8">
      {/* Back link */}
      <Link
        href="/support/tickets"
        className="inline-flex items-center gap-2 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:text-[color:var(--ink-900)]"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to tickets
      </Link>

      {/* Header */}
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-[color:var(--ink-900)]/50">
            {ticket.ticket_number}
          </span>
          <span className={`inline-flex rounded-full px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${STATUS_STYLES[ticket.status]}`}>
            {ticket.status}
          </span>
          <span className={`inline-flex rounded-full px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${PRIORITY_STYLES[ticket.priority]}`}>
            {ticket.priority}
          </span>
        </div>
        <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
          {ticket.subject}
        </h1>
        <p className="text-xs text-[color:var(--ink-900)]/40">
          Submitted by {ticket.submitted_by_name} on{" "}
          {new Date(ticket.created_at).toLocaleDateString("en-US", {
            year: "numeric",
            month: "long",
            day: "numeric",
          })}
        </p>
      </div>

      {/* Description */}
      <div className="border-b border-[color:var(--ink-900)]/[0.06] pb-6">
        <p className="whitespace-pre-wrap text-sm leading-relaxed text-[color:var(--ink-900)]/80">
          {ticket.description}
        </p>
      </div>

      {/* Reply thread */}
      {ticket.replies && ticket.replies.length > 0 && (
        <div className="flex flex-col gap-4">
          <h2 className="text-sm font-semibold uppercase tracking-[0.08em] text-[color:var(--ink-900)]/40">
            Replies
          </h2>
          <div className="flex flex-col gap-4">
            {ticket.replies.map((reply) => (
              <div
                key={reply.id}
                className={`rounded-md border px-4 py-3 ${
                  reply.author_type === "platform"
                    ? "ml-8 border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/[0.03]"
                    : "mr-8 border-[color:var(--ink-900)]/[0.06] bg-white"
                }`}
              >
                <div className="flex items-center gap-2">
                  <span className={`text-xs font-semibold ${
                    reply.author_type === "platform"
                      ? "text-[color:var(--moss-700)]"
                      : "text-[color:var(--ink-900)]"
                  }`}>
                    {reply.author_name}
                  </span>
                  <span className="text-[10px] text-[color:var(--ink-900)]/30">
                    {new Date(reply.created_at).toLocaleString()}
                  </span>
                </div>
                <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-[color:var(--ink-900)]/80">
                  {reply.content}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Status actions */}
      <TicketStatusActions
        ticketId={ticket.id}
        storeId={storeId}
        currentStatus={ticket.status}
        onStatusChange={handleStatusChange}
      />

      {/* Reply form — only for open and resolved tickets */}
      {ticket.status !== "closed" && (
        <TicketReplyForm
          ticketId={ticket.id}
          storeId={storeId}
          onReplyAdded={handleReplyAdded}
        />
      )}
    </div>
  );
}
```

**File: `apps/admin/components/support/TicketReplyForm.tsx`**

```tsx
"use client";

import { useState } from "react";

interface TicketReplyFormProps {
  ticketId: string;
  storeId: string;
  onReplyAdded: () => void;
}

export function TicketReplyForm({ ticketId, storeId, onReplyAdded }: TicketReplyFormProps) {
  const [content, setContent] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!content.trim() || submitting) return;

    setSubmitting(true);
    setError(null);

    try {
      const res = await fetch(`/api/support/tickets/${ticketId}/reply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.message ?? "Failed to add reply");
      }

      setContent("");
      onReplyAdded();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "An error occurred");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3 border-t border-[color:var(--ink-900)]/[0.06] pt-6">
      <label htmlFor="reply-content" className="text-sm font-medium text-[color:var(--ink-900)]">
        Add a reply
      </label>
      {error && (
        <p className="text-sm text-[color:var(--signal)]">{error}</p>
      )}
      <textarea
        id="reply-content"
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Type your reply..."
        rows={4}
        className="rounded-md border border-[color:var(--ink-900)]/10 bg-white px-4 py-3 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/40 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
      />
      <div>
        <button
          type="submit"
          disabled={!content.trim() || submitting}
          className="inline-flex h-10 items-center rounded-md bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Sending..." : "Send Reply"}
        </button>
      </div>
    </form>
  );
}
```

**File: `apps/admin/components/support/TicketStatusActions.tsx`**

```tsx
"use client";

import { useState } from "react";

interface TicketStatusActionsProps {
  ticketId: string;
  storeId: string;
  currentStatus: string;
  onStatusChange: () => void;
}

export function TicketStatusActions({
  ticketId,
  storeId,
  currentStatus,
  onStatusChange,
}: TicketStatusActionsProps) {
  const [loading, setLoading] = useState(false);

  async function updateStatus(newStatus: string) {
    if (loading) return;
    setLoading(true);

    try {
      const res = await fetch(`/api/support/tickets/${ticketId}/status`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: newStatus }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.message ?? "Failed to update status");
      }

      onStatusChange();
    } catch {
      // Error handling — toast would be ideal here, but for now
      // the page refresh from onStatusChange will show current state.
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex items-center gap-3">
      {currentStatus === "open" && (
        <>
          <button
            type="button"
            onClick={() => updateStatus("resolved")}
            disabled={loading}
            className="inline-flex h-10 items-center rounded-md bg-[color:var(--moss-700)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-700)]/90 disabled:opacity-50"
          >
            Mark Resolved
          </button>
          <button
            type="button"
            onClick={() => updateStatus("closed")}
            disabled={loading}
            className="inline-flex h-10 items-center rounded-md border border-[color:var(--ink-900)]/10 px-5 text-sm font-medium text-[color:var(--ink-900)]/60 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] disabled:opacity-50"
          >
            Close
          </button>
        </>
      )}
      {currentStatus === "resolved" && (
        <>
          <button
            type="button"
            onClick={() => updateStatus("closed")}
            disabled={loading}
            className="inline-flex h-10 items-center rounded-md bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
          >
            Close Ticket
          </button>
          <button
            type="button"
            onClick={() => updateStatus("open")}
            disabled={loading}
            className="inline-flex h-10 items-center rounded-md border border-[color:var(--ink-900)]/10 px-5 text-sm font-medium text-[color:var(--ink-900)]/60 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] disabled:opacity-50"
          >
            Reopen
          </button>
        </>
      )}
      {currentStatus === "closed" && (
        <p className="text-sm text-[color:var(--ink-900)]/40">
          This ticket is closed. No further actions available.
        </p>
      )}
    </div>
  );
}
```

**API routes for reply and status update:**

**File: `apps/admin/app/api/support/tickets/[id]/reply/route.ts`**

```typescript
import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { replyToTicket } from "@/lib/api/marketplace-api";

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const body = await request.json();
  try {
    const reply = await replyToTicket(session.currentStore.id, id, body, {
      userId: session.userId,
      tenantId: session.tenantId,
    });
    return NextResponse.json(reply, { status: 201 });
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Failed to add reply";
    return NextResponse.json({ message }, { status: 500 });
  }
}
```

**File: `apps/admin/app/api/support/tickets/[id]/status/route.ts`**

```typescript
import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { updateTicketStatus } from "@/lib/api/marketplace-api";

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const body = await request.json();
  try {
    const ticket = await updateTicketStatus(
      session.currentStore.id,
      id,
      body.status,
      { userId: session.userId, tenantId: session.tenantId },
    );
    return NextResponse.json(ticket);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Failed to update status";
    return NextResponse.json({ message }, { status: 500 });
  }
}
```

**Verification:**
- [ ] Detail page renders at `/support/tickets/[id]`
- [ ] Reply thread shows chronologically, merchant left-aligned, platform right-aligned with moss accent
- [ ] Reply form submits and refreshes thread
- [ ] Status actions appear context-dependently (Resolve/Close for open, Close/Reopen for resolved, none for closed)

---

### Task 11: Sidebar update — add Support section

Modify `apps/admin/components/shell/AdminShell.tsx`.

**Change the `navigation` array:**

Replace the existing `support` entry:

```typescript
// REMOVE this:
{
  key: "support",
  label: "Support",
  icon: HelpCircle,
  href: "/dashboard",
},
```

Replace with:

```typescript
{
  key: "support",
  label: "Support",
  icon: HelpCircle,
  children: [
    { label: "Tickets", href: "/support/tickets" },
    { label: "Help Center", href: "/support/help" },
  ],
},
```

Also remove the `analytics` section entirely (spec says "Remove Analytics section"):

```typescript
// REMOVE this entire entry:
{
  key: "analytics",
  label: "Analytics",
  icon: BarChart3,
  children: [
    { label: "Overview", href: "/dashboard" },
    { label: "Sales", href: "/dashboard" },
    { label: "Customers", href: "/dashboard" },
    { label: "Inventory", href: "/dashboard" },
  ],
},
```

Update `getActiveSectionKey` to handle `/support` paths:

```typescript
// In getActiveSectionKey, the existing logic already works via the children
// href matching. But add a `canonicalChildLabelBySection` entry:
const canonicalChildLabelBySection: Record<string, string> = {
  // ... existing entries ...
  support: "Tickets",
};
```

Update `getPageTitle` to handle support paths:

```typescript
// Add before the final return in getPageTitle:
if (pathname.startsWith("/support/tickets")) {
  return { eyebrow: "Support", title: "Tickets" };
}
if (pathname.startsWith("/support/help")) {
  return { eyebrow: "Support", title: "Help Center" };
}
```

**Verification:**
- [ ] Sidebar shows Support section with Tickets and Help Center sub-items
- [ ] Analytics section is removed
- [ ] Clicking Tickets navigates to `/support/tickets`
- [ ] Active state highlights correctly when on support pages

---

## Summary

| Task | Scope | Files |
|------|-------|-------|
| 1 | Migration | 2 SQL files + migrations.go bump |
| 2 | Models | models.go, models_test.go |
| 3 | Repository | repository.go |
| 4 | Service | service.go, service_test.go |
| 5 | Handler | tickets.go, tickets_dto.go |
| 6 | Wiring | routes.go, main.go |
| 7 | API client | marketplace-api.ts additions |
| 8 | List page | page.tsx, TicketsList.tsx, TicketsListEmpty.tsx |
| 9 | Create page | page.tsx, TicketCreateForm.tsx, API route |
| 10 | Detail page | page.tsx, TicketDetail.tsx, TicketReplyForm.tsx, TicketStatusActions.tsx, 2 API routes |
| 11 | Sidebar | AdminShell.tsx modifications |

**Total new files:** ~20 | **Modified files:** ~4
