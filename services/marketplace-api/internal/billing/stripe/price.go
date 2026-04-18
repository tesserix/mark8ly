package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

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
	v := url.Values{}
	v.Set("product", productID)
	v.Set("currency", desc.Baseline.Currency)
	v.Set("unit_amount", strconv.FormatInt(desc.Baseline.UnitAmountMinor, 10))
	v.Set("lookup_key", desc.LookupKey)
	v.Set("metadata[plan]", string(desc.Plan))
	v.Set("metadata[period]", string(desc.Period))
	v.Set("metadata[tier]", string(desc.Tier))

	interval := "month"
	if desc.Period == pricing.PeriodAnnual {
		interval = "year"
	}
	v.Set("recurring[interval]", interval)

	// Developed tier: emit currency_options. PPP tier: skip (single-currency Price).
	if desc.Tier == pricing.TierDeveloped {
		for cur, amt := range desc.Options {
			v.Set(fmt.Sprintf("currency_options[%s][unit_amount]", cur), strconv.FormatInt(amt.UnitAmountMinor, 10))
			if amt.TaxBehavior != "" {
				v.Set(fmt.Sprintf("currency_options[%s][tax_behavior]", cur), amt.TaxBehavior)
			}
		}
	}

	body, err := c.PostForm(ctx, "/v1/prices", "price:"+desc.LookupKey, v)
	if err != nil {
		return nil, err
	}
	var p Price
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// FindPriceByLookupKey returns the active Price matching lookupKey, or ErrNotFound.
func FindPriceByLookupKey(ctx context.Context, c *Client, lookupKey string) (*Price, error) {
	body, err := c.Get(ctx, "/v1/prices?lookup_keys[]="+url.QueryEscape(lookupKey)+"&active=true")
	if err != nil {
		return nil, err
	}
	var page struct {
		Data []Price `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	if len(page.Data) == 0 {
		return nil, ErrNotFound
	}
	return &page.Data[0], nil
}
