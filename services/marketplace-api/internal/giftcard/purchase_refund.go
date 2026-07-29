package giftcard

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// PurchaseRefund describes a refund the payment provider issued against a
// gift card's OWN purchase, as reported by the webhook.
type PurchaseRefund struct {
	// RefundedTotal is the CUMULATIVE value refunded against the purchase
	// so far, in major currency units — not just this one refund. It
	// mirrors Stripe's `charge.amount_refunded`, which is cumulative.
	//
	// Carrying the running total rather than a single refund's value is
	// what makes sequential partial refunds accumulate exactly once each
	// (the card stores what it has already applied, and only the delta is
	// clawed back) and what makes a redelivered webhook a no-op.
	RefundedTotal decimal.Decimal

	// Full reports that the ENTIRE purchase has now been refunded. Only a
	// full refund voids the card. A partial refund removes exactly the
	// refunded value and leaves the remainder spendable.
	//
	// Never infer this from the arrival of a refund event: Stripe sends
	// `charge.refunded` for partial refunds too.
	Full bool
}

// cardRefundState is the card's state immediately BEFORE the refund is
// applied. Refunded is the cumulative value already clawed back from this
// card by earlier deliveries.
type cardRefundState struct {
	Initial  decimal.Decimal
	Balance  decimal.Decimal
	Status   GiftCardStatus
	Refunded decimal.Decimal
}

// purchaseRefundEffect is the complete decision about what a refund does to
// a card: the new row values, and the single ledger row (if any) to write.
type purchaseRefundEffect struct {
	// Apply is false when the refund changes nothing — a redelivered or
	// out-of-order webhook, or a card already voided. Callers must write
	// nothing at all in that case.
	Apply bool

	// Voided is true only for a full refund: the card becomes terminal.
	Voided bool

	// Clawback is the value actually removed from the balance, and
	// Shortfall is the refunded value that could not be removed because
	// the customer had already spent it. Clawback + Shortfall is the value
	// this delivery accounted for.
	Clawback  decimal.Decimal
	Shortfall decimal.Decimal

	NewBalance decimal.Decimal
	NewStatus  GiftCardStatus

	WriteLedger  bool
	LedgerAmount decimal.Decimal // negative = value leaving the card
	Note         *string
}

// purchaseRefundFor decides what a provider refund does to a gift card.
//
// The rule, in one line: remove exactly the refunded amount; only a FULL
// purchase refund voids the card.
//
// Stripe fires `charge.refunded` for partial refunds as well as full ones,
// so treating every refund event as a void destroyed the customer's whole
// remaining balance when a merchant refunded a fraction of the purchase.
// The delta arithmetic below is what keeps a $10 refund a $10 clawback.
func purchaseRefundFor(cur cardRefundState, in PurchaseRefund) purchaseRefundEffect {
	noop := purchaseRefundEffect{
		Apply:      false,
		Clawback:   decimal.Zero,
		Shortfall:  decimal.Zero,
		NewBalance: cur.Balance,
		NewStatus:  cur.Status,
	}

	// `refunded` is terminal. The card was already voided and the customer
	// already has their money back in real currency; touching it again
	// could only take value twice.
	if cur.Status == StatusRefunded {
		return noop
	}

	// The single idempotency guard. Because RefundedTotal is cumulative, a
	// redelivered webhook carries a total that has already been applied and
	// a stale out-of-order one carries a smaller total — both leave nothing
	// to do. This is the only thing standing between a redelivery and a
	// double clawback, so it must not be weakened into a "same event id"
	// check: two DIFFERENT Stripe events carry the same cumulative total
	// only when nothing new was refunded.
	delta := in.RefundedTotal.Sub(cur.Refunded)
	if delta.LessThanOrEqual(decimal.Zero) {
		return noop
	}

	// A full refund zeroes the card whatever the reported total says; a
	// partial removes the delta but can never take more than is there, so
	// the balance never goes negative.
	clawback := delta
	if in.Full || clawback.GreaterThan(cur.Balance) {
		clawback = cur.Balance
	}

	shortfall := delta.Sub(clawback)
	if shortfall.LessThan(decimal.Zero) {
		shortfall = decimal.Zero
	}

	newBalance := cur.Balance.Sub(clawback)

	eff := purchaseRefundEffect{
		Apply:      true,
		Voided:     in.Full,
		Clawback:   clawback,
		Shortfall:  shortfall,
		NewBalance: newBalance,
		NewStatus:  newStatusFor(cur.Status, newBalance, in.Full),
		// A `pending` card was never funded: its zero balance means "no
		// money ever arrived", not "all of it was spent". A ledger row
		// there would claim value that never existed.
		WriteLedger:  cur.Status != StatusPending,
		LedgerAmount: clawback.Neg(),
	}
	if eff.WriteLedger {
		eff.Note = refundNote(in.Full, delta, clawback, shortfall)
	}
	if !eff.WriteLedger {
		// Nothing was recorded, so nothing may be claimed as a loss either.
		eff.Shortfall = decimal.Zero
	}
	return eff
}

// newStatusFor decides where the card lands.
//
// A full refund is terminal: `refunded`, which DebitInTx and CreditInTx
// both refuse.
//
// A partial refund that happens to drain the balance lands on `depleted`,
// not `active` and not `refunded`:
//
//   - `depleted` is what DebitInTx itself writes when legitimate spending
//     takes the balance to zero, and this clawback is arithmetically the
//     same event — value left the card. Leaving it `active` at zero would
//     make it the only zero-balance `active` row in the system.
//   - `refunded` would be a lie (the purchase was NOT fully refunded) and,
//     worse, permanent: CreditInTx refuses refunded cards, so a later
//     legitimate credit — an order paid with this card being returned —
//     would be rejected and the customer's money stranded.
//   - `depleted` keeps that door open. CreditInTx flips `depleted` back to
//     `active` when the balance goes positive again, so a later credit
//     restores spendability instead of stranding value.
//
// Every other status is preserved. A `disabled` card stays disabled — its
// balance is frozen by the merchant, and a refund must not silently
// re-enable it. A `pending` card was never funded, so there is nothing to
// deplete.
func newStatusFor(current GiftCardStatus, newBalance decimal.Decimal, full bool) GiftCardStatus {
	if full {
		return StatusRefunded
	}
	if current == StatusActive && newBalance.IsZero() {
		return StatusDepleted
	}
	return current
}

// refundNote spells out the merchant-facing numbers for the ledger row.
//
// The shortfall is value the customer already spent before the refund
// landed: unrecoverable, because the goods are gone and the purchase money
// is going back. It is named in the note rather than buried in the
// arithmetic, because it is the number the merchant will ask about.
//
// All four forms stay well inside gift_card_transactions.note (varchar 200).
func refundNote(full bool, delta, clawback, shortfall decimal.Decimal) *string {
	var note string
	switch {
	case full && shortfall.IsPositive():
		note = fmt.Sprintf("purchase refunded — card voided; %s already redeemed and not recoverable",
			shortfall.StringFixed(2))
	case full:
		note = "purchase refunded — card voided"
	case shortfall.IsPositive():
		note = fmt.Sprintf("partial purchase refund of %s — %s removed from balance, %s already redeemed and not recoverable",
			delta.StringFixed(2), clawback.StringFixed(2), shortfall.StringFixed(2))
	default:
		note = fmt.Sprintf("partial purchase refund of %s removed from balance", delta.StringFixed(2))
	}
	return &note
}
