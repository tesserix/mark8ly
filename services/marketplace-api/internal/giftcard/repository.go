package giftcard

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

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

	// DebitInTx atomically debits the gift card balance using an atomic
	// UPDATE WHERE pattern. Returns the new balance or
	// apperrors.ErrInsufficientGiftCardBalance if the balance is insufficient.
	// Inserts a "redeem" transaction row. The caller must supply an open tx.
	DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// CreditInTx atomically credits the gift card balance (for refunds).
	// Inserts a transaction row of the given type.
	CreditInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID *uuid.UUID, txnType TransactionType, note *string, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// ListTransactions returns all transactions for a gift card, ordered
	// by created_at desc.
	ListTransactions(ctx context.Context, db *gorm.DB, giftCardID uuid.UUID) ([]Transaction, error)

	// GetByCheckoutSessionID looks up a card by the provider hosted
	// checkout session id. Used by the webhook to correlate incoming
	// checkout.session.completed events to our pending gift card row.
	GetByCheckoutSessionID(ctx context.Context, db *gorm.DB, sessionID string) (*GiftCard, error)

	// GetByPaymentIntentID looks up a card by the provider payment intent
	// id. Used when webhook arrives with a pi_… reference (e.g. from
	// payment_intent.succeeded events instead of checkout.session.*).
	GetByPaymentIntentID(ctx context.Context, db *gorm.DB, intentID string) (*GiftCard, error)

	// ActivateAfterPayment flips a card from `pending` → `active`, sets
	// payment_status=paid, and records the settled payment_intent_id.
	// Idempotent: running it on an already-active card returns nil.
	ActivateAfterPayment(tx *gorm.DB, id uuid.UUID, paymentIntentID string) error

	// MarkPaymentFailed marks the card's payment as failed without
	// disabling the card itself (merchant can retry).
	MarkPaymentFailed(tx *gorm.DB, id uuid.UUID) error
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

// DebitInTx uses a single atomic UPDATE ... WHERE current_balance >= amount.
// Zero rows affected means insufficient balance. Inserts a "redeem"
// transaction with the new balance.
func (gormRepository) DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (decimal.Decimal, error) {
	// Atomic UPDATE WHERE pattern (amendment FIX 3): single round trip,
	// no SELECT FOR UPDATE needed.
	var gc GiftCard
	result := tx.Raw(`
		UPDATE gift_cards
		SET current_balance = current_balance - ?,
		    status = CASE WHEN current_balance - ? = 0 THEN 'depleted' ELSE status END,
		    updated_at = now()
		WHERE id = ? AND current_balance >= ?
		RETURNING id, current_balance, tenant_id`,
		amount, amount, cardID, amount).Scan(&gc)

	if result.Error != nil {
		return decimal.Zero, result.Error
	}
	if result.RowsAffected == 0 {
		// Either the card doesn't exist or balance is insufficient.
		// Check existence to give the right error.
		var exists int64
		tx.Model(&GiftCard{}).Where("id = ?", cardID).Count(&exists)
		if exists == 0 {
			return decimal.Zero, apperrors.NotFound("gift card")
		}
		return decimal.Zero, apperrors.New(apperrors.CodeInsufficientGiftCardBalance,
			"gift card balance is insufficient for this transaction")
	}

	newBalance := gc.CurrentBalance

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
	result := tx.Raw(`
		UPDATE gift_cards
		SET current_balance = current_balance + ?,
		    status = CASE WHEN status = 'depleted' AND current_balance + ? > 0 THEN 'active' ELSE status END,
		    updated_at = now()
		WHERE id = ?
		RETURNING id, current_balance`,
		amount, amount, cardID).Scan(&gc)

	if result.Error != nil {
		return decimal.Zero, result.Error
	}
	if result.RowsAffected == 0 {
		return decimal.Zero, apperrors.NotFound("gift card")
	}

	newBalance := gc.CurrentBalance

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

func (gormRepository) GetByCheckoutSessionID(ctx context.Context, db *gorm.DB, sessionID string) (*GiftCard, error) {
	var gc GiftCard
	err := db.WithContext(ctx).
		Where("checkout_session_id = ?", sessionID).
		First(&gc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.New(apperrors.CodeNotFound, "gift card not found for session")
		}
		return nil, err
	}
	return &gc, nil
}

func (gormRepository) GetByPaymentIntentID(ctx context.Context, db *gorm.DB, intentID string) (*GiftCard, error) {
	var gc GiftCard
	err := db.WithContext(ctx).
		Where("payment_intent_id = ?", intentID).
		First(&gc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.New(apperrors.CodeNotFound, "gift card not found for payment intent")
		}
		return nil, err
	}
	return &gc, nil
}

func (gormRepository) ActivateAfterPayment(tx *gorm.DB, id uuid.UUID, paymentIntentID string) error {
	// Idempotent: only flip `pending → active`; if the card is already
	// active, the UPDATE affects zero rows and we return nil. This lets
	// Stripe webhook retries be safe.
	updates := map[string]any{
		"status":         StatusActive,
		"payment_status": PaymentStatusPaid,
	}
	if paymentIntentID != "" {
		updates["payment_intent_id"] = paymentIntentID
	}
	return tx.Model(&GiftCard{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(updates).Error
}

func (gormRepository) MarkPaymentFailed(tx *gorm.DB, id uuid.UUID) error {
	return tx.Model(&GiftCard{}).
		Where("id = ?", id).
		Update("payment_status", PaymentStatusFailed).Error
}
