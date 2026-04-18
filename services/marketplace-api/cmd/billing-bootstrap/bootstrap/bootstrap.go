// Package bootstrap upserts Stripe Products and Prices from the pricing catalog.
// Safe to re-run: existing Products are reused via metadata.plan lookup; existing
// Prices are reused via lookup_key.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	stripec "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// Run performs the idempotent bootstrap. Returns on first error.
func Run(ctx context.Context, c *stripec.Client, descriptors []pricing.PriceDescriptor, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	// Discover unique plans in the descriptor set.
	planKeys := make(map[pricing.Plan]struct{})
	for _, d := range descriptors {
		planKeys[d.Plan] = struct{}{}
	}

	// Upsert one Product per plan.
	productIDs := make(map[pricing.Plan]string)
	for plan := range planKeys {
		p, err := stripec.FindProductByMetadata(ctx, c, string(plan))
		if errors.Is(err, stripec.ErrNotFound) {
			logger.Info("bootstrap: creating product", "plan", plan)
			p, err = stripec.CreateProduct(ctx, c, "Mark8ly "+string(plan), string(plan), "product:"+string(plan))
		} else if err == nil {
			logger.Info("bootstrap: reusing product", "plan", plan, "product_id", p.ID)
		}
		if err != nil {
			return fmt.Errorf("bootstrap: upsert product %s: %w", plan, err)
		}
		productIDs[plan] = p.ID
	}

	// Upsert one Price per descriptor (keyed by lookup_key).
	for _, d := range descriptors {
		prodID := productIDs[d.Plan]
		existing, err := stripec.FindPriceByLookupKey(ctx, c, d.LookupKey)
		if errors.Is(err, stripec.ErrNotFound) {
			logger.Info("bootstrap: creating price", "lookup_key", d.LookupKey, "plan", d.Plan, "period", d.Period, "tier", d.Tier)
			_, err = stripec.CreatePrice(ctx, c, prodID, d)
		} else if err == nil {
			logger.Info("bootstrap: reusing price", "lookup_key", d.LookupKey, "price_id", existing.ID)
		}
		if err != nil {
			return fmt.Errorf("bootstrap: upsert price %s: %w", d.LookupKey, err)
		}
	}
	return nil
}
