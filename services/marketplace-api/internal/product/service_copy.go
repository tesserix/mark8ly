// Package product — service_copy.go: cross-store Copy flow.
//
// Copy creates a fresh product aggregate in a target store under the
// SAME tenant, carrying over options, variants, media references, and
// categories from a source product.
//
// Locked decisions (spec §6.4 / §13.5.1):
//
//   - All rows get NEW UUIDs (product, options, option_values, variants,
//     variant_option_value links, media).
//   - The copy is always draft with published_at=nil and
//     copy_source_product_id pointing at the source.
//   - Handle is generated via SuggestNextHandle against the TARGET store
//     (pre-tx, on the main connection).
//   - Variants PRESERVE the SOURCE store's currency. This is the
//     deliberate "copy keeps original pricing" behaviour — different
//     from Create, which forces target currency. Editors can re-price
//     with UpdateVariants afterwards.
//   - Variant SKUs get a "-copy-<6hex>" suffix appended to avoid
//     collisions in the target store's partial-unique index. Spec
//     doesn't mandate a scheme; this is a conservative local choice.
//   - Media rows get fresh row IDs but keep the SAME StorageKey — the
//     copy references the same storage object (no GCS I/O).
//   - Categories: for each source link, find a target-store category
//     with the same slug; if missing, create one inside the tx with
//     (Name, Slug, IsActive=true) carried from the source.
//   - Inventory starts at zero in the target store: variant_stock row
//     at the default location with quantity=0 per new variant.
//   - A single product.created outbox row is enqueued inside the tx,
//     payload carries store_id (target) + product_id (new) +
//     copy_source_product_id (source).
package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CopyRequest is the service-level DTO for cross-store product copy.
type CopyRequest struct {
	SourceProductID string
	SourceTenantID  string
	SourceStoreID   string
	TargetStoreID   string
	CopiedBy        *string
}

// Copy clones a product from source store into target store within the
// same tenant. Returns the re-loaded target aggregate on success.
func (s *Service) Copy(ctx context.Context, req CopyRequest) (*Aggregate, error) {
	// 1. Load source aggregate (NotFound passes through).
	source, err := s.repo.GetByIDForStore(ctx, req.SourceProductID, req.SourceStoreID, req.SourceTenantID)
	if err != nil {
		return nil, err
	}

	// 2. Validate target store.
	if req.TargetStoreID == req.SourceStoreID {
		return nil, apperrors.TargetStoreInvalid(req.TargetStoreID, "target is the same as source")
	}
	if _, err := s.storesRepo.GetByIDForTenant(ctx, req.TargetStoreID, req.SourceTenantID); err != nil {
		// Tenant-scoped lookup: "not found" covers both "doesn't exist"
		// and "different tenant" without leaking which one.
		return nil, apperrors.TargetStoreInvalid(req.TargetStoreID, "not found or not owned by this tenant")
	}

	// 3. Build the fresh aggregate with new UUIDs.
	newProductID := uuid.NewString()

	// 3a. Handle: pre-tx lookup against target store via main s.db.
	takenInTarget := func(candidate string) bool {
		var n int64
		_ = s.db.WithContext(ctx).
			Model(&Product{}).
			Where("store_id = ? AND handle = ? AND deleted_at IS NULL",
				req.TargetStoreID, candidate).
			Count(&n).Error
		return n > 0
	}
	newHandle := SuggestNextHandle(source.Product.Handle, takenInTarget)

	// 3b. Options + option values with fresh UUIDs, keyed for variant
	//     link remapping by old OptionValue ID.
	oldValIDToNewValID := make(map[string]string, 8)
	newOptions := make([]Option, 0, len(source.Options))
	for _, opt := range source.Options {
		newOptID := uuid.NewString()
		newVals := make([]OptionValue, 0, len(opt.Values))
		for _, v := range opt.Values {
			newValID := uuid.NewString()
			oldValIDToNewValID[v.ID] = newValID
			newVals = append(newVals, OptionValue{
				ID:       newValID,
				OptionID: newOptID,
				Value:    v.Value,
				Position: v.Position,
			})
		}
		newOptions = append(newOptions, Option{
			ID:        newOptID,
			ProductID: newProductID,
			Name:      opt.Name,
			Position:  opt.Position,
			Values:    newVals,
		})
	}

	// 3c. Variants — preserve source currency (NOT target). Append a
	//     random "-copy-xxxxxx" suffix to SKU to avoid collisions in
	//     the target store's live-unique SKU index.
	newVariants := make([]Variant, 0, len(source.Variants))
	for _, sv := range source.Variants {
		newVariantID := uuid.NewString()
		newLinks := make([]VariantOptionValue, 0, len(sv.OptionValueLinks))
		for _, link := range sv.OptionValueLinks {
			newVID, ok := oldValIDToNewValID[link.OptionValueID]
			if !ok {
				return nil, fmt.Errorf("product: copy: variant %q references unknown option value %s",
					sv.SKU, link.OptionValueID)
			}
			newLinks = append(newLinks, VariantOptionValue{OptionValueID: newVID})
		}
		newVariants = append(newVariants, Variant{
			ID:                newVariantID,
			StoreID:           req.TargetStoreID,
			SKU:               sv.SKU + "-copy-" + uuid.NewString()[:6],
			Barcode:           sv.Barcode,
			Price:             sv.Price,
			CompareAtPrice:    sv.CompareAtPrice,
			CostPrice:         sv.CostPrice,
			CurrencyCode:      sv.CurrencyCode, // deliberate: SOURCE currency
			WeightGrams:       sv.WeightGrams,
			InventoryPolicy:   sv.InventoryPolicy,
			LowStockThreshold: sv.LowStockThreshold,
			Position:          sv.Position,
			OptionValueLinks:  newLinks,
		})
	}

	// 3d. Media — fresh row IDs, same storage_key (reference, not copy).
	newMedia := make([]Media, 0, len(source.Media))
	for _, m := range source.Media {
		newMedia = append(newMedia, Media{
			ID:         uuid.NewString(),
			StorageKey: m.StorageKey,
			URL:        m.URL,
			Alt:        m.Alt,
			Position:   m.Position,
			MediaType:  m.MediaType,
			// VariantID intentionally NOT remapped for slice 1 — Copy
			// only carries product-level media. Variant-level media
			// remap is a TODO if/when that field is populated.
			Width:  m.Width,
			Height: m.Height,
			Bytes:  m.Bytes,
		})
	}

	srcProductID := source.Product.ID
	newAgg := &Aggregate{
		Product: Product{
			ID:                  newProductID,
			TenantID:            req.SourceTenantID,
			StoreID:             req.TargetStoreID,
			Handle:              newHandle,
			Title:               source.Product.Title,
			Description:         source.Product.Description,
			Status:              StatusDraft,
			VendorID:            source.Product.VendorID,
			Tags:                source.Product.Tags,
			SEOTitle:            source.Product.SEOTitle,
			SEODescription:      source.Product.SEODescription,
			CopySourceProductID: &srcProductID,
			// PublishedAt left nil: copies start unpublished.
			CreatedBy: req.CopiedBy,
			UpdatedBy: req.CopiedBy,
		},
		Options:  newOptions,
		Variants: newVariants,
		Media:    newMedia,
		// CategoryLinks are resolved inside the tx below.
	}

	// 4. Transaction.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 4a. Resolve / create target categories by slug, inside tx.
		targetCategoryIDs, err := s.resolveCopyCategories(ctx, tx,
			source.CategoryLinks, req.TargetStoreID, req.SourceTenantID)
		if err != nil {
			return err
		}

		// 4b. Insert the aggregate.
		if err := s.repo.CreateAggregateInTx(ctx, tx, newAgg); err != nil {
			return err
		}

		// 4c. Replace category links with resolved target IDs.
		if err := s.repo.ReplaceCategoryLinksInTx(ctx, tx, newAgg.Product.ID, targetCategoryIDs); err != nil {
			return err
		}

		// 4d. Seed variant_stock rows at quantity 0 in target store.
		for i := range newAgg.Variants {
			if err := s.repo.UpdateVariantStockInTx(ctx, tx,
				newAgg.Variants[i].ID, DefaultLocationID, 0); err != nil {
				return err
			}
		}

		// 4e. Outbox: product.created with copy_source_product_id.
		payload, err := json.Marshal(map[string]any{
			"store_id":               req.TargetStoreID,
			"product_id":             newAgg.Product.ID,
			"copy_source_product_id": srcProductID,
		})
		if err != nil {
			return fmt.Errorf("product: marshal copy payload: %w", err)
		}
		evt := &outbox.OutboxEvent{
			TenantID:    req.SourceTenantID,
			Aggregate:   outbox.AggregateProduct,
			AggregateID: newAgg.Product.ID,
			EventType:   outbox.EventProductCreated,
			Payload:     datatypes.JSON(payload),
		}
		return s.outboxRepo.EnqueueInTx(ctx, tx, evt)
	})
	if err != nil {
		return nil, err
	}

	// 5. Re-load from the target store.
	return s.repo.GetByIDForStore(ctx, newAgg.Product.ID, req.TargetStoreID, req.SourceTenantID)
}

// resolveCopyCategories maps source category links onto the target
// store: existing same-slug category reused, missing slugs created
// inside the tx. Source categories are loaded by ID (tenant-scoped,
// across stores is fine because categories are per-store but under the
// same tenant).
func (s *Service) resolveCopyCategories(
	ctx context.Context,
	tx *gorm.DB,
	srcLinks []ProductCategory,
	targetStoreID, tenantID string,
) ([]string, error) {
	if len(srcLinks) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(srcLinks))
	for _, link := range srcLinks {
		// Load source category — tenant-scoped, any store.
		var src category.Category
		err := tx.WithContext(ctx).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", link.CategoryID, tenantID).
			First(&src).Error
		if err != nil {
			return nil, fmt.Errorf("product: copy: load source category %s: %w", link.CategoryID, err)
		}

		// Look for same slug in target store.
		var existing category.Category
		err = tx.WithContext(ctx).
			Where("store_id = ? AND slug = ? AND deleted_at IS NULL", targetStoreID, src.Slug).
			First(&existing).Error
		if err == nil {
			out = append(out, existing.ID)
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("product: copy: lookup target category %q: %w", src.Slug, err)
		}

		// Create a new category row in the target store inside the tx.
		fresh := category.Category{
			ID:       uuid.NewString(),
			TenantID: tenantID,
			StoreID:  targetStoreID,
			Name:     src.Name,
			Slug:     src.Slug,
			IsActive: true,
		}
		if err := tx.WithContext(ctx).Create(&fresh).Error; err != nil {
			return nil, fmt.Errorf("product: copy: create target category %q: %w", src.Slug, err)
		}
		out = append(out, fresh.ID)
	}
	return out, nil
}
