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
	// CreateInTx inserts a gift card and, when initialTxn is non-nil, its
	// initial "purchase" transaction inside the provided tx.
	//
	// Storefront purchases pass nil: an unpaid card holds no value, so no
	// ledger row may claim any. ActivateAfterPayment writes the purchase
	// row once the payment clears.
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
	// UPDATE WHERE pattern that also pins status = 'active', so the status
	// check and the write are a single statement with no TOCTOU window.
	// Returns the new balance, or apperrors.ErrGiftCardNotFound /
	// apperrors.ErrGiftCardNotRedeemable /
	// apperrors.ErrInsufficientGiftCardBalance depending on why the update
	// matched nothing. Inserts a "redeem" transaction row. The caller must
	// supply an open tx.
	DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// CreditInTx atomically credits the gift card balance (for refunds).
	// Inserts a transaction row of the given type.
	CreditInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID *uuid.UUID, txnType TransactionType, note *string, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// ListTransactions returns all transactions for a gift card, ordered
	// by created_at desc.
	ListTransactions(ctx context.Context, db *gorm.DB, giftCardID uuid.UUID) ([]Transaction, error)

	// ListByCustomerEmail returns cards where the given email is either
	// the purchaser or recipient, scoped to a store. Used by the
	// storefront account-area gift cards list.
	ListByCustomerEmail(ctx context.Context, db *gorm.DB, storeID uuid.UUID, email string) ([]GiftCard, error)

	// GetByCheckoutSessionID looks up a card by the provider hosted
	// checkout session id. Used by the webhook to correlate incoming
	// checkout.session.completed events to our pending gift card row.
	GetByCheckoutSessionID(ctx context.Context, db *gorm.DB, sessionID string) (*GiftCard, error)

	// GetByPaymentIntentID looks up a card by the provider payment intent
	// id. Used when webhook arrives with a pi_… reference (e.g. from
	// payment_intent.succeeded events instead of checkout.session.*).
	GetByPaymentIntentID(ctx context.Context, db *gorm.DB, intentID string) (*GiftCard, error)

	// ActivateAfterPayment flips a card from `pending` → `active`, funds
	// current_balance from initial_balance in the same statement, sets
	// payment_status=paid, and records the settled payment_intent_id. It
	// also writes the initial "purchase" ledger row, so the ledger only
	// ever claims value the buyer actually paid for.
	// Idempotent: running it on an already-active card returns nil and
	// writes nothing. The caller must supply an open tx so the balance
	// funding and the ledger row commit together.
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
	if initialTxn == nil {
		return nil
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

// DebitInTx uses a single atomic UPDATE ... WHERE status = 'active' AND
// current_balance >= amount. Pinning the status inside the same statement
// that moves the money is what makes an unpaid (`pending`), disabled or
// refunded card unspendable — there is no window between checking the
// status and writing the balance. Inserts a "redeem" transaction with the
// new balance.
func (gormRepository) DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (decimal.Decimal, error) {
	// Atomic UPDATE WHERE pattern (amendment FIX 3): single round trip,
	// no SELECT FOR UPDATE needed.
	var gc GiftCard
	result := tx.Raw(`
		UPDATE gift_cards
		SET current_balance = current_balance - ?,
		    status = CASE WHEN current_balance - ? = 0 THEN 'depleted' ELSE status END,
		    updated_at = now()
		WHERE id = ? AND status = 'active' AND current_balance >= ?
		RETURNING id, current_balance, tenant_id`,
		amount, amount, cardID, amount).Scan(&gc)

	if result.Error != nil {
		return decimal.Zero, result.Error
	}
	if result.RowsAffected == 0 {
		return decimal.Zero, classifyDebitFailure(tx, cardID)
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

// classifyDebitFailure explains why the debit UPDATE matched no row. The
// predicate has three independent arms — identity, status and balance — so
// a single "insufficient balance" answer would be a lie for two of them.
// Re-read the row and say which arm failed. Read-only: it never mutates,
// and it is only reached on the failure path.
func classifyDebitFailure(tx *gorm.DB, cardID uuid.UUID) error {
	var gc GiftCard
	err := tx.Select("id", "status", "current_balance").
		Where("id = ?", cardID).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NotFound("gift card")
	}
	if err != nil {
		return err
	}
	if gc.Status != StatusActive {
		return apperrors.New(apperrors.CodeGiftCardNotRedeemable,
			"this gift card cannot be redeemed")
	}
	return apperrors.New(apperrors.CodeInsufficientGiftCardBalance,
		"gift card balance is insufficient for this transaction")
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

// ListByCustomerEmail returns gift cards where the customer is either
// the purchaser OR the recipient, for a given store. Used by the
// storefront My Account → Gift cards page.
func (gormRepository) ListByCustomerEmail(ctx context.Context, db *gorm.DB, storeID uuid.UUID, email string) ([]GiftCard, error) {
	var cards []GiftCard
	err := db.WithContext(ctx).
		Where("store_id = ? AND (purchased_by_email = ? OR recipient_email = ?)",
			storeID, email, email).
		Order("created_at DESC").
		Limit(100).
		Find(&cards).Error
	return cards, err
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
	// The card was created unfunded (current_balance = 0) so that it was
	// never spendable before the money arrived. Funding it happens here,
	// in the *same* UPDATE that flips the status — the card becomes
	// redeemable and valuable in one atomic step, never one without the
	// other.
	//
	// Idempotent: only `pending` rows match, so a Stripe webhook retry
	// affects zero rows, returns nil, and writes no duplicate ledger row.
	var activated GiftCard
	result := tx.Raw(`
		UPDATE gift_cards
		SET status            = 'active',
		    payment_status    = 'paid',
		    current_balance   = initial_balance,
		    payment_intent_id = COALESCE(NULLIF(?, ''), payment_intent_id),
		    updated_at        = now()
		WHERE id = ? AND status = 'pending'
		RETURNING id, tenant_id, initial_balance, current_balance`,
		paymentIntentID, id).Scan(&activated)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}

	// Ledger row is written only now, so it never claims value that was
	// not paid for.
	return tx.Create(&Transaction{
		TenantID:     activated.TenantID,
		GiftCardID:   id,
		Type:         TxnPurchase,
		Amount:       activated.InitialBalance,
		BalanceAfter: activated.CurrentBalance,
	}).Error
}

func (gormRepository) MarkPaymentFailed(tx *gorm.DB, id uuid.UUID) error {
	return tx.Model(&GiftCard{}).
		Where("id = ?", id).
		Update("payment_status", PaymentStatusFailed).Error
}
