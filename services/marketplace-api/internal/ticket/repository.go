package ticket

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ListFilter holds the query parameters for listing tickets.
type ListFilter struct {
	StoreID  uuid.UUID
	TenantID uuid.UUID
	Status   string // optional filter
	Search   string // optional subject/description search
	Page     int
	PerPage  int
}

// ListResult holds a page of tickets and the total count.
type ListResult struct {
	Tickets []Ticket
	Total   int64
}

// MaxPlatformPageSize and DefaultPlatformPageSize mirror the audit package's
// values so every cross-tenant read on the platform surface clamps alike.
const MaxPlatformPageSize = 500
const DefaultPlatformPageSize = 50

// PlatformListFilter is the CROSS-STORE filter. It is deliberately a separate
// type from ListFilter: that one requires a store and a tenant and matches
// nothing without them, which is the safe failure for a merchant-facing query.
// Widening it to mean "all stores when unset" would make a forgotten field a
// cross-store leak.
type PlatformListFilter struct {
	StoreID  *uuid.UUID // optional NARROWING, not a scope
	Status   string
	Priority string
	From     *time.Time
	To       *time.Time
	Page     int
	Limit    int
}

// Repository is the data-access surface for tickets.
type Repository interface {
	// List returns a filtered, paginated list of tickets.
	List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error)

	// ListPlatform returns a filtered, paginated, CROSS-STORE list of
	// tickets for the platform console. StoreID narrows rather than
	// scopes — see PlatformListFilter.
	ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error)

	// GetByID returns a single ticket by primary key, scoped to store and tenant.
	GetByID(ctx context.Context, db *gorm.DB, storeID, tenantID, id uuid.UUID) (*Ticket, error)

	// ListByCustomerEmail returns tickets submitted by a given email for a
	// store, newest first. Scoped to store so cross-store leakage is not
	// possible even if the same email is used on another storefront.
	ListByCustomerEmail(ctx context.Context, db *gorm.DB, storeID uuid.UUID, email string) ([]Ticket, error)

	// GetByIDForCustomerEmail returns a single ticket (with replies) owned
	// by the given customer email, scoped to store. Returns NotFound if the
	// ticket exists but was submitted by someone else — guards against
	// enumeration attacks.
	GetByIDForCustomerEmail(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID, email string) (*Ticket, error)

	// GetByConversation returns the (at most one) ticket linked to a
	// given Otto conversation_id within a store. Used by the
	// slm-router escalation hook to make ticket creation idempotent on
	// conversation_id.
	GetByConversation(ctx context.Context, db *gorm.DB, storeID uuid.UUID, conversationID string) (*Ticket, error)

	// Create inserts a new ticket row.
	Create(ctx context.Context, db *gorm.DB, t *Ticket) error

	// UpdateStatus patches the status (and resolved_at) on a ticket.
	UpdateStatus(ctx context.Context, db *gorm.DB, t *Ticket) error

	// CreateReply inserts a new ticket_replies row.
	CreateReply(ctx context.Context, db *gorm.DB, r *TicketReply) error

	// NextTicketNumber returns the next ticket number for a store inside a tx.
	NextTicketNumber(tx *gorm.DB, storeID uuid.UUID) (string, error)

	// CountByStatus returns a map of status -> count for a store.
	CountByStatus(ctx context.Context, db *gorm.DB, storeID, tenantID uuid.UUID) (map[string]int64, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error) {
	var result ListResult
	q := db.WithContext(ctx).Model(&Ticket{}).
		Where("store_id = ? AND tenant_id = ?", f.StoreID, f.TenantID)

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Search != "" {
		like := "%" + strings.ToLower(f.Search) + "%"
		q = q.Where("LOWER(subject) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("ticket list count: %w", err)
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&result.Tickets).Error; err != nil {
		return result, fmt.Errorf("ticket list: %w", err)
	}
	return result, nil
}

func (gormRepository) ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error) {
	var result ListResult
	// Model(&Ticket{}) so TableName() picks support_tickets. The bare
	// `tickets` table belongs to a different system — see the model.
	q := db.WithContext(ctx).Model(&Ticket{})

	// StoreID NARROWS. Unset means every store, which is the whole point of
	// this method and exactly why it is not ListFilter.
	if f.StoreID != nil {
		q = q.Where("store_id = ?", *f.StoreID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Priority != "" {
		q = q.Where("priority = ?", f.Priority)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("ticket platform list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultPlatformPageSize
	}
	if limit > MaxPlatformPageSize {
		limit = MaxPlatformPageSize
	}
	page := f.Page
	if page < 1 {
		page = 1
	}

	if err := q.Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Find(&result.Tickets).Error; err != nil {
		return result, fmt.Errorf("ticket platform list: %w", err)
	}
	return result, nil
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, storeID, tenantID, id uuid.UUID) (*Ticket, error) {
	var t Ticket
	if err := db.WithContext(ctx).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Where("store_id = ? AND tenant_id = ? AND id = ?", storeID, tenantID, id).
		First(&t).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("ticket")
		}
		return nil, fmt.Errorf("ticket get by id: %w", err)
	}
	return &t, nil
}

func (gormRepository) ListByCustomerEmail(ctx context.Context, db *gorm.DB, storeID uuid.UUID, email string) ([]Ticket, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, apperrors.ValidationFailed("email", "email is required")
	}
	var tickets []Ticket
	if err := db.WithContext(ctx).Model(&Ticket{}).
		Where("store_id = ? AND LOWER(submitted_by_email) = ?", storeID, email).
		Order("created_at DESC").
		Limit(100).
		Find(&tickets).Error; err != nil {
		return nil, fmt.Errorf("ticket list by customer: %w", err)
	}
	return tickets, nil
}

func (gormRepository) GetByIDForCustomerEmail(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID, email string) (*Ticket, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, apperrors.ValidationFailed("email", "email is required")
	}
	var t Ticket
	if err := db.WithContext(ctx).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Where("store_id = ? AND id = ? AND LOWER(submitted_by_email) = ?", storeID, id, email).
		First(&t).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("ticket")
		}
		return nil, fmt.Errorf("ticket get by id for customer: %w", err)
	}
	return &t, nil
}

func (gormRepository) GetByConversation(ctx context.Context, db *gorm.DB, storeID uuid.UUID, conversationID string) (*Ticket, error) {
	if conversationID == "" {
		return nil, apperrors.NotFound("ticket")
	}
	var t Ticket
	err := db.WithContext(ctx).
		Where("store_id = ? AND conversation_id = ?", storeID, conversationID).
		Order("created_at DESC").
		First(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("ticket")
		}
		return nil, fmt.Errorf("ticket get by conversation: %w", err)
	}
	return &t, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, t *Ticket) error {
	if err := db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("ticket create: %w", err)
	}
	return nil
}

func (gormRepository) UpdateStatus(ctx context.Context, db *gorm.DB, t *Ticket) error {
	res := db.WithContext(ctx).Model(t).
		Select("status", "resolved_at", "updated_at").
		Updates(t)
	if res.Error != nil {
		return fmt.Errorf("ticket update status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("ticket")
	}
	return nil
}

func (gormRepository) CreateReply(ctx context.Context, db *gorm.DB, r *TicketReply) error {
	if err := db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("ticket reply create: %w", err)
	}
	return nil
}

// NextTicketNumber returns the next ticket number formatted as "TKT-NNNNN".
// Must be called inside a transaction — concurrent ticket creations for
// the same store serialize on a per-store advisory lock so two requests
// never pick the same number.
//
// Postgres rejects `FOR UPDATE` alongside aggregate functions like
// MAX() (SQLSTATE 0A000), so instead of locking the rows we lock the
// store using a transaction-scoped advisory lock keyed on the store's
// UUID. The lock releases automatically at commit/rollback.
func (gormRepository) NextTicketNumber(tx *gorm.DB, storeID uuid.UUID) (string, error) {
	if err := tx.Exec(
		`SELECT pg_advisory_xact_lock(hashtext(?))`,
		storeID.String(),
	).Error; err != nil {
		return "", fmt.Errorf("ticket advisory lock: %w", err)
	}

	var next int
	// Table is support_tickets (see Ticket.TableName) — NOT the bare
	// `tickets` table. Querying the wrong table made MAX always 0, so every
	// ticket got TKT-00001 and the 2nd insert hit the store_number unique
	// constraint.
	// Scope to our own 'TKT-NNNNN' rows: any other-format ticket_number in
	// the store would make SUBSTRING/CAST mis-parse and skew MAX, producing
	// a "TKT-<n>" that already exists (the store_number unique-constraint
	// violation we saw). The LIKE keeps the sequence self-consistent.
	row := tx.Raw(`
		SELECT COALESCE(MAX(CAST(SUBSTRING(ticket_number FROM 5) AS INT)), 0) + 1
		FROM support_tickets
		WHERE store_id = ? AND ticket_number LIKE 'TKT-%'
	`, storeID).Row()
	if err := row.Scan(&next); err != nil {
		return "", fmt.Errorf("ticket next number: %w", err)
	}
	return fmt.Sprintf("TKT-%05d", next), nil
}

func (gormRepository) CountByStatus(ctx context.Context, db *gorm.DB, storeID, tenantID uuid.UUID) (map[string]int64, error) {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := db.WithContext(ctx).Model(&Ticket{}).
		Select("status, COUNT(*) as count").
		Where("store_id = ? AND tenant_id = ?", storeID, tenantID).
		Group("status").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ticket count by status: %w", err)
	}

	counts := map[string]int64{
		string(TicketStatusOpen):       0,
		string(TicketStatusInProgress): 0,
		string(TicketStatusResolved):   0,
		string(TicketStatusClosed):     0,
	}
	for _, r := range rows {
		counts[r.Status] = r.Count
	}
	return counts, nil
}
