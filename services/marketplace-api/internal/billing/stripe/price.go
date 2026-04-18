package stripe

import (
	"context"

	sdk "github.com/stripe/stripe-go/v82"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// Price represents a Stripe Price object (fields used by billing-bootstrap).
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

// CreatePrice issues a Stripe Price with currency_options for every currency
// in desc.Options when desc.Tier is Developed. PPP tier prices ship as a
// single-currency object.
//
// Idempotency: callers should FindPriceByLookupKey first and only call
// CreatePrice on ErrNotFound (see cmd/billing-bootstrap). The Idempotency-Key
// header here protects against same-process retries only.
func CreatePrice(ctx context.Context, c *Client, productID string, desc pricing.PriceDescriptor) (*Price, error) {
	interval := "month"
	if desc.Period == pricing.PeriodAnnual {
		interval = "year"
	}

	params := &sdk.PriceCreateParams{}
	params.Context = ctx
	params.IdempotencyKey = sdk.String("price:" + desc.LookupKey)
	params.Product = sdk.String(productID)
	params.Currency = sdk.String(desc.Baseline.Currency)
	params.UnitAmount = sdk.Int64(desc.Baseline.UnitAmountMinor)
	params.LookupKey = sdk.String(desc.LookupKey)
	params.Recurring = &sdk.PriceCreateRecurringParams{
		Interval: sdk.String(interval),
	}
	params.AddMetadata("plan", string(desc.Plan))
	params.AddMetadata("period", string(desc.Period))
	params.AddMetadata("tier", string(desc.Tier))

	// Developed tier: emit currency_options. PPP tier: skip (single-currency Price).
	if desc.Tier == pricing.TierDeveloped {
		params.CurrencyOptions = make(map[string]*sdk.PriceCreateCurrencyOptionsParams, len(desc.Options))
		for cur, amt := range desc.Options {
			opt := &sdk.PriceCreateCurrencyOptionsParams{
				UnitAmount: sdk.Int64(amt.UnitAmountMinor),
			}
			if amt.TaxBehavior != "" {
				opt.TaxBehavior = sdk.String(amt.TaxBehavior)
			}
			params.CurrencyOptions[cur] = opt
		}
	}

	p, err := c.sdk.V1Prices.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapPrice(p), nil
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
