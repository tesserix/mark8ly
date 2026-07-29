package giftcard

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// TestPurchaseRefundFor covers the arithmetic behind a provider refund
// issued against a gift card's OWN purchase.
//
// Lives in the default suite (no build tag) because CI runs `go test ./...`
// with no tags — the full-vs-partial decision decides whether a customer
// keeps or loses their remaining balance, so it must be guarded by a test
// that actually gates a merge.
func TestPurchaseRefundFor(t *testing.T) {
	cases := []struct {
		name string

		initial   string
		balance   string
		status    GiftCardStatus
		refunded  string // cumulative already applied to this card
		refundNow string // cumulative reported by the provider
		full      bool

		wantApply     bool
		wantVoided    bool
		wantClawback  string
		wantShortfall string
		wantBalance   string
		wantStatus    GiftCardStatus
		wantWrite     bool
		wantLedger    string
	}{
		{
			// THE bug. A merchant refunding $10 of a $100 gift-card purchase
			// must remove $10, not destroy the card. Before this fix every
			// `charge.refunded` — which Stripe also sends for partials — ran
			// the full-void path and burned the whole $100.
			name:    "partial refund of an unspent card removes only the refunded amount",
			initial: "100.00", balance: "100.00", status: StatusActive,
			refunded: "0", refundNow: "10.00", full: false,
			wantApply: true, wantVoided: false,
			wantClawback: "10.00", wantShortfall: "0",
			wantBalance: "90.00", wantStatus: StatusActive,
			wantWrite: true, wantLedger: "-10.00",
		},
		{
			// The customer already spent below the refunded amount. Zero the
			// balance and record the shortfall — never go negative.
			name:    "partial refund larger than the remaining balance zeroes it and records the shortfall",
			initial: "100.00", balance: "30.00", status: StatusActive,
			refunded: "0", refundNow: "50.00", full: false,
			wantApply: true, wantVoided: false,
			wantClawback: "30.00", wantShortfall: "20.00",
			wantBalance: "0", wantStatus: StatusDepleted,
			wantWrite: true, wantLedger: "-30.00",
		},
		{
			// Stripe's amount_refunded is CUMULATIVE, so the second $20
			// refund arrives as 30.00 total, not 20.00. Clawing back the
			// reported total again would remove $50 for $30 refunded.
			name:    "second sequential partial claws back only the delta",
			initial: "100.00", balance: "90.00", status: StatusActive,
			refunded: "10.00", refundNow: "30.00", full: false,
			wantApply: true, wantVoided: false,
			wantClawback: "20.00", wantShortfall: "0",
			wantBalance: "70.00", wantStatus: StatusActive,
			wantWrite: true, wantLedger: "-20.00",
		},
		{
			// Webhook redelivery: identical cumulative total, nothing left
			// to apply. This is the guard that stops a double clawback.
			name:    "redelivered event with the same cumulative total is a no-op",
			initial: "100.00", balance: "90.00", status: StatusActive,
			refunded: "10.00", refundNow: "10.00", full: false,
			wantApply:    false,
			wantClawback: "0", wantShortfall: "0",
			wantBalance: "90.00", wantStatus: StatusActive,
			wantWrite: false, wantLedger: "0",
		},
		{
			// Out-of-order delivery: an older event carrying a smaller
			// cumulative total must not un-refund or re-refund anything.
			name:    "out-of-order event with a lower cumulative total is a no-op",
			initial: "100.00", balance: "70.00", status: StatusActive,
			refunded: "30.00", refundNow: "10.00", full: false,
			wantApply:    false,
			wantClawback: "0", wantShortfall: "0",
			wantBalance: "70.00", wantStatus: StatusActive,
			wantWrite: false, wantLedger: "0",
		},
		{
			// A partial that happens to drain the card lands on `depleted`,
			// not `active` and not `refunded`. See the docs on newStatusFor.
			name:    "partial refund landing on exactly zero leaves the card depleted",
			initial: "100.00", balance: "40.00", status: StatusActive,
			refunded: "0", refundNow: "40.00", full: false,
			wantApply: true, wantVoided: false,
			wantClawback: "40.00", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusDepleted,
			wantWrite: true, wantLedger: "-40.00",
		},
		{
			// Already fully spent by the customer, then partially refunded:
			// nothing to remove, but the loss is real and must be recorded.
			name:    "partial refund of an already depleted card removes nothing and never goes negative",
			initial: "100.00", balance: "0", status: StatusDepleted,
			refunded: "0", refundNow: "15.00", full: false,
			wantApply: true, wantVoided: false,
			wantClawback: "0", wantShortfall: "15.00",
			wantBalance: "0", wantStatus: StatusDepleted,
			wantWrite: true, wantLedger: "0",
		},
		{
			// A disabled card's balance is frozen, not destroyed. A partial
			// refund still removes value but must not silently re-enable it.
			name:    "partial refund of a disabled card keeps it disabled",
			initial: "80.00", balance: "80.00", status: StatusDisabled,
			refunded: "0", refundNow: "20.00", full: false,
			wantApply: true, wantVoided: false,
			wantClawback: "20.00", wantShortfall: "0",
			wantBalance: "60.00", wantStatus: StatusDisabled,
			wantWrite: true, wantLedger: "-20.00",
		},

		// ── The full-refund path, unchanged from 4039e8cb ─────────────────

		{
			name:    "full refund of a partially spent card still voids it",
			initial: "100.00", balance: "60.00", status: StatusActive,
			refunded: "0", refundNow: "100.00", full: true,
			wantApply: true, wantVoided: true,
			wantClawback: "60.00", wantShortfall: "40.00",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: true, wantLedger: "-60.00",
		},
		{
			name:    "full refund of an untouched card",
			initial: "100.00", balance: "100.00", status: StatusActive,
			refunded: "0", refundNow: "100.00", full: true,
			wantApply: true, wantVoided: true,
			wantClawback: "100.00", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: true, wantLedger: "-100.00",
		},
		{
			// Fully spent before the refund landed: nothing leaves the card,
			// but the entire face value is the shortfall — the largest loss
			// possible, so it must still be recorded.
			name:    "full refund of a fully depleted card",
			initial: "100.00", balance: "0", status: StatusDepleted,
			refunded: "0", refundNow: "100.00", full: true,
			wantApply: true, wantVoided: true,
			wantClawback: "0", wantShortfall: "100.00",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: true, wantLedger: "0",
		},
		{
			name:    "full refund of a disabled card voids its frozen balance",
			initial: "80.00", balance: "80.00", status: StatusDisabled,
			refunded: "0", refundNow: "80.00", full: true,
			wantApply: true, wantVoided: true,
			wantClawback: "80.00", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: true, wantLedger: "-80.00",
		},
		{
			// Never funded — current_balance is 0 because no money ever
			// arrived, NOT because it was spent. Deriving a shortfall here
			// would wrongly report the full face value as redeemed.
			name:    "pending card was never funded",
			initial: "100.00", balance: "0", status: StatusPending,
			refunded: "0", refundNow: "100.00", full: true,
			wantApply: true, wantVoided: true,
			wantClawback: "0", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: false, wantLedger: "0",
		},
		{
			// Partials first, then the merchant refunds the rest. The delta
			// is what is left, and the shortfall must not double-count the
			// value already clawed back by the earlier partials.
			name:    "partials then a full refund charges no shortfall for value already clawed back",
			initial: "100.00", balance: "70.00", status: StatusActive,
			refunded: "30.00", refundNow: "100.00", full: true,
			wantApply: true, wantVoided: true,
			wantClawback: "70.00", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: true, wantLedger: "-70.00",
		},
		{
			// `refunded` is terminal: the card was already voided and the
			// customer already got their money back in real currency.
			name:    "an already voided card is never touched again",
			initial: "100.00", balance: "0", status: StatusRefunded,
			refunded: "100.00", refundNow: "100.00", full: true,
			wantApply:    false,
			wantClawback: "0", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: false, wantLedger: "0",
		},
		{
			// Stripe permits refunding MORE than was captured (dispute and
			// goodwill flows), so a voided card can still receive an event
			// whose cumulative total exceeds what was already applied. The
			// delta is positive, so only the terminal-status guard stops it
			// — without that guard this writes a phantom ledger row
			// claiming a shortfall on a card that has nothing left.
			name:    "an over-refund arriving after the void is still a no-op",
			initial: "100.00", balance: "0", status: StatusRefunded,
			refunded: "100.00", refundNow: "120.00", full: true,
			wantApply:    false,
			wantClawback: "0", wantShortfall: "0",
			wantBalance: "0", wantStatus: StatusRefunded,
			wantWrite: false, wantLedger: "0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := purchaseRefundFor(
				cardRefundState{
					Initial:  dec(c.initial),
					Balance:  dec(c.balance),
					Status:   c.status,
					Refunded: dec(c.refunded),
				},
				PurchaseRefund{RefundedTotal: dec(c.refundNow), Full: c.full},
			)

			assert.Equal(t, c.wantApply, got.Apply, "Apply")
			assert.Equal(t, c.wantVoided, got.Voided, "Voided")
			assert.True(t, dec(c.wantClawback).Equal(got.Clawback),
				"Clawback = %s, want %s", got.Clawback, c.wantClawback)
			assert.True(t, dec(c.wantShortfall).Equal(got.Shortfall),
				"Shortfall = %s, want %s", got.Shortfall, c.wantShortfall)
			assert.True(t, dec(c.wantBalance).Equal(got.NewBalance),
				"NewBalance = %s, want %s", got.NewBalance, c.wantBalance)
			assert.Equal(t, c.wantStatus, got.NewStatus, "NewStatus")
			assert.Equal(t, c.wantWrite, got.WriteLedger, "WriteLedger")
			assert.True(t, dec(c.wantLedger).Equal(got.LedgerAmount),
				"LedgerAmount = %s, want %s", got.LedgerAmount, c.wantLedger)

			assert.False(t, got.NewBalance.IsNegative(),
				"a gift card balance must never go negative; got %s", got.NewBalance)
		})
	}
}

// TestPurchaseRefundFor_NoteCarriesTheNumbers proves the merchant-facing
// figures actually land in the ledger note, and that the note fits the
// column (varchar 200).
func TestPurchaseRefundFor_NoteCarriesTheNumbers(t *testing.T) {
	active := func(bal, refunded string) cardRefundState {
		return cardRefundState{
			Initial: dec("100.00"), Balance: dec(bal),
			Status: StatusActive, Refunded: dec(refunded),
		}
	}

	t.Run("full refund on a partly spent card names the shortfall", func(t *testing.T) {
		got := purchaseRefundFor(active("60.00", "0"), PurchaseRefund{RefundedTotal: dec("100.00"), Full: true})
		require.NotNil(t, got.Note)
		assert.Contains(t, *got.Note, "voided")
		assert.Contains(t, *got.Note, "40.00")
		assert.LessOrEqual(t, len(*got.Note), 200)
	})

	t.Run("full refund with nothing redeemed claims no shortfall", func(t *testing.T) {
		got := purchaseRefundFor(active("100.00", "0"), PurchaseRefund{RefundedTotal: dec("100.00"), Full: true})
		require.NotNil(t, got.Note)
		assert.False(t, strings.Contains(*got.Note, "redeemed"),
			"a card with nothing redeemed must not claim a shortfall: %q", *got.Note)
	})

	t.Run("partial refund note says partial, not voided", func(t *testing.T) {
		got := purchaseRefundFor(active("100.00", "0"), PurchaseRefund{RefundedTotal: dec("10.00")})
		require.NotNil(t, got.Note)
		assert.Contains(t, *got.Note, "partial")
		assert.Contains(t, *got.Note, "10.00")
		assert.NotContains(t, *got.Note, "voided",
			"a partial refund does not void the card, so the ledger must not say it did: %q", *got.Note)
		assert.LessOrEqual(t, len(*got.Note), 200)
	})

	t.Run("partial refund beyond the balance names both numbers", func(t *testing.T) {
		got := purchaseRefundFor(active("30.00", "0"), PurchaseRefund{RefundedTotal: dec("50.00")})
		require.NotNil(t, got.Note)
		assert.Contains(t, *got.Note, "30.00", "value actually removed")
		assert.Contains(t, *got.Note, "20.00", "unrecoverable shortfall")
		assert.LessOrEqual(t, len(*got.Note), 200)
	})
}
