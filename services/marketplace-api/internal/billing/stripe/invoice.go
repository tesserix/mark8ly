package stripe

import (
	"context"

	sdk "github.com/stripe/stripe-go/v82"
)

type Invoice struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	HostedInvoiceURL string `json:"hosted_invoice_url"`
	PaymentIntent    string `json:"payment_intent"`
	Currency         string `json:"currency"`
	AmountDue        int64  `json:"amount_due"`
	AmountPaid       int64  `json:"amount_paid"`
	Customer         string `json:"customer"`
	Subscription     string `json:"subscription"`
}

func GetInvoice(ctx context.Context, c *Client, id string) (*Invoice, error) {
	inv, err := c.sdk.V1Invoices.Retrieve(ctx, id, nil)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapInvoice(inv), nil
}

// mapInvoice maps the SDK Invoice type to our public Invoice struct.
// PaymentIntent: the SDK v82 moved payment_intent off the top-level Invoice;
// we leave it empty to preserve the struct field without breaking callers.
// Subscription: extracted from Parent.SubscriptionDetails if present.
func mapInvoice(inv *sdk.Invoice) *Invoice {
	var paymentIntentID string
	// payment_intent is no longer a top-level field in SDK v82 Invoice;
	// callers that need it should expand the confirmation_secret or use
	// the Payments list. We preserve the field on our struct as empty.
	_ = paymentIntentID

	var subscriptionID string
	if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil && inv.Parent.SubscriptionDetails.Subscription != nil {
		subscriptionID = inv.Parent.SubscriptionDetails.Subscription.ID
	}

	var customerID string
	if inv.Customer != nil {
		customerID = inv.Customer.ID
	}

	return &Invoice{
		ID:               inv.ID,
		Status:           string(inv.Status),
		HostedInvoiceURL: inv.HostedInvoiceURL,
		PaymentIntent:    paymentIntentID,
		Currency:         string(inv.Currency),
		AmountDue:        inv.AmountDue,
		AmountPaid:       inv.AmountPaid,
		Customer:         customerID,
		Subscription:     subscriptionID,
	}
}
