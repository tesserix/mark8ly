//go:build integration

package giftcard

import (
	"context"
	"time"

	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These cover the new SetStatus write path — the first thing in this
// codebase that ever writes StatusDisabled — and re-prove the two existing
// security defenses (DebitInTx's `status = 'active'` predicate, and
// CheckBalance's disabled/pending/refunded rejection) against cards that
// reach `disabled` via this new path, not just hand-seeded rows.
//
// Run: TEST_DATABASE_URL=... go test -tags=integration ./internal/giftcard/...

// seedCardWithExpiry inserts a card in an explicit state with a specific
// expires_at, since seedCard doesn't take an expiry parameter.
func seedCardWithExpiry(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID, status GiftCardStatus, balance string, expiresAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	code := "CODE" + id.String()[:8]
	require.NoError(t, tx.Exec(`
		INSERT INTO gift_cards (id, tenant_id, store_id, code, initial_balance,
		                        current_balance, currency_code, status, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, 'AUD', ?, ?)`,
		id, tenantID, storeID, code,
		decimal.RequireFromString("100.00"), decimal.RequireFromString(balance), status, expiresAt).Error)
	return id
}

// seedSecondStoreForTenant inserts a second store row under the same
// tenant_id as an existing seedStore call, so tests can prove store-scoping
// independently of tenant-scoping (Global Constraint 5: "tenant- OR
// store-scoped").
func seedSecondStoreForTenant(t *testing.T, tx *gorm.DB, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	storeID := uuid.New()
	require.NoError(t, tx.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code,
		                    timezone, status, storefront_customer_portal_secret)
		VALUES (?, ?, ?, 'Test Store B', 'AU', 'AUD', 'Australia/Sydney', 'active', 'secret')`,
		storeID, tenantID, "gc-b-"+storeID.String()[:8]).Error)
	return storeID
}

func statusOf(t *testing.T, tx *gorm.DB, id uuid.UUID) GiftCardStatus {
	t.Helper()
	var got GiftCardStatus
	require.NoError(t, tx.Raw(`SELECT status FROM gift_cards WHERE id = ?`, id).Scan(&got).Error)
	return got
}

// 1. Active → disable succeeds.
func TestSetStatus_DisablesActiveCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "50.00")
	repo := NewRepository()

	gc, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)

	require.NoError(t, err)
	assert.Equal(t, StatusDisabled, gc.Status)
	assert.True(t, decimal.RequireFromString("50.00").Equal(gc.CurrentBalance))
	assert.True(t, decimal.RequireFromString("50.00").Equal(balanceOf(t, tx, cardID)))
}

// 2. Disabled → enable succeeds.
func TestSetStatus_EnablesDisabledCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusDisabled, "50.00")
	repo := NewRepository()

	gc, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusActive)

	require.NoError(t, err)
	assert.Equal(t, StatusActive, gc.Status)
}

// 3. Round trip must leave the balance byte-identical at every step.
// Must prove red→green: fails to compile before SetStatus exists.
func TestSetStatus_RoundTrip_BalanceByteIdentical(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	original := decimal.RequireFromString("73.42")
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "73.42")
	repo := NewRepository()

	assert.True(t, original.Equal(balanceOf(t, tx, cardID)), "before")

	disabled, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)
	require.NoError(t, err)
	assert.True(t, original.Equal(disabled.CurrentBalance), "after disable (returned)")
	assert.True(t, original.Equal(balanceOf(t, tx, cardID)), "after disable (read)")

	enabled, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusActive)
	require.NoError(t, err)
	assert.True(t, original.Equal(enabled.CurrentBalance), "after enable (returned)")
	assert.True(t, original.Equal(balanceOf(t, tx, cardID)), "after enable (read)")
}

// 4. Pending card cannot be disabled — wrong source status.
func TestSetStatus_RefusesPendingCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusPending, "100.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeInvalidTransition, ae.Code)
	assert.Equal(t, StatusPending, statusOf(t, tx, cardID))
	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)))
}

// 5. Depleted card cannot be disabled — wrong source status.
func TestSetStatus_RefusesDepletedCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusDepleted, "0.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeInvalidTransition, ae.Code)
	assert.Equal(t, StatusDepleted, statusOf(t, tx, cardID))
	assert.True(t, decimal.RequireFromString("0.00").Equal(balanceOf(t, tx, cardID)))
}

// 6. Refunded card cannot be disabled — wrong source status.
func TestSetStatus_RefusesRefundedCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusRefunded, "0.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeInvalidTransition, ae.Code)
	assert.Equal(t, StatusRefunded, statusOf(t, tx, cardID))
	assert.True(t, decimal.RequireFromString("0.00").Equal(balanceOf(t, tx, cardID)))
}

// 7. An expired active card cannot be disabled — expiry is a more truthful
// reason than "wrong status" when the source status was otherwise legal.
// Must prove red→green.
func TestSetStatus_RefusesExpiredActiveCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	past := time.Now().Add(-24 * time.Hour)
	cardID := seedCardWithExpiry(t, tx, storeID, tenantID, StatusActive, "100.00", past)
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardExpired, ae.Code)
	assert.NotEqual(t, apperrors.CodeInvalidTransition, ae.Code)
}

// 8. Symmetric case: an expired disabled card cannot be enabled either.
func TestSetStatus_RefusesExpiredDisabledCard(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	past := time.Now().Add(-24 * time.Hour)
	cardID := seedCardWithExpiry(t, tx, storeID, tenantID, StatusDisabled, "100.00", past)
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusActive)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardExpired, ae.Code)
}

// 9. Disabling an already-disabled card is idempotent success, not an
// error. Must prove red→green: this assertion only passes if the
// implementation actually takes the idempotent branch, not the
// CodeInvalidTransition branch.
func TestSetStatus_SameStatusDisableIsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusDisabled, "42.00")
	repo := NewRepository()

	gc, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)

	require.NoError(t, err, "disabling an already-disabled card must be idempotent, not an error")
	assert.Equal(t, StatusDisabled, gc.Status)
	assert.True(t, decimal.RequireFromString("42.00").Equal(gc.CurrentBalance))
	assert.True(t, decimal.RequireFromString("42.00").Equal(balanceOf(t, tx, cardID)))
}

// 10. Mirror of #9 for already-active.
func TestSetStatus_SameStatusEnableIsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "42.00")
	repo := NewRepository()

	gc, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusActive)

	require.NoError(t, err, "enabling an already-active card must be idempotent, not an error")
	assert.Equal(t, StatusActive, gc.Status)
	assert.True(t, decimal.RequireFromString("42.00").Equal(gc.CurrentBalance))
	assert.True(t, decimal.RequireFromString("42.00").Equal(balanceOf(t, tx, cardID)))
}

// 11. Cross-tenant lookup must return NotFound, not leak that the card
// exists under a different tenant/store — and must not mutate the row.
// Must prove red→green.
func TestSetStatus_DifferentTenantCardNotFoundAndUnaffected(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "88.00")
	repo := NewRepository()

	otherTenantID := uuid.New()
	otherStoreID := uuid.New()

	_, err := repo.SetStatus(tx, cardID, otherTenantID, otherStoreID, StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeNotFound, ae.Code)
	assert.Equal(t, StatusActive, statusOf(t, tx, cardID), "card must be unaffected by a cross-tenant attempt")
	assert.True(t, decimal.RequireFromString("88.00").Equal(balanceOf(t, tx, cardID)))
}

// 11b. Same tenant, different store must also be refused as NotFound — the
// realistic multi-store-merchant case. Every other cross-scope test above
// varies tenant (and usually store) together; this proves store_id alone is
// enforced, not just tenant_id.
func TestSetStatus_DifferentStoreSameTenantNotFoundAndUnaffected(t *testing.T) {
	tx := testdb.NewTx(t)
	storeA, tenantID := seedStore(t, tx)
	storeB := seedSecondStoreForTenant(t, tx, tenantID)
	cardID, _ := seedCard(t, tx, storeA, tenantID, StatusActive, "88.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeB, StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeNotFound, ae.Code)
	assert.Equal(t, StatusActive, statusOf(t, tx, cardID), "card must be unaffected by a different-store attempt")
	assert.True(t, decimal.RequireFromString("88.00").Equal(balanceOf(t, tx, cardID)))
}

// 12. Unknown card id is NotFound.
func TestSetStatus_UnknownCardIsNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	_, tenantID := seedStore(t, tx)
	repo := NewRepository()

	_, err := repo.SetStatus(tx, uuid.New(), tenantID, uuid.New(), StatusDisabled)

	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeNotFound, ae.Code)
}

// 13. A card disabled via SetStatus must be unspendable through DebitInTx —
// the end-to-end proof that the new write path feeds the existing
// `status = 'active'` defense (6fa9d885), not a hand-seeded row.
// Must prove red→green: see the task report for the temporary revert of
// DebitInTx's status predicate used to prove this test actually catches a
// regression.
func TestDisabledCard_ViaSetStatus_IsUnspendable(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "100.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)
	require.NoError(t, err)

	_, err = repo.DebitInTx(tx, cardID, decimal.RequireFromString("10.00"), uuid.New(), tenantID)

	require.Error(t, err, "a card disabled via SetStatus must not be redeemable")
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardNotRedeemable, ae.Code)
	assert.True(t, decimal.RequireFromString("100.00").Equal(balanceOf(t, tx, cardID)),
		"balance must be untouched after a rejected debit")
}

// 14. Same setup, but through the CheckBalance truthfulness guard rather
// than the DebitInTx security boundary.
func TestDisabledCard_ViaSetStatus_CheckBalanceRejects(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, code := seedCard(t, tx, storeID, tenantID, StatusActive, "100.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)
	require.NoError(t, err)

	svc := NewService(tx, NewRepository(), nil)
	_, err = svc.CheckBalance(context.Background(), storeID, code)

	require.Error(t, err, "a card disabled via SetStatus must not report a spendable balance")
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeGiftCardNotFound, ae.Code)
}

// 15. The full product promise, end to end: disable stops spending,
// re-enable restores spending at the same balance the card had when it was
// disabled — not the balance it started with.
func TestReenabledCard_ViaSetStatus_IsSpendableAgainAtSameBalance(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	cardID, _ := seedCard(t, tx, storeID, tenantID, StatusActive, "50.00")
	repo := NewRepository()

	_, err := repo.SetStatus(tx, cardID, tenantID, storeID, StatusDisabled)
	require.NoError(t, err)

	_, err = repo.DebitInTx(tx, cardID, decimal.RequireFromString("10.00"), uuid.New(), tenantID)
	require.Error(t, err, "disabled card must not be spendable")

	_, err = repo.SetStatus(tx, cardID, tenantID, storeID, StatusActive)
	require.NoError(t, err)

	after, err := repo.DebitInTx(tx, cardID, decimal.RequireFromString("20.00"), uuid.New(), tenantID)
	require.NoError(t, err, "re-enabled card must be spendable again")
	assert.True(t, decimal.RequireFromString("30.00").Equal(after))
	assert.True(t, decimal.RequireFromString("30.00").Equal(balanceOf(t, tx, cardID)))
}
