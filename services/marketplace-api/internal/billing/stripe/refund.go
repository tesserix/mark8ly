package stripe

import (
	"context"
	"errors"
	"net/http"

	sdk "github.com/stripe/stripe-go/v82"
)

// Refund is our sanitised representation of a Stripe Refund object.
type Refund struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	ChargeID string `json:"charge_id"`
	Reason   string `json:"reason"`
}

// CreateRefundInput holds the parameters for creating a Stripe Refund.
type CreateRefundInput struct {
	// ChargeID is the Stripe charge to refund. Mutually exclusive with PaymentIntentID.
	ChargeID string
	// PaymentIntentID may be used instead of ChargeID for PI-based charges.
	PaymentIntentID string
	Amount          *int64 // nil = full refund
	Reason          string // "duplicate" | "fraudulent" | "requested_by_customer"
	IdempotencyKey  string
}

// CreateRefund issues a Stripe Refund for the given charge or payment intent.
func CreateRefund(ctx context.Context, c *Client, in CreateRefundInput) (*Refund, error) {
	params := &sdk.RefundCreateParams{}
	if in.ChargeID != "" {
		params.Charge = sdk.String(in.ChargeID)
	}
	if in.PaymentIntentID != "" {
		params.PaymentIntent = sdk.String(in.PaymentIntentID)
	}
	if in.Amount != nil {
		params.Amount = in.Amount
	}
	if in.Reason != "" {
		params.Reason = sdk.String(in.Reason)
	}
	if in.IdempotencyKey != "" {
		params.SetIdempotencyKey(in.IdempotencyKey)
	}

	r, err := c.sdk.V1Refunds.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapRefund(r), nil
}

// Charge is our sanitised representation of a Stripe Charge object.
// Only the fields needed by the refund 14-day gate and fraud guard are included.
type Charge struct {
	ID              string `json:"id"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	Created         int64  `json:"created"` // Unix timestamp — authoritative for 14-day gate
	CardFingerprint string `json:"card_fingerprint,omitempty"`
}

// GetCharge retrieves a Stripe Charge by ID.
// Returns ErrNotFound when the Stripe API returns HTTP 404.
func GetCharge(ctx context.Context, c *Client, id string) (*Charge, error) {
	ch, err := c.sdk.V1Charges.Retrieve(ctx, id, nil)
	if err != nil {
		var se *sdk.Error
		if errors.As(err, &se) && se.HTTPStatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, toAPIError(err)
	}
	return mapCharge(ch), nil
}

// ListInvoicesForCustomer returns all invoices for the given Stripe customer ID.
// Used by the billing archive builder to snapshot billing history before hard-delete.
func ListInvoicesForCustomer(ctx context.Context, c *Client, customerID string) ([]Invoice, error) {
	params := &sdk.InvoiceListParams{
		Customer: sdk.String(customerID),
	}
	params.Limit = sdk.Int64(100) // batch up to 100; archive builder accumulates all

	var out []Invoice
	for inv, err := range c.sdk.V1Invoices.List(ctx, params) {
		if err != nil {
			return nil, toAPIError(err)
		}
		out = append(out, *mapInvoice(inv))
	}
	return out, nil
}

func mapRefund(r *sdk.Refund) *Refund {
	out := &Refund{
		ID:       r.ID,
		Amount:   r.Amount,
		Currency: string(r.Currency),
		Status:   string(r.Status),
		Reason:   string(r.Reason),
	}
	if r.Charge != nil {
		out.ChargeID = r.Charge.ID
	}
	return out
}

func mapCharge(ch *sdk.Charge) *Charge {
	out := &Charge{
		ID:       ch.ID,
		Amount:   ch.Amount,
		Currency: string(ch.Currency),
		Status:   string(ch.Status),
		Created:  ch.Created,
	}
	// Extract card fingerprint from payment_method_details.card.fingerprint (§8 pattern D).
	if ch.PaymentMethodDetails != nil && ch.PaymentMethodDetails.Card != nil {
		out.CardFingerprint = ch.PaymentMethodDetails.Card.Fingerprint
	}
	return out
}
