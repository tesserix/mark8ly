// Package product — service_variant_patch.go: UpdateVariantBasics
// applies a narrow PATCH to one variant row. Inventory writes go
// through variant_stock so the sync trigger owns the denormalised
// inventory_quantity column.
package product

import (
	"context"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// UpdateVariantBasicsRequest is the narrow quick-PATCH DTO for one
// variant. CurrencyCode is intentionally not applied — a non-nil value
// that differs from the store currency is rejected with
// CurrencyChangeForbidden.
type UpdateVariantBasicsRequest struct {
	ProductID         string
	VariantID         string
	StoreID           string
	TenantID          string
	SKU               *string
	Barcode           *string
	Price             *decimal.Decimal
	CompareAtPrice    *decimal.Decimal
	CostPrice         *decimal.Decimal
	WeightGrams       *int
	LengthCM          *decimal.Decimal
	WidthCM           *decimal.Decimal
	HeightCM          *decimal.Decimal
	InventoryQuantity *int
	InventoryPolicy   *string
	LowStockThreshold *int
	Position          *int
	CurrencyCode      *string
}

// UpdateVariantBasics applies the non-nil fields to one variant row.
// Inventory quantity writes go to variant_stock (not product_variants
// directly) so the trigger maintains the denormalised column.
func (s *Service) UpdateVariantBasics(ctx context.Context, req UpdateVariantBasicsRequest) (*Variant, error) {
	// Currency change guard.
	if req.CurrencyCode != nil {
		store, err := s.storesRepo.GetByIDForTenant(ctx, req.StoreID, req.TenantID)
		if err != nil {
			return nil, apperrors.NotFound("store")
		}
		if *req.CurrencyCode != store.CurrencyCode {
			return nil, apperrors.CurrencyChangeForbidden()
		}
	}

	fields := map[string]any{}
	if req.SKU != nil {
		fields["sku"] = *req.SKU
	}
	if req.Barcode != nil {
		fields["barcode"] = *req.Barcode
	}
	if req.Price != nil {
		fields["price"] = *req.Price
	}
	if req.CompareAtPrice != nil {
		fields["compare_at_price"] = *req.CompareAtPrice
	}
	if req.CostPrice != nil {
		fields["cost_price"] = *req.CostPrice
	}
	if req.WeightGrams != nil {
		fields["weight_grams"] = *req.WeightGrams
	}
	if req.LengthCM != nil {
		fields["length_cm"] = *req.LengthCM
	}
	if req.WidthCM != nil {
		fields["width_cm"] = *req.WidthCM
	}
	if req.HeightCM != nil {
		fields["height_cm"] = *req.HeightCM
	}
	if req.InventoryPolicy != nil {
		fields["inventory_policy"] = *req.InventoryPolicy
	}
	if req.LowStockThreshold != nil {
		fields["low_stock_threshold"] = *req.LowStockThreshold
	}
	if req.Position != nil {
		fields["position"] = *req.Position
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			if err := s.repo.UpdateVariantBasicsInTx(ctx, tx, req.ProductID, req.VariantID, req.StoreID, req.TenantID, fields); err != nil {
				return err
			}
		} else if req.InventoryQuantity == nil {
			// Nothing to do — still need to verify existence. Do a
			// zero-field update to surface NotFound consistently.
			if err := s.repo.UpdateVariantBasicsInTx(ctx, tx, req.ProductID, req.VariantID, req.StoreID, req.TenantID, map[string]any{"updated_at": gorm.Expr("now()")}); err != nil {
				return err
			}
		}
		if req.InventoryQuantity != nil {
			if err := s.repo.UpdateVariantStockInTx(ctx, tx, req.VariantID, DefaultLocationID, *req.InventoryQuantity); err != nil {
				return err
			}
		}
		return s.enqueue(ctx, tx, req.TenantID, req.StoreID, req.ProductID, outbox.EventProductUpdated)
	})
	if err != nil {
		return nil, err
	}

	// Re-load product aggregate and pick the variant.
	agg, err := s.repo.GetByIDForStore(ctx, req.ProductID, req.StoreID, req.TenantID)
	if err != nil {
		return nil, err
	}
	for i := range agg.Variants {
		if agg.Variants[i].ID == req.VariantID {
			return &agg.Variants[i], nil
		}
	}
	return nil, apperrors.NotFound("variant")
}
