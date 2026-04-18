package stripe

import (
	"context"
	"encoding/json"
	"net/url"
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
	v := url.Values{}
	if in.Email != "" {
		v.Set("email", in.Email)
	}
	if in.Name != "" {
		v.Set("name", in.Name)
	}
	v.Set("metadata[store_id]", in.StoreID)
	v.Set("metadata[tenant_id]", in.TenantID)
	if in.Country != "" {
		v.Set("address[country]", in.Country)
	}

	body, err := c.PostForm(ctx, "/v1/customers", CustomerIdempotencyKey(in.StoreID), v)
	if err != nil {
		return nil, err
	}
	var cu Customer
	if err := json.Unmarshal(body, &cu); err != nil {
		return nil, err
	}
	return &cu, nil
}
