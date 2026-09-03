package stripe

import (
	"context"

	sdk "github.com/stripe/stripe-go/v82"
)

// Price represents a Stripe Price object.
type Price struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Currency    string            `json:"currency"`
	UnitAmount  int64             `json:"unit_amount"`
	LookupKey   string            `json:"lookup_key"`
	Metadata    map[string]string `json:"metadata"`
	Active      bool              `json:"active"`
	TaxBehavior string            `json:"tax_behavior"`
}

// FindPriceByLookupKey returns the active Price matching lookupKey, or ErrNotFound.
func FindPriceByLookupKey(ctx context.Context, c *Client, lookupKey string) (*Price, error) {
	listParams := &sdk.PriceListParams{
		LookupKeys: sdk.StringSlice([]string{lookupKey}),
		Active:     sdk.Bool(true),
	}

	for p, err := range c.sdk.V1Prices.List(ctx, listParams) {
		if err != nil {
			return nil, toAPIError(err)
		}
		return mapPrice(p), nil
	}
	return nil, ErrNotFound
}

func mapPrice(p *sdk.Price) *Price {
	return &Price{
		ID:          p.ID,
		Object:      p.Object,
		Currency:    string(p.Currency),
		UnitAmount:  p.UnitAmount,
		LookupKey:   p.LookupKey,
		Metadata:    p.Metadata,
		Active:      p.Active,
		TaxBehavior: string(p.TaxBehavior),
	}
}
