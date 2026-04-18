package stripe

import (
	"context"
	"encoding/json"
	"net/url"
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
	body, err := c.Get(ctx, "/v1/invoices/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	var inv Invoice
	if err := json.Unmarshal(body, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}
