package coupon

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ListFilter holds the query parameters for listing coupons.
type ListFilter struct {
	StoreID  uuid.UUID
	TenantID uuid.UUID
	Status   string // optional filter
	Search   string // optional code/title search
	Page     int
	PerPage  int
}

// ListResult holds a page of coupons and the total count.
type ListResult struct {
	Coupons []Coupon
	Total   int64
}

// Repository is the data-access surface for coupons. Mutating methods
// take an explicit *gorm.DB so callers can thread a transaction through.
type Repository interface {
	// List returns a filtered, paginated list of coupons.
	List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error)

	// GetByID returns a single coupon by primary key, scoped to store.
	GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*Coupon, error)

	// GetByCode returns a coupon by its code within a store.
	GetByCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*Coupon, error)

	// Create inserts a new coupon row.
	Create(ctx context.Context, db *gorm.DB, c *Coupon) error

	// Update patches mutable fields on a coupon.
	Update(ctx context.Context, db *gorm.DB, c *Coupon) error

	// SoftDisable sets status = 'disabled' on the coupon.
	SoftDisable(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error

	// IncrementUsageInTx atomically increments usage_count by 1 inside the
	// supplied tx. Returns apperrors.ErrCouponUsageLimitReached when the
	// UPDATE matches zero rows (limit already hit).
	IncrementUsageInTx(tx *gorm.DB, couponID uuid.UUID) error

	// RecordUsage inserts a coupon_usage row inside the supplied tx.
	RecordUsage(tx *gorm.DB, u *CouponUsage) error

	// CountCustomerUsage returns how many times a customer email has used
	// a specific coupon, scoped to tenant.
	CountCustomerUsage(ctx context.Context, db *gorm.DB, tenantID, couponID uuid.UUID, email string) (int64, error)

	// ListUsage returns the usage records for a coupon, scoped to tenant,
	// ordered by created_at desc.
	ListUsage(ctx context.Context, db *gorm.DB, tenantID, couponID uuid.UUID, page, perPage int) ([]CouponUsage, int64, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error) {
	var result ListResult
	q := db.WithContext(ctx).Model(&Coupon{}).Where("store_id = ? AND tenant_id = ?", f.StoreID, f.TenantID)

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Search != "" {
		like := "%" + strings.ToLower(f.Search) + "%"
		q = q.Where("LOWER(code) LIKE ? OR LOWER(title) LIKE ?", like, like)
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("coupon list count: %w", err)
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

	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&result.Coupons).Error; err != nil {
		return result, fmt.Errorf("coupon list: %w", err)
	}
	return result, nil
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*Coupon, error) {
	var c Coupon
	if err := db.WithContext(ctx).Where("store_id = ? AND id = ?", storeID, id).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("coupon")
		}
		return nil, fmt.Errorf("coupon get by id: %w", err)
	}
	return &c, nil
}

func (gormRepository) GetByCode(ctx context.Context, db *gorm.DB, storeID uuid.UUID, code string) (*Coupon, error) {
	var c Coupon
	upperCode := strings.ToUpper(strings.TrimSpace(code))
	if err := db.WithContext(ctx).Where("store_id = ? AND UPPER(code) = ?", storeID, upperCode).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.CouponNotFound(code)
		}
		return nil, fmt.Errorf("coupon get by code: %w", err)
	}
	return &c, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, c *Coupon) error {
	if err := db.WithContext(ctx).Create(c).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "store_id") {
			return apperrors.CouponInvalid(fmt.Sprintf("coupon code %q already exists in this store", c.Code))
		}
		return fmt.Errorf("coupon create: %w", err)
	}
	return nil
}

func (gormRepository) Update(ctx context.Context, db *gorm.DB, c *Coupon) error {
	if err := db.WithContext(ctx).Save(c).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "store_id") {
			return apperrors.CouponInvalid(fmt.Sprintf("coupon code %q already exists in this store", c.Code))
		}
		return fmt.Errorf("coupon update: %w", err)
	}
	return nil
}

func (gormRepository) SoftDisable(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error {
	res := db.WithContext(ctx).Model(&Coupon{}).
		Where("store_id = ? AND id = ?", storeID, id).
		Update("status", CouponStatusDisabled)
	if res.Error != nil {
		return fmt.Errorf("coupon soft disable: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("coupon")
	}
	return nil
}

// IncrementUsageInTx atomically increments usage_count. The single UPDATE
// guards against overshooting usage_limit via the WHERE clause. If zero
// rows are affected, the limit was already reached.
func (gormRepository) IncrementUsageInTx(tx *gorm.DB, couponID uuid.UUID) error {
	res := tx.Exec(`
		UPDATE coupons
		SET usage_count = usage_count + 1, updated_at = now()
		WHERE id = ?
		  AND (usage_limit IS NULL OR usage_count < usage_limit)
	`, couponID)
	if res.Error != nil {
		return fmt.Errorf("coupon increment usage: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.CouponUsageLimitReached("", 0)
	}
	return nil
}

func (gormRepository) RecordUsage(tx *gorm.DB, u *CouponUsage) error {
	if err := tx.Create(u).Error; err != nil {
		return fmt.Errorf("coupon record usage: %w", err)
	}
	return nil
}

// CountCustomerUsage includes tenant_id in the WHERE clause per amendment FIX 3.
func (gormRepository) CountCustomerUsage(ctx context.Context, db *gorm.DB, tenantID, couponID uuid.UUID, email string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&CouponUsage{}).
		Where("tenant_id = ? AND coupon_id = ? AND customer_email = ?", tenantID, couponID, email).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("coupon count customer usage: %w", err)
	}
	return count, nil
}

// ListUsage includes tenant_id in the WHERE clause per amendment FIX 3.
func (gormRepository) ListUsage(ctx context.Context, db *gorm.DB, tenantID, couponID uuid.UUID, page, perPage int) ([]CouponUsage, int64, error) {
	var total int64
	q := db.WithContext(ctx).Model(&CouponUsage{}).Where("tenant_id = ? AND coupon_id = ?", tenantID, couponID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("coupon usage count: %w", err)
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var usages []CouponUsage
	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&usages).Error; err != nil {
		return nil, 0, fmt.Errorf("coupon usage list: %w", err)
	}
	return usages, total, nil
}
