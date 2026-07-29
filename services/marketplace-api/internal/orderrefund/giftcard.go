package orderrefund

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/order"
)

// giftCardLedger reads the two sums that describe an order's gift-card
// side. Both are aggregates over committed gift_card_transactions rows, so
// they are the same numbers on every re-entry — that is what makes the
// credit convergent instead of drifting across partial refunds.
//
// Purchase-refund voids (Gap A) also write `refund` rows, but with a NULL
// order_id, so they never leak into an order's accounting.
type giftCardLedger struct {
	Applied  decimal.Decimal
	Returned decimal.Decimal
}

func readGiftCardLedger(tx *gorm.DB, orderID uuid.UUID) (giftCardLedger, error) {
	var row struct {
		Applied  decimal.Decimal `gorm:"column:applied"`
		Returned decimal.Decimal `gorm:"column:returned"`
	}
	err := tx.Raw(`
		SELECT COALESCE(SUM(CASE WHEN type = 'redeem' THEN -amount ELSE 0 END), 0) AS applied,
		       COALESCE(SUM(CASE WHEN type = 'refund' THEN  amount ELSE 0 END), 0) AS returned
		  FROM gift_card_transactions
		 WHERE order_id = ?`, orderID).Scan(&row).Error
	return giftCardLedger{Applied: row.Applied, Returned: row.Returned}, err
}

// sumSucceededRefunds totals the order's settled refund ledger rows. Each
// row's amount is the TOTAL of that refund across both ledgers, so this is
// the authoritative "money already returned to the customer" figure.
func sumSucceededRefunds(tx *gorm.DB, orderID uuid.UUID) (decimal.Decimal, error) {
	var result struct{ Sum decimal.Decimal }
	err := tx.Table("refund_transactions").
		Where("order_id = ? AND status = ?", orderID, refundStatusSucceeded).
		Select("COALESCE(SUM(amount), 0) AS sum").
		Scan(&result).Error
	return result.Sum, err
}

// giftCardShare is one card's participation in an order.
type giftCardShare struct {
	GiftCardID uuid.UUID       `gorm:"column:gift_card_id"`
	Applied    decimal.Decimal `gorm:"column:applied"`
	Returned   decimal.Decimal `gorm:"column:returned"`
}

// remaining is how much this card is still owed back.
func (s giftCardShare) remaining() decimal.Decimal { return clampZero(s.Applied.Sub(s.Returned)) }

func readGiftCardShares(tx *gorm.DB, orderID uuid.UUID) ([]giftCardShare, error) {
	var rows []giftCardShare
	err := tx.Raw(`
		SELECT gift_card_id,
		       COALESCE(SUM(CASE WHEN type = 'redeem' THEN -amount ELSE 0 END), 0) AS applied,
		       COALESCE(SUM(CASE WHEN type = 'refund' THEN  amount ELSE 0 END), 0) AS returned
		  FROM gift_card_transactions
		 WHERE order_id = ?
		 GROUP BY gift_card_id
		 HAVING COALESCE(SUM(CASE WHEN type = 'redeem' THEN -amount ELSE 0 END), 0) > 0
		 ORDER BY MIN(created_at) ASC, gift_card_id ASC`, orderID).Scan(&rows).Error
	return rows, err
}

// returnGiftCardPortion credits back whatever of the order's gift-card
// portion is owed but not yet returned, and is the reason a refund on a
// part-store-credit order no longer short-changes the customer.
//
// It MUST be called inside the transaction that records the refund as
// succeeded and while the order row is locked, so the credit and the refund
// record commit together and two concurrent finalizers cannot each read a
// pre-credit total.
//
// It never returns an error. The gateway has already moved real money by the
// time this runs; refusing to record that refund because a gift-card row
// misbehaved would be strictly worse than a logged, merchant-visible gap.
// Every write it makes is wrapped in its own savepoint, so a failure rolls
// back only the credit, never the refund.
func (c *Coordinator) returnGiftCardPortion(ctx context.Context, tx *gorm.DB, o *order.Order, reason string) {
	if err := tx.Transaction(func(sp *gorm.DB) error {
		return c.creditGiftCards(ctx, sp, o, reason)
	}); err != nil {
		c.logError("refund: gift-card portion NOT returned — customer is short this amount",
			"order_id", o.ID.String(), "err", err)
	}
}

func (c *Coordinator) creditGiftCards(ctx context.Context, tx *gorm.DB, o *order.Order, reason string) error {
	ledger, err := readGiftCardLedger(tx, o.ID)
	if err != nil {
		return fmt.Errorf("read gift card ledger: %w", err)
	}
	if ledger.Applied.LessThanOrEqual(decimal.Zero) {
		return nil // no gift card paid for this order — the common case
	}

	returnedTotal, err := sumSucceededRefunds(tx, o.ID)
	if err != nil {
		return fmt.Errorf("sum succeeded refunds: %w", err)
	}

	owed := GiftCardCreditTarget(returnedTotal, o.GrandTotal, ledger.Applied, ledger.Returned)
	if owed.LessThanOrEqual(decimal.Zero) {
		return nil // gateway portion not exhausted yet, or already credited
	}

	shares, err := readGiftCardShares(tx, o.ID)
	if err != nil {
		return fmt.Errorf("read gift card shares: %w", err)
	}

	repo := giftcard.NewRepository()
	note := giftCardRefundNote(reason)
	left := owed

	for _, share := range shares {
		if left.LessThanOrEqual(decimal.Zero) {
			break
		}
		amount := decimal.Min(left, share.remaining())
		if amount.LessThanOrEqual(decimal.Zero) {
			continue
		}

		cardID := share.GiftCardID
		orderID := o.ID
		// Per-card savepoint: one unusable card must not strand the others.
		if err := tx.Transaction(func(sp *gorm.DB) error {
			_, cErr := repo.CreditInTx(sp, cardID, amount, &orderID, giftcard.TxnRefund, note, o.TenantID)
			return cErr
		}); err != nil {
			c.recordGiftCardCreditSkipped(tx, o, cardID, amount, err)
			continue
		}

		left = left.Sub(amount)
		c.recordGiftCardCredited(tx, o, cardID, amount)
	}
	return nil
}

// giftCardRefundNote keeps the ledger note inside the column's 200 chars.
func giftCardRefundNote(reason string) *string {
	note := "refund returned to gift card"
	if reason != "" {
		note = note + " — " + reason
	}
	if len(note) > 200 {
		note = note[:200]
	}
	return &note
}

// recordGiftCardCredited puts the store-credit half of a refund on the order
// timeline. Without it the only visible number is orders.refunded_amount,
// which counts gateway money alone — so a customer whose refund came back as
// store credit would see an order that looks under-refunded.
func (c *Coordinator) recordGiftCardCredited(tx *gorm.DB, o *order.Order, cardID uuid.UUID, amount decimal.Decimal) {
	c.appendGiftCardEvent(tx, o, order.EventKindGiftCardCredited, order.GiftCardRefundPayload{
		GiftCardID:  cardID.String(),
		Amount:      amount.StringFixed(2),
		Description: fmt.Sprintf("%s returned to gift card", amount.StringFixed(2)),
	})
}

// recordGiftCardCreditSkipped is the merchant-visible trace for money that
// could NOT be returned to a card — most often because the card's own
// purchase was already refunded, so crediting it would create value from
// nothing. Merchant-only: it is operational signal, not something a buyer
// should read, so it is filtered off the storefront timeline.
func (c *Coordinator) recordGiftCardCreditSkipped(tx *gorm.DB, o *order.Order, cardID uuid.UUID, amount decimal.Decimal, cause error) {
	c.logError("refund: gift card could not be credited — this amount was NOT returned to the customer",
		"order_id", o.ID.String(), "gift_card_id", cardID.String(),
		"amount", amount.StringFixed(2), "err", cause)

	c.appendGiftCardEvent(tx, o, order.EventKindGiftCardCreditSkipped, order.GiftCardRefundPayload{
		GiftCardID: cardID.String(),
		Amount:     amount.StringFixed(2),
		Description: fmt.Sprintf("%s could not be returned to gift card: %s",
			amount.StringFixed(2), cause),
	})
}

func (c *Coordinator) appendGiftCardEvent(tx *gorm.DB, o *order.Order, kind order.EventKind, payload order.GiftCardRefundPayload) {
	// Own savepoint: the timeline entry is a nice-to-have, the credit is not.
	err := tx.Transaction(func(sp *gorm.DB) error {
		return c.orderRepo.AppendEvent(sp, &order.OrderEvent{
			OrderID: o.ID,
			Kind:    string(kind),
			Payload: order.EncodeGiftCardRefund(payload),
		})
	})
	if err != nil {
		c.logError("refund: gift-card order event not written",
			"order_id", o.ID.String(), "kind", string(kind), "err", err)
	}
}
