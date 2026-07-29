package giftcard

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// voidLedger describes the single ledger row a purchase-refund void writes.
//
// The refund reverses the purchase, so the row removes whatever value was
// still sitting on the card (Amount = -PreviousBalance) and lands the card
// on a zero balance — which keeps the ledger self-consistent: purchase
// (+initial) + redeems (−spent) + this row (−remaining) sums to 0.
//
// Redeemed is the merchant's actual loss: value the customer already spent
// before the refund landed. It is unrecoverable — the goods are gone and
// the purchase money is going back — so it is spelled out in Note rather
// than buried in the arithmetic.
type voidLedger struct {
	Write    bool
	Amount   decimal.Decimal
	Redeemed decimal.Decimal
	Note     *string
}

// voidLedgerFor computes the ledger row for voiding a card whose purchase
// was refunded, from the card's state immediately BEFORE the void.
//
// A `pending` card was never funded: its zero balance means "no money ever
// arrived", not "all of it was spent". Deriving the shortfall as
// initial − current there would report the entire face value as redeemed,
// which is a lie, so pending cards get voided with no ledger row at all.
func voidLedgerFor(initial, previousBalance decimal.Decimal, previousStatus GiftCardStatus) voidLedger {
	if previousStatus == StatusPending {
		return voidLedger{Write: false, Amount: decimal.Zero, Redeemed: decimal.Zero}
	}

	redeemed := initial.Sub(previousBalance)
	if redeemed.LessThan(decimal.Zero) {
		// Defensive: a manually adjusted card can carry more than it was
		// issued with. That is a credit, not a shortfall.
		redeemed = decimal.Zero
	}

	note := "purchase refunded — card voided"
	if redeemed.GreaterThan(decimal.Zero) {
		note = fmt.Sprintf("purchase refunded — card voided; %s already redeemed and not recoverable",
			redeemed.StringFixed(2))
	}

	return voidLedger{
		Write:    true,
		Amount:   previousBalance.Neg(),
		Redeemed: redeemed,
		Note:     &note,
	}
}
