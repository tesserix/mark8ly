package customer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
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

	// ListForStore returns customers with aggregated order stats for the admin list.
	ListForStore(ctx context.Context, storeID, tenantID string, q ListCustomersQuery) ([]CustomerWithStats, int64, error)

	// GetByIDForAdmin returns a customer scoped to store + tenant.
	GetByIDForAdmin(ctx context.Context, storeID, tenantID, customerID string) (*CustomerProfile, error)

	// UpdateTags replaces the tags array on a customer profile.
	UpdateTags(ctx context.Context, storeID, tenantID, customerID string, tags []string) (*CustomerProfile, error)

	// UpdateNotes replaces the notes field on a customer profile.
	UpdateNotes(ctx context.Context, storeID, tenantID, customerID, notes string) (*CustomerProfile, error)

	// SetStatus sets the customer status to active or blocked.
	SetStatus(ctx context.Context, storeID, tenantID, customerID, status, reason string) (*CustomerProfile, error)

	// ListAddressesByCustomer returns all addresses for a customer (admin detail).
	ListAddressesByCustomer(ctx context.Context, customerID string) ([]CustomerAddress, error)
}

type gormRepo struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by GORM.
func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) UpsertProfile(ctx context.Context, p *CustomerProfile) (*CustomerProfile, error) {
	// On conflict we only refresh gip_uid (in case the IdP rotated it)
	// and updated_at. We deliberately do NOT touch first_name / last_name
	// here — those are owned by the customer via the storefront /account
	// PATCH. Including them in DoUpdates caused every subsequent login
	// to clobber saved edits with whatever (usually empty) values the
	// session carried.
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "store_id"}, {Name: "email"}},
			DoUpdates: clause.AssignmentColumns([]string{"gip_uid", "updated_at"}),
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

// ---------------------------------------------------------------------------
// Admin repository methods (C2)
// ---------------------------------------------------------------------------

const statsSelect = `cp.*,
	(SELECT COUNT(*) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id AND deleted_at IS NULL) AS order_count,
	(SELECT COALESCE(SUM(grand_total), 0) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id AND deleted_at IS NULL) AS total_spent,
	(SELECT MAX(placed_at) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id AND deleted_at IS NULL) AS last_order_at`

func (r *gormRepo) ListForStore(ctx context.Context, storeID, tenantID string, q ListCustomersQuery) ([]CustomerWithStats, int64, error) {
	q.Defaults()

	base := r.db.WithContext(ctx).
		Table("customer_profiles AS cp").
		Where("cp.store_id = ? AND cp.tenant_id = ?", storeID, tenantID)

	if q.Search != "" {
		like := "%" + strings.ToLower(q.Search) + "%"
		base = base.Where("(LOWER(cp.email) LIKE ? OR LOWER(cp.first_name) LIKE ? OR LOWER(cp.last_name) LIKE ?)", like, like, like)
	}
	if q.Status != "" {
		base = base.Where("cp.status = ?", q.Status)
	}
	if q.Tag != "" {
		base = base.Where("? = ANY(cp.tags)", q.Tag)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("customer: list count: %w", err)
	}

	col := sanitizeSortColumn(q.SortBy)
	dir := sanitizeSortDir(q.SortDir)
	orderClause := col + " " + dir

	var rows []CustomerWithStats
	err := base.
		Select(statsSelect).
		Order(orderClause).
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("customer: list for store: %w", err)
	}
	if rows == nil {
		rows = []CustomerWithStats{}
	}
	return rows, total, nil
}

func (r *gormRepo) GetByIDForAdmin(ctx context.Context, storeID, tenantID, customerID string) (*CustomerProfile, error) {
	var p CustomerProfile
	err := r.db.WithContext(ctx).
		Where("id = ? AND store_id = ? AND tenant_id = ?", customerID, storeID, tenantID).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: get by id for admin: %w", err)
	}
	return &p, nil
}

func (r *gormRepo) UpdateTags(ctx context.Context, storeID, tenantID, customerID string, tags []string) (*CustomerProfile, error) {
	if tags == nil {
		tags = []string{}
	}
	err := r.db.WithContext(ctx).
		Model(&CustomerProfile{}).
		Where("id = ? AND store_id = ? AND tenant_id = ?", customerID, storeID, tenantID).
		Updates(map[string]any{
			"tags":       pq.StringArray(tags),
			"updated_at": time.Now(),
		}).Error
	if err != nil {
		return nil, fmt.Errorf("customer: update tags: %w", err)
	}
	return r.GetByIDForAdmin(ctx, storeID, tenantID, customerID)
}

func (r *gormRepo) UpdateNotes(ctx context.Context, storeID, tenantID, customerID, notes string) (*CustomerProfile, error) {
	err := r.db.WithContext(ctx).
		Model(&CustomerProfile{}).
		Where("id = ? AND store_id = ? AND tenant_id = ?", customerID, storeID, tenantID).
		Updates(map[string]any{
			"notes":      notes,
			"updated_at": time.Now(),
		}).Error
	if err != nil {
		return nil, fmt.Errorf("customer: update notes: %w", err)
	}
	return r.GetByIDForAdmin(ctx, storeID, tenantID, customerID)
}

func (r *gormRepo) SetStatus(ctx context.Context, storeID, tenantID, customerID, status, reason string) (*CustomerProfile, error) {
	if status != StatusActive && status != StatusBlocked {
		return nil, fmt.Errorf("customer: invalid status %q", status)
	}
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}
	if status == StatusBlocked {
		updates["block_reason"] = reason
	} else {
		updates["block_reason"] = nil
	}
	err := r.db.WithContext(ctx).
		Model(&CustomerProfile{}).
		Where("id = ? AND store_id = ? AND tenant_id = ?", customerID, storeID, tenantID).
		Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("customer: set status: %w", err)
	}
	return r.GetByIDForAdmin(ctx, storeID, tenantID, customerID)
}

func (r *gormRepo) ListAddressesByCustomer(ctx context.Context, customerID string) ([]CustomerAddress, error) {
	var out []CustomerAddress
	err := r.db.WithContext(ctx).
		Where("customer_id = ?", customerID).
		Order("is_default DESC, created_at ASC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("customer: list addresses by customer: %w", err)
	}
	if out == nil {
		out = []CustomerAddress{}
	}
	return out, nil
}
