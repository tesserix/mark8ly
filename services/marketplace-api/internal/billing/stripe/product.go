package stripe

import (
	"context"
	"errors"

	sdk "github.com/stripe/stripe-go/v82"
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
	params := &sdk.ProductCreateParams{}
	params.Context = ctx
	params.IdempotencyKey = sdk.String(idempotencyKey)
	params.Name = sdk.String(name)
	params.AddMetadata("plan", planKey)

	p, err := c.sdk.V1Products.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapProduct(p), nil
}

// FindProductByMetadata searches the first page of active products (limit=100)
// for a product whose metadata.plan matches planKey. This makes the bootstrap
// job idempotent without the service persisting Stripe product IDs locally.
func FindProductByMetadata(ctx context.Context, c *Client, planKey string) (*Product, error) {
	listParams := &sdk.ProductListParams{
		Active: sdk.Bool(true),
	}
	listParams.Limit = sdk.Int64(100)

	for p, err := range c.sdk.V1Products.List(ctx, listParams) {
		if err != nil {
			return nil, toAPIError(err)
		}
		if p.Metadata["plan"] == planKey {
			return mapProduct(p), nil
		}
	}
	return nil, ErrNotFound
}

func mapProduct(p *sdk.Product) *Product {
	return &Product{
		ID:       p.ID,
		Name:     p.Name,
		Metadata: p.Metadata,
		Active:   p.Active,
	}
}
