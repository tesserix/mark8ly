package giftcard

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// TestVoidLedgerFor covers the arithmetic behind the purchase-refund void.
// Lives in the default suite (no build tag) because CI runs `go test ./...`
// with no tags — the shortfall number the merchant will ask about must be
// guarded by a test that actually gates a merge.
func TestVoidLedgerFor(t *testing.T) {
	cases := []struct {
		name         string
		initial      string
		prevBalance  string
		prevStatus   GiftCardStatus
		wantWrite    bool
		wantAmount   string
		wantRedeemed string
	}{
		{
			// The case the whole feature exists for: the buyer spent $40
			// of a $100 card, then the merchant refunded the purchase.
			// $60 leaves the card; $40 is gone and is the merchant's loss.
			name: "partially spent card", initial: "100.00", prevBalance: "60.00",
			prevStatus: StatusActive,
			wantWrite:  true, wantAmount: "-60.00", wantRedeemed: "40.00",
		},
		{
			name: "untouched card", initial: "100.00", prevBalance: "100.00",
			prevStatus: StatusActive,
			wantWrite:  true, wantAmount: "-100.00", wantRedeemed: "0",
		},
		{
			// Fully spent before the refund landed: nothing leaves the
			// card, but the entire face value is the shortfall — the
			// largest loss possible, so it must still be recorded.
			name: "fully depleted card", initial: "100.00", prevBalance: "0",
			prevStatus: StatusDepleted,
			wantWrite:  true, wantAmount: "0", wantRedeemed: "100.00",
		},
		{
			name: "disabled card keeps its frozen balance as the void amount",
			initial: "80.00", prevBalance: "80.00", prevStatus: StatusDisabled,
			wantWrite: true, wantAmount: "-80.00", wantRedeemed: "0",
		},
		{
			// Never funded — current_balance is 0 because no money ever
			// arrived, NOT because it was spent. `initial - current` would
			// wrongly report the full face value as redeemed.
			name: "pending card was never funded", initial: "100.00", prevBalance: "0",
			prevStatus: StatusPending,
			wantWrite:  false, wantAmount: "0", wantRedeemed: "0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := voidLedgerFor(dec(c.initial), dec(c.prevBalance), c.prevStatus)
			assert.Equal(t, c.wantWrite, got.Write, "Write")
			assert.True(t, dec(c.wantAmount).Equal(got.Amount),
				"Amount = %s, want %s", got.Amount, c.wantAmount)
			assert.True(t, dec(c.wantRedeemed).Equal(got.Redeemed),
				"Redeemed = %s, want %s", got.Redeemed, c.wantRedeemed)
		})
	}
}

// TestVoidLedgerFor_NoteCarriesShortfall proves the merchant-facing number
// actually lands in the ledger note, and fits the note column (varchar 200).
func TestVoidLedgerFor_NoteCarriesShortfall(t *testing.T) {
	spent := voidLedgerFor(dec("100.00"), dec("60.00"), StatusActive)
	require.NotNil(t, spent.Note)
	assert.Contains(t, *spent.Note, "40")
	assert.LessOrEqual(t, len(*spent.Note), 200)

	untouched := voidLedgerFor(dec("100.00"), dec("100.00"), StatusActive)
	require.NotNil(t, untouched.Note)
	assert.False(t, strings.Contains(*untouched.Note, "redeemed"),
		"a card with nothing redeemed must not claim a shortfall: %q", *untouched.Note)
}
