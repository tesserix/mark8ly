//go:build integration

package giftcard

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These cover the security boundary itself: the SQL predicate that
// decides whether money moves. They need real Postgres — the guard is a
// WHERE clause and a CHECK constraint, neither of which exists in Go.
//
// Run: TEST_DATABASE_URL=... go test -tags=integration ./internal/giftcard/...

func seedStore(t *testing.T, tx *gorm.DB) (storeID, tenantID uuid.UUID) {
	t.Helper()
	storeID, tenantID = uuid.New(), uuid.New()
	require.NoError(t, tx.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code,
		                    timezone, status, storefront_customer_portal_secret)
		VALUES (?, ?, ?, 'Test Store', 'AU', 'AUD', 'Australia/Sydney', 'active', 'secret')`,
		storeID, tenantID, "gc-"+storeID.String()[:8]).Error)
	return storeID, tenantID
}

// seedCard inserts a card in an explicit state, bypassing the service so
// the repository is tested in isolation. Returns the id and the code.
func seedCard(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID, status GiftCardStatus, balance string) (uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	// Codes are stored canonical-uppercase; normalizeCode uppercases on
	// the way in, so a lowercase seed would never be found by lookup.
	code := "CODE" + strings.ToUpper(id.String()[:8])
	require.NoError(t, tx.Exec(`
		INSERT INTO gift_cards (id, tenant_id, store_id, code, initial_balance,
		                        current_balance, currency_code, status)
		VALUES (?, ?, ?, ?, ?, ?, 'AUD', ?)`,
		id, tenantID, storeID, code,
		decimal.RequireFromString("100.00"), decimal.RequireFromString(balance), status).Error)
	return id, code
}

func balanceOf(t *testing.T, tx *gorm.DB, id uuid.UUID) decimal.Decimal {
	t.Helper()
	var got decimal.Decimal
	require.NoError(t, tx.Raw(`SELECT current_balance FROM gift_cards WHERE id = ?`, id).Scan(&got).Error)
	return got
}

// ── The money bug ──────────────────────────────────────────────────────

// TestDebitInTx_RejectsPendingCard is the regression test for the free-money
// path: a card whose Stripe checkout was abandoned must not be spendable.
func TestDebitInTx_RejectsPendingCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	// Balance deliberately funded, as the old Purchase did — proving the
	// status predicate is what stops the debit, not an empty balance.
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "100.00")
	repo := NewRepository()

	_, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("40.00"), uuid.New(), tenantID)

	require.Error(t, err, "an unpaid (pending) gift card must not be redeemable")
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardNotRedeemable, ae.Code)
	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)),
		"balance must be untouched after a rejected debit")
}

// TestDebitInTx_RejectsDeclinedCard covers the second free-money path:
// Stripe explicitly declined the payment, MarkPaymentFailed set
// payment_status='failed' and left status='pending'.
func TestDebitInTx_RejectsDeclinedCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "100.00")
	repo := NewRepository()

	require.NoError(t, repo.MarkPaymentFailed(tx, cardID))

	_, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("40.00"), uuid.New(), tenantID)

	require.Error(t, err, "a declined gift card must not be redeemable")
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardNotRedeemable, ae.Code)
	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)))
}

// A disabled card must not be reported as "insufficient balance" — that
// sends support down the wrong path and lies to the shopper.
func TestDebitInTx_DisabledCardIsNotReportedAsInsufficientBalance(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusDisabled, "100.00")
	repo := NewRepository()

	_, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("10.00"), uuid.New(), tenantID)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardNotRedeemable, ae.Code)
	assert.NotEqual(t, apperrors.CodeInsufficientGiftCardBalance, ae.Code)
}

// ── The legitimate path must survive ───────────────────────────────────

func TestDebitInTx_ActiveCardRedeemsAndWritesLedger(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "100.00")
	repo := NewRepository()
	orderID := uuid.New()

	after, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("40.00"), orderID, tenantID)

	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("60.00").Equal(after), "got %s", after)
	assert.True(t, decimal.RequireFromString("60.00").Equal(balanceOf(t, tx, cardID)))

	txns, err := repo.ListTransactions(context.Background(), tx, cardID)
	require.NoError(t, err)
	require.Len(t, txns, 1)
	assert.Equal(t, TxnRedeem, txns[0].Type)
	assert.True(t, decimal.RequireFromString("-40.00").Equal(txns[0].Amount))
}

func TestDebitInTx_ExactBalanceDepletesCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "40.00")
	repo := NewRepository()

	after, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("40.00"), uuid.New(), tenantID)

	require.NoError(t, err)
	assert.True(t, after.IsZero())

	var status GiftCardStatus
	require.NoError(t, tx.Raw(`SELECT status FROM gift_cards WHERE id = ?`, cardID).Scan(&status).Error)
	assert.Equal(t, StatusDepleted, status)
}

func TestDebitInTx_OverdraftStillRejectedAndBalanceNeverNegative(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "10.00")
	repo := NewRepository()

	_, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("40.00"), uuid.New(), tenantID)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeInsufficientGiftCardBalance, ae.Code,
		"an active card with too little money is an insufficient-balance case, not a status case")
	assert.True(t, decimal.RequireFromString("10.00").Equal(balanceOf(t, tx, cardID)))
}

func TestDebitInTx_UnknownCardIsNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository()

	_, err := repo.DebitInTx(tx, uuid.New(), decimal.RequireFromString("1.00"), uuid.New(), uuid.New())

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeNotFound, ae.Code)
}

// ── Activation funds the card ──────────────────────────────────────────

func TestActivateAfterPayment_FundsBalanceAndMakesCardSpendable(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	// As Purchase now writes it: pending and unfunded.
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "0.00")
	repo := NewRepository()

	// Before the webhook: no money, no ledger, no redemption.
	assert.True(t, balanceOf(t, tx, cardID).IsZero())
	_, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("1.00"), uuid.New(), tenantID)
	require.Error(t, err, "an unfunded pending card must not be redeemable")

	require.NoError(t, repo.ActivateAfterPayment(tx, cardID, "pi_test_123"))

	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)),
		"activation must fund current_balance from initial_balance")

	var status GiftCardStatus
	require.NoError(t, tx.Raw(`SELECT status FROM gift_cards WHERE id = ?`, cardID).Scan(&status).Error)
	assert.Equal(t, StatusActive, status)

	// The purchase ledger row appears only now — never before payment.
	txns, err := repo.ListTransactions(context.Background(), tx, cardID)
	require.NoError(t, err)
	require.Len(t, txns, 1)
	assert.Equal(t, TxnPurchase, txns[0].Type)
	assert.True(t, decimal.RequireFromString("100.00").Equal(txns[0].BalanceAfter))

	// And now the shopper can actually spend it.
	after, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("30.00"), uuid.New(), tenantID)
	require.NoError(t, err, "a paid, active card must still redeem end to end")
	assert.True(t, decimal.RequireFromString("70.00").Equal(after))
}

// ActivateByCheckoutSession can arrive without a payment intent id. The
// funding must still happen, and the existing id must not be clobbered.
func TestActivateAfterPayment_EmptyPaymentIntentStillFunds(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "0.00")
	require.NoError(t, tx.Exec(`UPDATE gift_cards SET payment_intent_id = 'pi_existing' WHERE id = ?`, cardID).Error)
	repo := NewRepository()

	require.NoError(t, repo.ActivateAfterPayment(tx, cardID, ""))

	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)))
	var intent string
	require.NoError(t, tx.Raw(`SELECT payment_intent_id FROM gift_cards WHERE id = ?`, cardID).Scan(&intent).Error)
	assert.Equal(t, "pi_existing", intent, "an empty intent id must not blank the recorded one")
}

// Stripe retries webhooks. A second activation must neither re-fund a
// partly-spent card nor duplicate the purchase ledger row.
func TestActivateAfterPayment_IsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "0.00")
	repo := NewRepository()

	require.NoError(t, repo.ActivateAfterPayment(tx, cardID, "pi_test_123"))
	_, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("100.00"), uuid.New(), tenantID)
	require.NoError(t, err)

	require.NoError(t, repo.ActivateAfterPayment(tx, cardID, "pi_test_123"),
		"a webhook retry must be a no-op, not an error")

	assert.True(t, balanceOf(t, tx, cardID).IsZero(),
		"a retry must not re-fund a card the shopper already spent")
	txns, err := repo.ListTransactions(context.Background(), tx, cardID)
	require.NoError(t, err)
	assert.Len(t, txns, 2, "one purchase + one redeem — no duplicate purchase row")
}

// ── The checkout sequence, end to end ──────────────────────────────────

// checkout_ext.go does exactly two things with a gift card: CheckBalance
// at Step 3.5 (which decides the discount) and Debit at Step 4.6 (which
// moves the money). These replay that pair against real SQL.

func checkoutRedeem(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID, code string, orderTotal decimal.Decimal) error {
	t.Helper()
	svc := NewService(tx, NewRepository(), nil)

	// Step 3.5 — look up the balance and size the discount.
	bal, err := svc.CheckBalance(context.Background(), storeID, code)
	if err != nil {
		return err
	}
	debitAmount := orderTotal
	if bal.CurrentBalance.LessThan(orderTotal) {
		debitAmount = bal.CurrentBalance
	}

	card, err := svc.GetByCode(context.Background(), storeID, code)
	if err != nil {
		return err
	}

	// Step 4.6 — move the money on the caller's tx.
	_, err = svc.Debit(tx, card.ID, debitAmount, uuid.New(), tenantID)
	return err
}

func TestCheckoutSequence_ActiveCardRedeems(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, code := seedCard(t, tx, storeID, tenantID, StatusActive, "100.00")

	err := checkoutRedeem(t, tx, storeID, tenantID, code, decimal.RequireFromString("60.00"))

	require.NoError(t, err, "a legitimate active card must still redeem at checkout")
	assert.True(t, decimal.RequireFromString("40.00").Equal(balanceOf(t, tx, cardID)))
}

func TestCheckoutSequence_PendingCardIsRejectedAtLookup(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, code := seedCard(t, tx, storeID, tenantID, StatusPending, "100.00")

	err := checkoutRedeem(t, tx, storeID, tenantID, code, decimal.RequireFromString("60.00"))

	require.Error(t, err, "checkout must refuse an unpaid gift card")
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardNotFound, ae.Code)
	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)))
}

// ── Schema guard (migration 000097) ────────────────────────────────────

func TestGiftCardStatusCheckConstraintExists(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)

	err := tx.Exec(`
		INSERT INTO gift_cards (id, tenant_id, store_id, code, initial_balance,
		                        current_balance, currency_code, status)
		VALUES (?, ?, ?, 'BOGUSSTATUSCODE', 100.00, 100.00, 'AUD', 'disable')`,
		uuid.New(), tenantID, storeID).Error

	require.Error(t, err, "gift_cards.status must be constrained to the known lifecycle states")
	assert.Contains(t, err.Error(), "gift_cards_status_valid")
}
