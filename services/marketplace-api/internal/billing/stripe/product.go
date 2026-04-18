package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

// ErrNotFound is returned when a Stripe resource lookup yields no matching object.
var ErrNotFound = errors.New("stripe: resource not found")

// Product represents a Stripe Product object (fields used by billing-bootstrap).
type Product struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata"`
	Active   bool              `json:"active"`
}

// CreateProduct calls POST /v1/products with metadata[plan] set so subsequent
// FindProductByMetadata lookups succeed without storing the Stripe ID locally.
func CreateProduct(ctx context.Context, c *Client, name, planKey, idempotencyKey string) (*Product, error) {
	v := url.Values{}
	v.Set("name", name)
	v.Set("metadata[plan]", planKey)
	body, err := c.PostForm(ctx, "/v1/products", idempotencyKey, v)
	if err != nil {
		return nil, err
	}
	var p Product
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// FindProductByMetadata searches the first page of active products (limit=100)
// for a product whose metadata.plan matches planKey. This makes the bootstrap
// job idempotent without the service persisting Stripe product IDs locally.
func FindProductByMetadata(ctx context.Context, c *Client, planKey string) (*Product, error) {
	body, err := c.Get(ctx, "/v1/products?active=true&limit=100")
	if err != nil {
		return nil, err
	}
	var page struct {
		Data []Product `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	for i := range page.Data {
		if page.Data[i].Metadata["plan"] == planKey {
			return &page.Data[i], nil
		}
	}
	return nil, ErrNotFound
}
