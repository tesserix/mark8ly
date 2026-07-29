package orderrefund

import (
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// A refunded order can owe the customer money on two ledgers at once.
//
// At checkout the gift-card amount is subtracted from grand_total BEFORE the
// order row is written, so grand_total is the gateway-charged amount only.
// A $100 basket paid with a $40 gift card is persisted as a $60 order with
// $40 of `redeem` rows against the card. Refunding "the order" through the
// gateway therefore returns $60 and silently keeps the $40.
//
// The rule, in one line: **real money comes back before store credit does.**
// Every refund fills the gateway side first and only spills onto the gift
// card once the gateway portion is exhausted.

// RefundSplit is one refund divided between the two ledgers.
type RefundSplit struct {
	Gateway  decimal.Decimal
	GiftCard decimal.Decimal
}

// Total is what the customer gets back in this refund, across both ledgers.
func (s RefundSplit) Total() decimal.Decimal { return s.Gateway.Add(s.GiftCard) }

// RefundLedgerState is the persisted accounting a split is derived from.
// Every field is a SUM over committed rows — nothing is carried in memory
// between refunds, which is what stops multi-partial accounting drifting.
type RefundLedgerState struct {
	// GatewayCharged is min(grand_total, captured_total): the ceiling on
	// what the provider can give back.
	GatewayCharged decimal.Decimal
	// GatewayReturned is orders.refunded_amount — gateway money only.
	GatewayReturned decimal.Decimal
	// GiftCardApplied is Σ of the order's `redeem` gift-card ledger rows.
	GiftCardApplied decimal.Decimal
	// GiftCardReturned is Σ of the order's `refund` gift-card ledger rows.
	GiftCardReturned decimal.Decimal
	// InFlight is Σ amount of still-pending refund ledger rows for the
	// order — money a concurrent caller has reserved but not yet settled.
	InFlight decimal.Decimal
}

// available returns the unclaimed capacity on each side, with in-flight
// refunds consuming capacity under the same gateway-first rule they will
// settle under.
func (s RefundLedgerState) available() (gateway, giftCard decimal.Decimal) {
	gateway = clampZero(s.GatewayCharged.Sub(s.GatewayReturned))
	giftCard = clampZero(s.GiftCardApplied.Sub(s.GiftCardReturned))

	inFlightGateway := decimal.Min(clampZero(s.InFlight), gateway)
	gateway = gateway.Sub(inFlightGateway)
	giftCard = clampZero(giftCard.Sub(clampZero(s.InFlight).Sub(inFlightGateway)))
	return gateway, giftCard
}

// Remaining is the full outstanding refundable balance across both ledgers —
// what a nil-Amount ("refund everything left") command resolves to.
func (s RefundLedgerState) Remaining() decimal.Decimal {
	gateway, giftCard := s.available()
	return gateway.Add(giftCard)
}

// SplitRefund divides amount gateway-first, refusing anything the order
// cannot cover with apperrors.ErrRefundExceedsTotal.
func SplitRefund(s RefundLedgerState, amount decimal.Decimal) (RefundSplit, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return RefundSplit{}, apperrors.ValidationFailed("amount", "refund amount must be positive")
	}

	gatewayAvail, giftCardAvail := s.available()

	gateway := decimal.Min(amount, gatewayAvail)
	giftCard := decimal.Min(amount.Sub(gateway), giftCardAvail)

	if gateway.Add(giftCard).LessThan(amount) {
		return RefundSplit{}, apperrors.ErrRefundExceedsTotal
	}
	return RefundSplit{Gateway: gateway, GiftCard: giftCard}, nil
}

// GiftCardCreditTarget answers "how much of this order's gift-card portion
// SHOULD have been returned by now", from totals alone.
//
// This — not the amount of the refund being processed — is what drives the
// credit, and it is why the credit is idempotent. A redelivered webhook, a
// re-entered saga and the pending-refund sweeper all recompute the same
// target from the same committed rows; whatever has already been credited is
// subtracted, so a replay resolves to zero rather than to a second credit.
//
//	returnedTotal  Σ amount of the order's succeeded refund ledger rows
//	               (each row's amount is the TOTAL of that refund, both sides)
//	gatewayCharged the gateway-side ceiling — everything up to it is real money
//	applied        Σ of the order's `redeem` gift-card rows
//	already        Σ of the order's `refund` gift-card rows
func GiftCardCreditTarget(returnedTotal, gatewayCharged, applied, already decimal.Decimal) decimal.Decimal {
	pastGateway := clampZero(returnedTotal.Sub(gatewayCharged))
	owed := decimal.Min(pastGateway, clampZero(applied))
	return clampZero(owed.Sub(already))
}

func clampZero(v decimal.Decimal) decimal.Decimal {
	if v.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	return v
}
