package giftcard

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	//
	// The UPDATE pins `status <> 'refunded'`: a card whose own purchase was
	// refunded has already been paid back to the customer in real money, so
	// crediting it would mint value from nothing. It also flips a `depleted`
	// card back to `active` — DebitInTx requires status = 'active', so a
	// restored balance on a depleted card would otherwise be unspendable
	// money. `disabled` cards keep their status: the balance is frozen, not
	// destroyed, and becomes spendable again when the merchant re-enables.
	//
	// Returns apperrors.ErrGiftCardNotRedeemable when the card is refunded
	// and apperrors.ErrNotFound when the row is gone.
	CreditInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID *uuid.UUID, txnType TransactionType, note *string, tenantID uuid.UUID) (balanceAfter decimal.Decimal, err error)

	// ApplyPurchaseRefundInTx applies a provider refund issued against the
	// card's OWN purchase: it removes exactly the newly-refunded value from
	// current_balance and records it on the ledger.
	//
	// Only a FULL purchase refund voids the card (status → 'refunded',
	// balance → 0, payment_status → 'refunded'). A partial refund leaves
	// the remainder spendable — Stripe sends `charge.refunded` for partial
	// refunds too, so voiding on every refund event destroyed value the
	// merchant never refunded.
	//
	// When the customer has already spent below the refunded amount the
	// balance is floored at zero and the difference is recorded as a
	// shortfall — an unrecoverable merchant loss rather than a silent one.
	// The balance can never go negative.
	//
	// Idempotent via the cumulative total: in.RefundedTotal is the running
	// total the provider reports, and the card stores what it has already
	// applied, so a redelivered or out-of-order webhook has a non-positive
	// delta and returns Applied=false having written nothing.
	ApplyPurchaseRefundInTx(tx *gorm.DB, cardID uuid.UUID, in PurchaseRefund) (*PurchaseRefundResult, error)

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

	// SetStatus atomically flips a gift card between StatusActive and
	// StatusDisabled — the only two legal targets. Tenant- and
	// store-scoped, atomic UPDATE WHERE pattern (mirrors
	// DebitInTx/ActivateAfterPayment): the status check and the write are
	// one statement, so there is no window between reading a card's
	// eligibility and changing it. Idempotent when the card is already at
	// the target status (returns it unchanged, no error). Any other
	// source status, or an expired card in either direction, is refused
	// via classifyStatusTransition.
	SetStatus(tx *gorm.DB, id, tenantID, storeID uuid.UUID, to GiftCardStatus) (*GiftCard, error)
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
		WHERE id = ? AND status <> 'refunded'
		RETURNING id, current_balance`,
		amount, amount, cardID).Scan(&gc)

	if result.Error != nil {
		return decimal.Zero, result.Error
	}
	if result.RowsAffected == 0 {
		return decimal.Zero, classifyCreditFailure(tx, cardID)
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

// classifyCreditFailure explains why the credit UPDATE matched no row. Same
// shape as classifyDebitFailure: two independent arms (identity and the
// refunded guard), so one generic answer would be wrong for one of them.
// Read-only, failure path only.
func classifyCreditFailure(tx *gorm.DB, cardID uuid.UUID) error {
	var gc GiftCard
	err := tx.Select("id", "status").Where("id = ?", cardID).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NotFound("gift card")
	}
	if err != nil {
		return err
	}
	return apperrors.New(apperrors.CodeGiftCardNotRedeemable,
		"this gift card was refunded and cannot be credited")
}

// PurchaseRefundResult reports what applying a purchase refund did, so the
// caller can log the merchant's loss. Applied is false on a redelivered or
// out-of-order webhook, and on a card that was already voided or is gone.
type PurchaseRefundResult struct {
	CardID   uuid.UUID
	TenantID uuid.UUID
	StoreID  uuid.UUID

	Applied bool
	Voided  bool // true only for a FULL purchase refund

	// Clawback is the value removed from the balance; Shortfall is the
	// refunded value the customer had already spent, which is gone.
	Clawback  decimal.Decimal
	Shortfall decimal.Decimal

	PreviousStatus  GiftCardStatus
	PreviousBalance decimal.Decimal
	NewStatus       GiftCardStatus
	NewBalance      decimal.Decimal
}

func (gormRepository) ApplyPurchaseRefundInTx(tx *gorm.DB, cardID uuid.UUID, in PurchaseRefund) (*PurchaseRefundResult, error) {
	// SELECT ... FOR UPDATE rather than the atomic UPDATE ... WHERE pattern
	// used by DebitInTx: a partial clawback is arithmetic over the CURRENT
	// balance and the total already refunded, and expressing that in the SET
	// clause would bury the money maths in a SQL string that only an
	// integration test can reach. The row lock is held for the rest of the
	// caller's transaction, so a concurrent redeem serialises behind it and
	// there is no TOCTOU window — the lock, not a re-checked predicate, is
	// what closes it. All the arithmetic lives in purchaseRefundFor, which
	// the default (untagged) test suite covers exhaustively.
	var prev GiftCard
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", cardID).First(&prev).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// A webhook must never fail the provider over a card that is gone.
		return &PurchaseRefundResult{CardID: cardID}, nil
	}
	if err != nil {
		return nil, err
	}

	eff := purchaseRefundFor(cardRefundState{
		Initial:  prev.InitialBalance,
		Balance:  prev.CurrentBalance,
		Status:   prev.Status,
		Refunded: prev.RefundedAmount,
	}, in)

	res := &PurchaseRefundResult{
		CardID:          cardID,
		TenantID:        prev.TenantID,
		StoreID:         prev.StoreID,
		Applied:         eff.Apply,
		Voided:          eff.Voided,
		Clawback:        eff.Clawback,
		Shortfall:       eff.Shortfall,
		PreviousStatus:  prev.Status,
		PreviousBalance: prev.CurrentBalance,
		NewStatus:       eff.NewStatus,
		NewBalance:      eff.NewBalance,
	}
	if !eff.Apply {
		return res, nil
	}

	updates := map[string]any{
		"current_balance": eff.NewBalance,
		"status":          eff.NewStatus,
		"refunded_amount": in.RefundedTotal,
		"updated_at":      gorm.Expr("now()"),
	}
	// payment_status tracks the GATEWAY payment, so only a full refund
	// changes it. A partially refunded purchase is still a paid one; how
	// much came back is carried by refunded_amount.
	if eff.Voided {
		updates["payment_status"] = PaymentStatusRefunded
	}
	if err := tx.Model(&GiftCard{}).Where("id = ?", cardID).Updates(updates).Error; err != nil {
		return nil, err
	}

	if !eff.WriteLedger {
		return res, nil
	}
	return res, tx.Create(&Transaction{
		TenantID:     prev.TenantID,
		GiftCardID:   cardID,
		Type:         TxnRefund,
		Amount:       eff.LedgerAmount,
		BalanceAfter: eff.NewBalance,
		Note:         eff.Note,
	}).Error
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

// SetStatus atomically flips a gift card between StatusActive and
// StatusDisabled — the only two legal targets. Tenant- and store-scoped,
// atomic UPDATE WHERE pattern (mirrors DebitInTx/ActivateAfterPayment): the
// status check and the write are one statement, so there is no window
// between reading a card's eligibility and changing it. Idempotent when the
// card is already at the target status (returns it unchanged, no error) —
// disable/enable are treated as set-state operations, not strict
// transitions, matching the long-press-menu idempotent-200 convention used
// elsewhere in this codebase. Any other source status, or an expired card
// in either direction, is refused via classifyStatusTransition.
func (gormRepository) SetStatus(tx *gorm.DB, id, tenantID, storeID uuid.UUID, to GiftCardStatus) (*GiftCard, error) {
	var from GiftCardStatus
	switch to {
	case StatusActive:
		from = StatusDisabled
	case StatusDisabled:
		from = StatusActive
	default:
		return nil, apperrors.ValidationFailed("status", "must be \"active\" or \"disabled\"")
	}

	var gc GiftCard
	result := tx.Raw(`
		UPDATE gift_cards
		SET status = ?, updated_at = now()
		WHERE id = ? AND tenant_id = ? AND store_id = ? AND status = ?
		  AND (expires_at IS NULL OR expires_at > now())
		RETURNING *`,
		to, id, tenantID, storeID, from).Scan(&gc)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		return &gc, nil
	}
	return classifyStatusTransition(tx, id, tenantID, storeID, from, to)
}

// classifyStatusTransition explains why SetStatus's UPDATE matched no row.
// Read-only re-read, same shape as classifyDebitFailure: four independent
// arms (not found / already at target / expired / illegal source status),
// so a single generic error would be wrong for most of them.
//
// No wall-clock comparison: the UPDATE's WHERE clause only has two
// conditions beyond identity — status = from and the expiry predicate,
// evaluated by Postgres's own now(). If this re-read finds the row still at
// the `from` status the UPDATE required, the status arm of the predicate
// would have matched, so the expiry predicate is the only thing that could
// have blocked the write — no need to re-derive that with time.Now() on the
// app pod, which can disagree with the DB host under clock skew.
func classifyStatusTransition(tx *gorm.DB, id, tenantID, storeID uuid.UUID, from, to GiftCardStatus) (*GiftCard, error) {
	var gc GiftCard
	err := tx.Where("id = ? AND tenant_id = ? AND store_id = ?", id, tenantID, storeID).First(&gc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("gift card")
	}
	if err != nil {
		return nil, err
	}
	if gc.Status == to {
		// Idempotent no-op: caller asked for the state the card is already
		// in. Succeed even if the card has since expired — nothing is
		// changing, so there is nothing to refuse.
		return &gc, nil
	}
	if gc.Status == from {
		// Status matched what the UPDATE required — only the expiry
		// predicate could have blocked it.
		return nil, apperrors.New(apperrors.CodeGiftCardExpired, "gift card has expired")
	}
	return nil, apperrors.InvalidTransition("status", string(gc.Status), string(to))
}
