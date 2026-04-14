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

	// Transactions (append-only ledger)
	CreateTransaction(tx *gorm.DB, t *LoyaltyTransaction) error
	ListTransactions(ctx context.Context, db *gorm.DB, loyaltyID uuid.UUID, page, limit int) ([]LoyaltyTransaction, int64, error)
	HasEarnForOrder(ctx context.Context, db *gorm.DB, orderID uuid.UUID) (bool, error)

	// Referrals
	CreateReferral(tx *gorm.DB, r *Referral) error
	CompleteReferral(tx *gorm.DB, referralID uuid.UUID) error
	ListReferrals(ctx context.Context, db *gorm.DB, storeID uuid.UUID, page, limit int) ([]Referral, int64, error)

	// Expiry — batch select for point expiry worker.
	// Returns individual expired earn transactions with FOR UPDATE SKIP LOCKED.
	SelectExpiredTransactions(ctx context.Context, tx *gorm.DB, expiryBefore time.Time, batchSize int) ([]ExpiredTransaction, error)
}

// ExpiredTransaction represents a single expired earn transaction
// selected with FOR UPDATE SKIP LOCKED for the expiry worker.
type ExpiredTransaction struct {
	ID        uuid.UUID
	LoyaltyID uuid.UUID
	TenantID  uuid.UUID
	Points    int
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
// Also increments lifetime_points (amendment FIX 4: no separate UpdateLifetimePoints needed).
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

// --- Transactions ---

func (gormRepository) CreateTransaction(tx *gorm.DB, t *LoyaltyTransaction) error {
	return tx.Create(t).Error
}

// HasEarnForOrder reports whether an earn transaction already exists for the
// given order. Used by AwardPoints to stay idempotent under webhook retries.
func (gormRepository) HasEarnForOrder(ctx context.Context, db *gorm.DB, orderID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).
		Model(&LoyaltyTransaction{}).
		Where("order_id = ? AND type = ?", orderID, TxTypeEarn).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
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

// SelectExpiredTransactions finds individual earn-type transactions older than
// expiryBefore that haven't already been expired. Uses FOR UPDATE SKIP LOCKED
// on individual rows (amendment FIX 3: no FOR UPDATE with aggregates).
func (gormRepository) SelectExpiredTransactions(ctx context.Context, tx *gorm.DB, expiryBefore time.Time, batchSize int) ([]ExpiredTransaction, error) {
	var rows []ExpiredTransaction
	err := tx.WithContext(ctx).Raw(`
		SELECT lt.id, lt.loyalty_id, lt.tenant_id, lt.points
		FROM loyalty_transactions lt
		WHERE lt.type = 'earn'
		  AND lt.created_at < ?
		  AND NOT EXISTS (
		      SELECT 1 FROM loyalty_transactions lt2
		      WHERE lt2.loyalty_id = lt.loyalty_id
		        AND lt2.type = 'expire'
		        AND lt2.description LIKE 'expiry:' || lt.id::text || '%'
		  )
		ORDER BY lt.created_at ASC
		LIMIT ?
		FOR UPDATE OF lt SKIP LOCKED
	`, expiryBefore, batchSize).Scan(&rows).Error
	return rows, err
}

// --- Referral Code Generation ---

// GenerateReferralCode produces a 10-character uppercase base32 string
// using crypto/rand. 64 bits of entropy (acceptable for per-store uniqueness).
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
