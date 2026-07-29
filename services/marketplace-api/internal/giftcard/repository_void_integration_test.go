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

// Gap A: a gift-card PURCHASE that the merchant refunds through the gateway
// must stop the card being spendable. Before this, `charge.refunded` hit the
// `default:` arm of handleGiftCardEvent and the card stayed active with a
// full balance — the merchant paid for the card twice.
//
// Run: TEST_DATABASE_URL=... go test -tags=integration ./internal/giftcard/...

func txnsFor(t *testing.T, tx *gorm.DB, cardID uuid.UUID) []Transaction {
	t.Helper()
	var out []Transaction
	require.NoError(t, tx.Where("gift_card_id = ?", cardID).Order("created_at ASC").Find(&out).Error)
	return out
}

// seedFundedCardWithRedeem seeds an active card that has already had part of
// its balance spent — the fixture that actually exposes the bug. A pristine
// unspent card would pass even a naive implementation.
func seedFundedCardWithRedeem(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID, spend string) uuid.UUID {
	t.Helper()
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "100.00")
	require.NoError(t, tx.Exec(
		`UPDATE gift_cards SET payment_status = 'paid' WHERE id = ?`, cardID).Error)
	_, err := NewRepository().DebitInTx(tx, cardID, decimal.RequireFromString(spend), uuid.New(), tenantID)
	require.NoError(t, err)
	return cardID
}

// 1. The headline case: partially-spent card, purchase refunded.
func TestVoidForPurchaseRefund_PartiallySpentCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "40.00")
	repo := NewRepository()

	res, err := repo.VoidForPurchaseRefundInTx(tx, cardID)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Voided)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID),
		"a refunded purchase must leave the card unspendable")
	assert.True(t, decimal.Zero.Equal(balanceOf(t, tx, cardID)),
		"balance must be zeroed; got %s", balanceOf(t, tx, cardID))
	assert.True(t, decimal.RequireFromString("40.00").Equal(res.Redeemed),
		"Redeemed = %s, want 40.00", res.Redeemed)

	// Ledger: purchase(+100) is absent here (seeded row), redeem(-40) from
	// the debit, and the void row.
	txns := txnsFor(t, tx, cardID)
	require.Len(t, txns, 2)
	void := txns[1]
	assert.Equal(t, TxnRefund, void.Type)
	assert.True(t, decimal.RequireFromString("-60.00").Equal(void.Amount),
		"void amount = %s, want -60.00", void.Amount)
	assert.True(t, decimal.Zero.Equal(void.BalanceAfter))
	require.NotNil(t, void.Note)
	assert.Contains(t, *void.Note, "40.00",
		"the shortfall the merchant will ask about must be recorded: %q", *void.Note)
}

// 2. The card is unspendable AFTERWARDS — the status flip is the security
// boundary, so prove the debit predicate actually refuses it.
func TestVoidForPurchaseRefund_CardIsUnspendableAfterwards(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "10.00")
	repo := NewRepository()

	_, err := repo.VoidForPurchaseRefundInTx(tx, cardID)
	require.NoError(t, err)

	_, err = repo.DebitInTx(tx, cardID, decimal.RequireFromString("1.00"), uuid.New(), tenantID)
	require.Error(t, err, "a voided card must not be spendable")
}

// 3. Idempotency: webhooks are delivered more than once. A second delivery
// must not write a second ledger row.
func TestVoidForPurchaseRefund_IsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "40.00")
	repo := NewRepository()

	first, err := repo.VoidForPurchaseRefundInTx(tx, cardID)
	require.NoError(t, err)
	require.True(t, first.Voided)

	second, err := repo.VoidForPurchaseRefundInTx(tx, cardID)
	require.NoError(t, err, "a redelivered webhook must not error")
	assert.False(t, second.Voided, "second delivery must be a no-op")

	assert.Len(t, txnsFor(t, tx, cardID), 2, "a redelivery must not write a duplicate ledger row")
	assert.True(t, decimal.Zero.Equal(balanceOf(t, tx, cardID)))
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID))
}

// 4. A card that was never funded still lands in `refunded`, and must NOT
// claim its whole face value as a shortfall.
func TestVoidForPurchaseRefund_PendingCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "0")
	repo := NewRepository()

	res, err := repo.VoidForPurchaseRefundInTx(tx, cardID)

	require.NoError(t, err)
	assert.True(t, res.Voided)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID))
	assert.True(t, decimal.Zero.Equal(res.Redeemed),
		"an unfunded card has redeemed nothing; got %s", res.Redeemed)
	assert.Empty(t, txnsFor(t, tx, cardID),
		"an unfunded card has no value to remove, so no ledger row")
}

// 5. A fully-spent card is the biggest merchant loss — the void moves no
// balance at all, so only the ledger note records what happened.
func TestVoidForPurchaseRefund_FullySpentCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID := seedFundedCardWithRedeem(t, tx, storeID, tenantID, "100.00")
	require.Equal(t, StatusDepleted, statusOf(t, tx, cardID))
	repo := NewRepository()

	res, err := repo.VoidForPurchaseRefundInTx(tx, cardID)

	require.NoError(t, err)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID),
		"a depleted card must still be pinned to refunded, not left depleted")
	assert.True(t, decimal.RequireFromString("100.00").Equal(res.Redeemed))

	txns := txnsFor(t, tx, cardID)
	require.Len(t, txns, 2)
	require.NotNil(t, txns[1].Note)
	assert.Contains(t, *txns[1].Note, "100.00")
}
