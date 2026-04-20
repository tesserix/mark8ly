package stripe

import (
	"context"
	"errors"
	"net/http"

	sdk "github.com/stripe/stripe-go/v82"
)

type Customer struct {
	ID       string            `json:"id"`
	Email    string            `json:"email"`
	Metadata map[string]string `json:"metadata"`
}

type CreateCustomerInput struct {
	StoreID  string
	TenantID string
	Email    string
	Name     string
	Country  string // ISO 3166 alpha-2
}

func CreateCustomer(ctx context.Context, c *Client, in CreateCustomerInput) (*Customer, error) {
	params := &sdk.CustomerCreateParams{}
	params.Context = ctx
	params.IdempotencyKey = sdk.String(CustomerIdempotencyKey(in.StoreID))
	if in.Email != "" {
		params.Email = sdk.String(in.Email)
	}
	if in.Name != "" {
		params.Name = sdk.String(in.Name)
	}
	if in.Country != "" {
		params.Address = &sdk.AddressParams{Country: sdk.String(in.Country)}
	}
	params.AddMetadata("store_id", in.StoreID)
	params.AddMetadata("tenant_id", in.TenantID)

	cu, err := c.sdk.V1Customers.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return &Customer{
		ID:       cu.ID,
		Email:    cu.Email,
		Metadata: cu.Metadata,
	}, nil
}

// DeleteCustomer permanently deletes a Stripe customer. This is irreversible
// and is only called as part of the hard-delete pipeline (§15.2 — 150-day step).
//
// PaymentMethodSummary captures just the card-brand + last4 surfaced to the
// billing page. Returned from GetCustomerDefaultCard. Both fields empty means
// the customer has no payment method on file yet.
type PaymentMethodSummary struct {
	Brand string
	Last4 string
}

// GetCustomerDefaultCard returns the brand + last4 of the customer's current
// billing card. Prefers invoice_settings.default_payment_method; falls back to
// the most recently attached card PaymentMethod so portal-added methods surface
// even when Stripe hasn't promoted them to default yet.
//
// Missing PM is not an error — returns (PaymentMethodSummary{}, false, nil).
func GetCustomerDefaultCard(ctx context.Context, c *Client, customerID string) (PaymentMethodSummary, bool, error) {
	if customerID == "" {
		return PaymentMethodSummary{}, false, nil
	}

	cuParams := &sdk.CustomerRetrieveParams{}
	cuParams.Context = ctx
	cuParams.AddExpand("invoice_settings.default_payment_method")
	cu, err := c.sdk.V1Customers.Retrieve(ctx, customerID, cuParams)
	if err != nil {
		return PaymentMethodSummary{}, false, toAPIError(err)
	}
	if cu != nil && cu.InvoiceSettings != nil &&
		cu.InvoiceSettings.DefaultPaymentMethod != nil &&
		cu.InvoiceSettings.DefaultPaymentMethod.Card != nil {
		card := cu.InvoiceSettings.DefaultPaymentMethod.Card
		return PaymentMethodSummary{Brand: string(card.Brand), Last4: card.Last4}, true, nil
	}

	// Fallback — list attached card payment methods and take the first.
	listParams := &sdk.PaymentMethodListParams{
		Customer: sdk.String(customerID),
		Type:     sdk.String("card"),
	}
	listParams.Context = ctx
	listParams.Limit = sdk.Int64(1)
	for pm, err := range c.sdk.V1PaymentMethods.List(ctx, listParams) {
		if err != nil {
			return PaymentMethodSummary{}, false, toAPIError(err)
		}
		if pm != nil && pm.Card != nil {
			return PaymentMethodSummary{Brand: string(pm.Card.Brand), Last4: pm.Card.Last4}, true, nil
		}
	}
	return PaymentMethodSummary{}, false, nil
}

// Idempotent: if the customer is already deleted in Stripe (HTTP 404), the
// error is treated as a no-op success so the caller can safely retry.
func DeleteCustomer(ctx context.Context, c *Client, customerID string) error {
	_, err := c.sdk.V1Customers.Delete(ctx, customerID, nil)
	if err != nil {
		var se *sdk.Error
		if errors.As(err, &se) && se.HTTPStatusCode == http.StatusNotFound {
			// Already deleted — idempotent success.
			return nil
		}
		return toAPIError(err)
	}
	return nil
}
