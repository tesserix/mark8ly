package stripe

import (
	"context"
	"errors"

	sdk "github.com/stripe/stripe-go/v82"
)

// ErrNotFound is returned when a Stripe resource lookup yields no matching object.
var ErrNotFound = errors.New("stripe: resource not found")

// Product represents a Stripe Product object.
type Product struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata"`
	Active   bool              `json:"active"`
}

// FindProductByMetadata searches the first page of active products (limit=100)
// for a product whose metadata.plan matches planKey. This is the read side of
// product lookup: the console is the authoring surface for the plan catalog
// now (#303), and this lets callers resolve a Product without the service
// persisting Stripe product IDs locally.
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
