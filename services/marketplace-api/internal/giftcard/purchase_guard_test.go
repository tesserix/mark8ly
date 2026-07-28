package giftcard

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ── The bug these tests pin ────────────────────────────────────────────
// A storefront buyer could create a gift card, abandon the Stripe
// checkout, read the full code back from GET /account/gift-cards, and
// spend it. `pending` was asserted to be non-redeemable by four comments
// and enforced by nothing.
//
// The repository-level half of the fix (the `status = 'active'` predicate
// in DebitInTx) is exercised in repository_integration_test.go, which
// needs real SQL. These are the pure-Go halves.

// ── newPendingCard ─────────────────────────────────────────────────────

func TestNewPendingCard_IsCreatedUnfunded(t *testing.T) {
	amount := decimal.RequireFromString("500.00")
	in := PurchaseInput{
		TenantID:       uuid.New(),
		StoreID:        uuid.New(),
		InitialBalance: amount,
		CurrencyCode:   "aud",
		PurchaserEmail: "attacker@example.com",
	}

	gc := newPendingCard(in, "CODE", "stripe", time.Now())

	assert.True(t, gc.CurrentBalance.IsZero(),
		"an unpaid gift card must carry zero spendable balance — got %s", gc.CurrentBalance)
	assert.True(t, amount.Equal(gc.InitialBalance),
		"initial_balance records what the buyer agreed to pay")
	assert.Equal(t, StatusPending, gc.Status)
	assert.Equal(t, "AUD", gc.CurrencyCode)
}

// ── CheckBalance ───────────────────────────────────────────────────────

func TestCheckBalance_RejectsNonRedeemableStatuses(t *testing.T) {
	storeID := uuid.New()

	for _, tc := range []struct {
		name   string
		status GiftCardStatus
	}{
		{"pending — payment never captured", StatusPending},
		{"refunded — money returned to the buyer", StatusRefunded},
		{"disabled — merchant killed the card", StatusDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil, &stubRepo{card: &GiftCard{
				ID:             uuid.New(),
				Code:           "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
				Status:         tc.status,
				CurrentBalance: decimal.RequireFromString("500.00"),
				CurrencyCode:   "AUD",
			}}, nil)

			res, err := svc.CheckBalance(context.Background(), storeID, "ABCD-EFGH-IJKL-MNOP-QRST-UVWX-YZ")

			require.Error(t, err, "a %s card must not report a spendable balance", tc.status)
			assert.Nil(t, res)
			var ae *apperrors.Error
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, apperrors.CodeGiftCardNotFound, ae.Code)
		})
	}
}

func TestCheckBalance_ActiveCardStillWorks(t *testing.T) {
	// Guards against over-correction: the legitimate path must survive.
	balance := decimal.RequireFromString("42.50")
	svc := NewService(nil, &stubRepo{card: &GiftCard{
		ID:             uuid.New(),
		Code:           "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		Status:         StatusActive,
		CurrentBalance: balance,
		CurrencyCode:   "AUD",
	}}, nil)

	res, err := svc.CheckBalance(context.Background(), uuid.New(), "ABCD-EFGH-IJKL-MNOP-QRST-UVWX-YZ")

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, balance.Equal(res.CurrentBalance))
	assert.Equal(t, StatusActive, res.Status)
}

// ── stubRepo ───────────────────────────────────────────────────────────

// stubRepo serves one card from GetByCode and panics on anything else, so
// a test that accidentally exercises an unstubbed path fails loudly rather
// than passing on a zero value.
type stubRepo struct {
	Repository
	card *GiftCard
}

func (r *stubRepo) GetByCode(_ context.Context, _ *gorm.DB, _ uuid.UUID, _ string) (*GiftCard, error) {
	if r.card == nil {
		return nil, apperrors.NotFound("gift card")
	}
	return r.card, nil
}
