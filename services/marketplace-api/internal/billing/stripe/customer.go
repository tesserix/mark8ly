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
// PaymentMethodSummary captures the card-brand + last4 (or Link email)
// surfaced to the billing page. Empty Type + Brand means no PM on file.
// For Type="card" the UI renders "<Brand> ending in ···· <Last4>".
// For Type="link" Last4 holds the Link account email (for display only).
type PaymentMethodSummary struct {
	Type  string // "card" | "link" | "" (none)
	Brand string // card brand (visa/mastercard/...) or "link"
	Last4 string // card last-4 digits, or Link account email for Type=link
}

// GetCustomerDefaultPaymentMethod returns a summary of the customer's current
// billing method. Prefers invoice_settings.default_payment_method; falls back
// to the most recently attached PaymentMethod of any type so portal-added
// methods surface even when Stripe hasn't promoted them to default yet.
//
// Handles both card and Stripe Link payment methods. Missing PM is not an
// error — returns (PaymentMethodSummary{}, false, nil).
func GetCustomerDefaultPaymentMethod(ctx context.Context, c *Client, customerID string) (PaymentMethodSummary, bool, error) {
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
	if cu != nil && cu.InvoiceSettings != nil && cu.InvoiceSettings.DefaultPaymentMethod != nil {
		if sum, ok := summariseSDKPaymentMethod(cu.InvoiceSettings.DefaultPaymentMethod); ok {
			return sum, true, nil
		}
	}

	// Fallback — list attached payment methods of any type and take the
	// first. Stripe Link PMs don't get listed under type=card, so don't
	// filter by type.
	listParams := &sdk.PaymentMethodListParams{
		Customer: sdk.String(customerID),
	}
	listParams.Context = ctx
	listParams.Limit = sdk.Int64(5)
	for pm, err := range c.sdk.V1PaymentMethods.List(ctx, listParams) {
		if err != nil {
			return PaymentMethodSummary{}, false, toAPIError(err)
		}
		if pm == nil {
			continue
		}
		if sum, ok := summariseSDKPaymentMethod(pm); ok {
			return sum, true, nil
		}
	}
	return PaymentMethodSummary{}, false, nil
}

func summariseSDKPaymentMethod(pm *sdk.PaymentMethod) (PaymentMethodSummary, bool) {
	if pm == nil {
		return PaymentMethodSummary{}, false
	}
	switch string(pm.Type) {
	case "card":
		if pm.Card != nil {
			return PaymentMethodSummary{
				Type:  "card",
				Brand: string(pm.Card.Brand),
				Last4: pm.Card.Last4,
			}, true
		}
	case "link":
		email := ""
		if pm.Link != nil {
			email = pm.Link.Email
		}
		return PaymentMethodSummary{
			Type:  "link",
			Brand: "link",
			Last4: email,
		}, true
	}
	return PaymentMethodSummary{}, false
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
