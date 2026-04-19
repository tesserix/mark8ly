package stripe

import (
	"context"
	"fmt"

	sdk "github.com/stripe/stripe-go/v82"
)

// OneOffInvoiceInput describes a single one-shot Stripe invoice — used
// by the white-label app add-on purchase path (P15 §3.4) where the
// charge is a single prorated amount + setup fee, not a recurring
// subscription.
type OneOffInvoiceInput struct {
	CustomerID  string // stripe_customer_id from store_subscriptions
	Currency    string // ISO-4217, lowercase (e.g. "usd")
	AmountCents int64  // total charge in smallest currency unit
	Description string // shown on the Stripe hosted-invoice page
	// Metadata is copied onto the invoice; use it to wire the webhook
	// handler back to the subscription — e.g.
	//   {"kind":"white_label_app_add_on","tenant_id":"…","store_id":"…"}
	Metadata map[string]string
	// IdempotencyKey keeps a replayed handler call (user double-clicks)
	// from creating two invoices. Recommended format: "addon:{storeID}:{v}".
	IdempotencyKey string
}

// CreateOneOffInvoice creates a single Stripe invoice with exactly one
// line item. Returns the mapped Invoice (including hosted_invoice_url
// for the caller to redirect to).
//
// Flow:
//  1. Create an InvoiceItem with the amount and currency.
//  2. Create an Invoice with auto_advance=true — Stripe sweeps the
//     pending item into the new invoice and finalizes immediately.
//  3. Retrieve the finalized invoice to capture hosted_invoice_url.
//
// On Stripe-side error anywhere in the flow the caller sees a wrapped
// error; the pending InvoiceItem from step 1 is orphaned on failure
// (Stripe's model — it sweeps next time). The idempotency key scopes
// steps 1+2 so a retry converges on the same invoice.
func CreateOneOffInvoice(ctx context.Context, c *Client, in OneOffInvoiceInput) (*Invoice, error) {
	if in.CustomerID == "" {
		return nil, fmt.Errorf("stripe: CreateOneOffInvoice: CustomerID required")
	}
	if in.AmountCents <= 0 {
		return nil, fmt.Errorf("stripe: CreateOneOffInvoice: AmountCents must be > 0")
	}
	if in.Currency == "" {
		return nil, fmt.Errorf("stripe: CreateOneOffInvoice: Currency required")
	}

	// Step 1 — invoice item.
	itemParams := &sdk.InvoiceItemCreateParams{
		Customer:    sdk.String(in.CustomerID),
		Amount:      sdk.Int64(in.AmountCents),
		Currency:    sdk.String(in.Currency),
		Description: sdk.String(in.Description),
	}
	if in.IdempotencyKey != "" {
		itemParams.IdempotencyKey = sdk.String(in.IdempotencyKey + ":item")
	}
	for k, v := range in.Metadata {
		itemParams.AddMetadata(k, v)
	}
	if _, err := c.sdk.V1InvoiceItems.Create(ctx, itemParams); err != nil {
		return nil, toAPIError(err)
	}

	// Step 2 — invoice that sweeps the pending item.
	invParams := &sdk.InvoiceCreateParams{
		Customer:    sdk.String(in.CustomerID),
		AutoAdvance: sdk.Bool(true),
		Description: sdk.String(in.Description),
	}
	if in.IdempotencyKey != "" {
		invParams.IdempotencyKey = sdk.String(in.IdempotencyKey + ":invoice")
	}
	for k, v := range in.Metadata {
		invParams.AddMetadata(k, v)
	}

	inv, err := c.sdk.V1Invoices.Create(ctx, invParams)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapInvoice(inv), nil
}
