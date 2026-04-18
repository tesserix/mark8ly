package stripe

import (
	"context"

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
