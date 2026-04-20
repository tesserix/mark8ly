package stripe

import (
	"context"
	"time"

	sdk "github.com/stripe/stripe-go/v82"
)

// InvoiceSummary is the trimmed view of a Stripe invoice surfaced on the admin
// billing page. Fields mirror what the UI actually renders; everything else
// on the Stripe object is intentionally omitted. Named *Summary to avoid a
// clash with the existing Invoice type used by the webhook dispatch path.
type InvoiceSummary struct {
	ID          string    `json:"id"`
	Number      string    `json:"number"`
	Created     time.Time `json:"created"`
	AmountPaid  int64     `json:"amount_paid"` // minor units (cents/paise/etc.)
	AmountDue   int64     `json:"amount_due"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"` // paid, open, void, draft, uncollectible
	HostedURL   string    `json:"hosted_invoice_url"`
	PDFURL      string    `json:"invoice_pdf"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
}

// ListCustomerInvoices returns the most recent invoices for the given customer,
// newest first. Returns an empty slice (not nil) when the customer has no
// invoices on file.
func ListCustomerInvoices(ctx context.Context, c *Client, customerID string, limit int64) ([]InvoiceSummary, error) {
	if customerID == "" {
		return []InvoiceSummary{}, nil
	}
	if limit <= 0 {
		limit = 25
	}

	params := &sdk.InvoiceListParams{
		Customer: sdk.String(customerID),
	}
	params.Context = ctx
	params.Limit = sdk.Int64(limit)

	out := make([]InvoiceSummary, 0, limit)
	for inv, err := range c.sdk.V1Invoices.List(ctx, params) {
		if err != nil {
			return nil, toAPIError(err)
		}
		if inv == nil {
			continue
		}
		out = append(out, InvoiceSummary{
			ID:          inv.ID,
			Number:      inv.Number,
			Created:     time.Unix(inv.Created, 0).UTC(),
			AmountPaid:  inv.AmountPaid,
			AmountDue:   inv.AmountDue,
			Currency:    string(inv.Currency),
			Status:      string(inv.Status),
			HostedURL:   inv.HostedInvoiceURL,
			PDFURL:      inv.InvoicePDF,
			PeriodStart: time.Unix(inv.PeriodStart, 0).UTC(),
			PeriodEnd:   time.Unix(inv.PeriodEnd, 0).UTC(),
		})
		if int64(len(out)) >= limit {
			break
		}
	}
	return out, nil
}
