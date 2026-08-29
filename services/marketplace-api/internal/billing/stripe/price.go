package stripe

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/stripe/stripe-go/v82"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// zeroDecimalCurrencies lists currencies that Stripe treats as zero-decimal:
// unit_amount is the raw currency value, NOT multiplied by 100. The pricing
// catalog stores everything in "minor units × 100" for internal consistency,
// so we divide by 100 at the Stripe boundary for these currencies.
// Source: https://stripe.com/docs/currencies#zero-decimal
var zeroDecimalCurrencies = map[string]bool{
	"bif": true, "clp": true, "djf": true, "gnf": true, "jpy": true,
	"kmf": true, "krw": true, "mga": true, "pyg": true, "rwf": true,
	"ugx": true, "vnd": true, "vuv": true, "xaf": true, "xof": true, "xpf": true,
}

// stripeUnitAmount converts the catalog's UnitAmountMinor to the integer
// Stripe expects for the given currency. For zero-decimal currencies this is
// value/100; otherwise it's value as-is.
func stripeUnitAmount(currency string, catalogMinor int64) int64 {
	if zeroDecimalCurrencies[strings.ToLower(currency)] {
		return catalogMinor / 100
	}
	return catalogMinor
}

// DescribeMismatch reports how an existing Stripe Price disagrees with the
// descriptor claiming the same lookup_key, or "" when they agree.
//
// It lives HERE, beside stripeUnitAmount, deliberately. The catalog stores
// zero-decimal currencies pre-multiplied by 100 and that conversion is
// applied on the way into Stripe, so a comparison written anywhere else
// would have to re-implement it — which is the exact second-derivation
// mistake #459 exists to remove. Keeping the check next to the conversion
// means the two cannot drift.
//
// # Known limit: currency_options are not compared
//
// A developed-tier Price carries currency_options for seven currencies, and
// Stripe does not return them unless the read expands them — which
// FindPriceByLookupKey does not do and Price does not map. A change confined
// to a NON-baseline currency is therefore still undetected. That is a much
// narrower hole than the one this closes; it is stated so it is not
// mistaken for covered.
func DescribeMismatch(d pricing.PriceDescriptor, existing *Price) string {
	if existing == nil {
		return ""
	}
	want := stripeUnitAmount(d.Baseline.Currency, d.Baseline.UnitAmountMinor)
	if existing.UnitAmount != want {
		return fmt.Sprintf("catalog says unit_amount=%d, Stripe holds %d", want, existing.UnitAmount)
	}
	if !strings.EqualFold(existing.Currency, d.Baseline.Currency) {
		return fmt.Sprintf("catalog says currency=%q, Stripe holds %q", d.Baseline.Currency, existing.Currency)
	}
	// An empty TaxBehavior in the catalog means "Stripe's default", which
	// Stripe reports back as "unspecified" — not a divergence.
	if d.Baseline.TaxBehavior != "" && !strings.EqualFold(existing.TaxBehavior, d.Baseline.TaxBehavior) {
		return fmt.Sprintf("catalog says tax_behavior=%q, Stripe holds %q", d.Baseline.TaxBehavior, existing.TaxBehavior)
	}
	return ""
}

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
	// v2: the v1 idempotency key got stuck on a failed bootstrap run where
	// currency_options[baseline] was rejected; bump so Stripe doesn't replay
	// the cached error. Effective idempotency still comes from lookup_key
	// which CreatePrice's callers check first.
	params.IdempotencyKey = sdk.String("price:v3:" + desc.LookupKey)
	params.Product = sdk.String(productID)
	params.Currency = sdk.String(desc.Baseline.Currency)
	params.UnitAmount = sdk.Int64(stripeUnitAmount(desc.Baseline.Currency, desc.Baseline.UnitAmountMinor))
	params.LookupKey = sdk.String(desc.LookupKey)
	params.Recurring = &sdk.PriceCreateRecurringParams{
		Interval: sdk.String(interval),
	}
	params.AddMetadata("plan", string(desc.Plan))
	params.AddMetadata("period", string(desc.Period))
	params.AddMetadata("tier", string(desc.Tier))

	// Developed tier: emit currency_options for non-baseline currencies. PPP
	// tier: skip (single-currency Price). Stripe rejects the create call if
	// currency_options contains the top-level currency, so explicitly skip it.
	if desc.Tier == pricing.TierDeveloped {
		params.CurrencyOptions = make(map[string]*sdk.PriceCreateCurrencyOptionsParams, len(desc.Options))
		for cur, amt := range desc.Options {
			if cur == desc.Baseline.Currency {
				continue
			}
			opt := &sdk.PriceCreateCurrencyOptionsParams{
				UnitAmount: sdk.Int64(stripeUnitAmount(cur, amt.UnitAmountMinor)),
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
