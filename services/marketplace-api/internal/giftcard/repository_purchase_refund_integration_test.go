//go:build integration

package giftcard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// A gift-card PURCHASE that the merchant refunds through the gateway must
// stop the card carrying value the merchant just handed back — but only to
// the extent it was actually refunded. Stripe sends `charge.refunded` for
// PARTIAL refunds too, so voiding on the event alone destroyed the whole
// balance over a fractional refund.
//
// These exercise the real UPDATE + ledger write. The arithmetic itself is
// covered by TestPurchaseRefundFor in the default (untagged) suite, which
// is what actually gates CI.
//
// Run: TEST_DATABASE_URL=... go test -tags=integration ./internal/giftcard/...

func txnsFor(t *testing.T, tx *gorm.DB, cardID uuid.UUID) []Transaction {
	t.Helper()
	var out []Transaction
	require.NoError(t, tx.Where("gift_card_id = ?", cardID).Order("created_at ASC").Find(&out).Error)
	return out
}

func refundedAmountOf(t *testing.T, tx *gorm.DB, id uuid.UUID) decimal.Decimal {
	t.Helper()
	var got decimal.Decimal
	require.NoError(t, tx.Raw(`SELECT refunded_amount FROM gift_cards WHERE id = ?`, id).Scan(&got).Error)
	return got
}

// seedPaidCard seeds an active, fully-funded $100 card as if its purchase
// had cleared.
func seedPaidCard(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "100.00")
	require.NoError(t, tx.Exec(
		`UPDATE gift_cards SET payment_status = 'paid' WHERE id = ?`, cardID).Error)
	return cardID
}

// seedFundedCardWithRedeem seeds a paid card that has already had part of
// its balance spent — the fixture that exposes shortfall handling. A
// pristine unspent card would pass even a naive implementation.
func seedFundedCardWithRedeem(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID, spend string) uuid.UUID {
	t.Helper()
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	_, err := NewRepository().DebitInTx(tx, cardID, decimal.RequireFromString(spend), uuid.New(), tenantID)
	require.NoError(t, err)
	return cardID
}

func partial(total string) PurchaseRefund {
	return PurchaseRefund{RefundedTotal: decimal.RequireFromString(total)}
}

func full(total string) PurchaseRefund {
	return PurchaseRefund{RefundedTotal: decimal.RequireFromString(total), Full: true}
}

// ── Partial refunds: the case that must NOT void ────────────────────────

// 1. The headline case. $10 refunded off a $100 unspent card removes $10
// and leaves $90 spendable.
func TestApplyPurchaseRefund_PartialRefundOfUnspentCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	res, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.00"))

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Applied)
	assert.False(t, res.Voided, "a partial refund must not void the card")
	assert.Equal(t, StatusActive, statusOf(t, tx, cardID),
		"a partially refunded card stays spendable")
	assert.True(t, decimal.RequireFromString("90.00").Equal(balanceOf(t, tx, cardID)),
		"balance = %s, want 90.00 — only the refunded value may leave", balanceOf(t, tx, cardID))
	assert.True(t, decimal.RequireFromString("10.00").Equal(refundedAmountOf(t, tx, cardID)))

	txns := txnsFor(t, tx, cardID)
	require.Len(t, txns, 1)
	assert.Equal(t, TxnRefund, txns[0].Type)
	assert.True(t, decimal.RequireFromString("-10.00").Equal(txns[0].Amount),
		"ledger amount = %s, want -10.00", txns[0].Amount)
	assert.True(t, decimal.RequireFromString("90.00").Equal(txns[0].BalanceAfter))

	// The remaining balance is still redeemable — the whole point.
	_, err = repo.DebitInTx(tx, cardID, decimal.RequireFromString("90.00"), uuid.New(), tenantID)
	require.NoError(t, err, "the un-refunded remainder must still be spendable")
}

// 2. Refund larger than what is left: floor at zero, record the shortfall,
// never go negative.
func TestApplyPurchaseRefund_PartialBeyondRemainingBalance(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "70.00") // $30 left
	repo := NewRepository()

	res, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("50.00"))

	require.NoError(t, err)
	assert.True(t, res.Applied)
	assert.False(t, res.Voided)
	assert.True(t, decimal.RequireFromString("30.00").Equal(res.Clawback))
	assert.True(t, decimal.RequireFromString("20.00").Equal(res.Shortfall),
		"shortfall = %s, want 20.00", res.Shortfall)
	assert.True(t, decimal.Zero.Equal(balanceOf(t, tx, cardID)))
	assert.False(t, balanceOf(t, tx, cardID).IsNegative(),
		"a gift card balance must never go negative")
	assert.Equal(t, StatusDepleted, statusOf(t, tx, cardID),
		"a partial clawback that drains the card lands on depleted, not refunded")

	txns := txnsFor(t, tx, cardID)
	require.Len(t, txns, 2) // the redeem, then the clawback
	require.NotNil(t, txns[1].Note)
	assert.Contains(t, *txns[1].Note, "20.00", "the unrecoverable shortfall must be recorded")
}

// 3. Sequential partials accumulate: Stripe reports a CUMULATIVE total, so
// the second event carries 30.00, not 20.00. Clawing back the reported
// total twice would take $50 for $30 refunded.
func TestApplyPurchaseRefund_SequentialPartialsAccumulate(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	first, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.00"))
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.True(t, decimal.RequireFromString("90.00").Equal(balanceOf(t, tx, cardID)))

	second, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("30.00"))
	require.NoError(t, err)
	assert.True(t, second.Applied)
	assert.True(t, decimal.RequireFromString("20.00").Equal(second.Clawback),
		"clawback = %s, want the 20.00 delta — not the 30.00 cumulative total", second.Clawback)
	assert.True(t, decimal.RequireFromString("70.00").Equal(balanceOf(t, tx, cardID)),
		"balance = %s, want 70.00 after $30 total refunded", balanceOf(t, tx, cardID))
	assert.True(t, decimal.RequireFromString("30.00").Equal(refundedAmountOf(t, tx, cardID)))
	assert.Len(t, txnsFor(t, tx, cardID), 2)
	assert.Equal(t, StatusActive, statusOf(t, tx, cardID))
}

// 4. Redelivery of the SAME partial event: the cumulative total has not
// moved, so nothing may be clawed back twice.
func TestApplyPurchaseRefund_PartialRedeliveryIsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	first, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.00"))
	require.NoError(t, err)
	require.True(t, first.Applied)

	second, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.00"))
	require.NoError(t, err, "a redelivered webhook must not error")
	assert.False(t, second.Applied, "a redelivery must be a no-op")

	assert.True(t, decimal.RequireFromString("90.00").Equal(balanceOf(t, tx, cardID)),
		"balance = %s, want 90.00 — a redelivery must not claw back twice", balanceOf(t, tx, cardID))
	assert.Len(t, txnsFor(t, tx, cardID), 1, "a redelivery must not write a duplicate ledger row")
}

// 4b. The same redelivery check for a THREE-decimal currency amount. This
// is why refunded_amount is numeric(12,3) and not (12,2) like the balance:
// at scale 2 the stored total would round to 10.50, the redelivery would
// see a positive 0.005 delta, and the customer would be clawed back twice.
func TestApplyPurchaseRefund_ThreeDecimalAmountRoundTripsExactly(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	first, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.505"))
	require.NoError(t, err)
	require.True(t, first.Applied)
	assert.True(t, decimal.RequireFromString("10.505").Equal(refundedAmountOf(t, tx, cardID)),
		"stored cumulative total = %s, want 10.505 exactly — a rounded total re-opens the double-clawback window",
		refundedAmountOf(t, tx, cardID))

	second, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.505"))
	require.NoError(t, err)
	assert.False(t, second.Applied, "a redelivery of a 3-decimal amount must still be a no-op")
	assert.True(t, decimal.Zero.Equal(second.Clawback))
}

// 5. An out-of-order delivery carrying a smaller cumulative total must not
// re-apply anything or restore value.
func TestApplyPurchaseRefund_OutOfOrderDeliveryIsIgnored(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	_, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("30.00"))
	require.NoError(t, err)

	stale, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("10.00"))
	require.NoError(t, err)
	assert.False(t, stale.Applied)
	assert.True(t, decimal.RequireFromString("70.00").Equal(balanceOf(t, tx, cardID)))
	assert.True(t, decimal.RequireFromString("30.00").Equal(refundedAmountOf(t, tx, cardID)),
		"a stale event must not lower the cumulative refunded total")
	assert.Len(t, txnsFor(t, tx, cardID), 1)
}

// 6. A partial that lands exactly on zero leaves the card `depleted`, so a
// later legitimate credit can restore it — CreditInTx flips depleted back
// to active. Landing on `refunded` would strand that money permanently.
func TestApplyPurchaseRefund_PartialToExactlyZeroStaysRestorable(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	_, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("100.00"))
	require.NoError(t, err)
	require.Equal(t, StatusDepleted, statusOf(t, tx, cardID))

	// An order paid with this card is returned: the credit must land and
	// make the card spendable again.
	_, err = repo.CreditInTx(tx, cardID, decimal.RequireFromString("5.00"), nil, TxnAdjustment, nil, tenantID)
	require.NoError(t, err, "a depleted card must still accept a credit")
	assert.Equal(t, StatusActive, statusOf(t, tx, cardID))

	_, err = repo.DebitInTx(tx, cardID, decimal.RequireFromString("5.00"), uuid.New(), tenantID)
	require.NoError(t, err, "restored value must be spendable")
}

// ── Full refunds: 4039e8cb's behaviour, unchanged ───────────────────────

// 7. A full refund of a partly-spent card still voids it and records the
// shortfall.
func TestApplyPurchaseRefund_FullRefundVoidsPartiallySpentCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "40.00")
	repo := NewRepository()

	res, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))

	require.NoError(t, err)
	assert.True(t, res.Applied)
	assert.True(t, res.Voided)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID),
		"a fully refunded purchase must leave the card unspendable")
	assert.True(t, decimal.Zero.Equal(balanceOf(t, tx, cardID)))
	assert.True(t, decimal.RequireFromString("40.00").Equal(res.Shortfall),
		"shortfall = %s, want 40.00", res.Shortfall)

	txns := txnsFor(t, tx, cardID)
	require.Len(t, txns, 2)
	void := txns[1]
	assert.Equal(t, TxnRefund, void.Type)
	assert.True(t, decimal.RequireFromString("-60.00").Equal(void.Amount),
		"void amount = %s, want -60.00", void.Amount)
	require.NotNil(t, void.Note)
	assert.Contains(t, *void.Note, "40.00")
}

// 8. The status flip is the security boundary — prove the debit predicate
// actually refuses a voided card.
func TestApplyPurchaseRefund_VoidedCardIsUnspendable(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "10.00")
	repo := NewRepository()

	_, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))
	require.NoError(t, err)

	_, err = repo.DebitInTx(tx, cardID, decimal.RequireFromString("1.00"), uuid.New(), tenantID)
	require.Error(t, err, "a voided card must not be spendable")
}

// 9. A redelivered FULL refund must not write a second ledger row.
func TestApplyPurchaseRefund_FullRefundIsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "40.00")
	repo := NewRepository()

	first, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))
	require.NoError(t, err)
	require.True(t, first.Applied)

	second, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))
	require.NoError(t, err, "a redelivered webhook must not error")
	assert.False(t, second.Applied, "second delivery must be a no-op")

	assert.Len(t, txnsFor(t, tx, cardID), 2, "a redelivery must not write a duplicate ledger row")
	assert.True(t, decimal.Zero.Equal(balanceOf(t, tx, cardID)))
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID))
}

// 10. Partials followed by a full refund: the earlier clawbacks are already
// out of the balance, so the final void only removes what is left and
// claims no shortfall for value it already took.
func TestApplyPurchaseRefund_PartialsThenFullRefund(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedPaidCard(t, tx, storeID, tenantID)
	repo := NewRepository()

	_, err := repo.ApplyPurchaseRefundInTx(tx, cardID, partial("30.00"))
	require.NoError(t, err)

	res, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))
	require.NoError(t, err)
	assert.True(t, res.Voided)
	assert.True(t, decimal.RequireFromString("70.00").Equal(res.Clawback))
	assert.True(t, decimal.Zero.Equal(res.Shortfall),
		"shortfall = %s, want 0 — the first 30.00 was clawed back, not spent", res.Shortfall)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID))
	assert.True(t, decimal.Zero.Equal(balanceOf(t, tx, cardID)))
}

// 11. A card that was never funded still lands in `refunded`, and must NOT
// claim its whole face value as a shortfall.
func TestApplyPurchaseRefund_PendingCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "0")
	repo := NewRepository()

	res, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))

	require.NoError(t, err)
	assert.True(t, res.Applied)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID))
	assert.True(t, decimal.Zero.Equal(res.Shortfall),
		"an unfunded card has redeemed nothing; got %s", res.Shortfall)
	assert.Empty(t, txnsFor(t, tx, cardID),
		"an unfunded card has no value to remove, so no ledger row")
}

// 12. A fully-spent card is the biggest merchant loss — the void moves no
// balance at all, so only the ledger note records what happened.
func TestApplyPurchaseRefund_FullySpentCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "100.00")
	require.Equal(t, StatusDepleted, statusOf(t, tx, cardID))
	repo := NewRepository()

	res, err := repo.ApplyPurchaseRefundInTx(tx, cardID, full("100.00"))

	require.NoError(t, err)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID),
		"a depleted card must still be pinned to refunded, not left depleted")
	assert.True(t, decimal.RequireFromString("100.00").Equal(res.Shortfall))

	txns := txnsFor(t, tx, cardID)
	require.Len(t, txns, 2)
	require.NotNil(t, txns[1].Note)
	assert.Contains(t, *txns[1].Note, "100.00")
}

// 13. A card that is gone is a no-op, not an error — a webhook must never
// fail the provider over it.
func TestApplyPurchaseRefund_MissingCardIsNoOp(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository()

	res, err := repo.ApplyPurchaseRefundInTx(tx, uuid.New(), full("100.00"))

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Applied)
}
