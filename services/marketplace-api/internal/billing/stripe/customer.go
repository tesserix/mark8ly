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
