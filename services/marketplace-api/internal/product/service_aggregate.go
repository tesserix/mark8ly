// Package product — service_aggregate.go: UpdateAggregate fan-out for
// the admin PATCH /products/:id route. This is the single entry point
// the wire handler uses when a request body mutates any combination of
// scalar fields, options, variants, removed variants, or category
// links in one go.
//
// Design notes:
//   - Reuses existing vocabulary: OptionSpec, VariantInput, OptionValueRef,
//     decimal.Decimal prices. There is no parallel type hierarchy.
//   - Currency is immutable (spec §14.2). UpdateAggregateRequest has no
//     currency field; per-variant CurrencyCode is forced to the store
//     currency, same as Create.
//   - Removed variants (both the explicit RemovedVariantIDs list and
//     orphan variants that fall out of the diff) are soft-deleted via
//     the existing ApplyVariantDiffInTx to preserve order-history FKs.
//   - Option value removal is rejected with OptionValueInUse if a
//     surviving variant still references the value — the client's
//     generateVariants() is expected to have dropped orphans first.
//   - ONE product.updated outbox event is enqueued at the end of the
//     transaction regardless of which subsets changed.
package product

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// UpdateAggregateRequest is the service-level shape of a full aggregate
// PATCH. Pointer semantics distinguish "unset" from "zero":
//   - Nil sub-slices are NOT touched.
//   - Non-nil (possibly empty) sub-slices are authoritative.
//
// Note: no CurrencyCode field — currency is immutable in slice 1.
type UpdateAggregateRequest struct {
	ID       string
	StoreID  string
	TenantID string

	// Scalar fields — nil means "leave alone".
	Title             *string
	Handle            *string
	Description       *string
	Status            *string
	Tags              *[]string
	SEOTitle          *string
	SEODescription    *string
	PrimaryCategoryID *string
	TaxCode           *string
	TaxRateOverride   *decimal.Decimal
	TaxCategory       *string
	UpdatedBy         *string

	// Aggregate sub-sections — nil means "leave alone".
	Options           *[]OptionSpec
	Variants          *[]VariantInput
	RemovedVariantIDs *[]string
	CategoryIDs       *[]string
}

// UpdateAggregate applies a full-aggregate PATCH to a product. Runs in
// a single gorm.Transaction and enqueues exactly one product.updated
// outbox event on success.
func (s *Service) UpdateAggregate(ctx context.Context, req UpdateAggregateRequest) (*Aggregate, error) {
	// Resolve store (+ currency).
	store, err := s.storesRepo.GetByIDForTenant(ctx, req.StoreID, req.TenantID)
	if err != nil {
		return nil, apperrors.NotFound("store")
	}

	// Load existing aggregate (outside tx is fine — NotFound surfaces cleanly).
	existing, err := s.repo.GetByIDForStore(ctx, req.ID, req.StoreID, req.TenantID)
	if err != nil {
		return nil, err
	}

	// If variants are being replaced, validate the incoming matrix
	// against the incoming options (or existing options if Options is nil)
	// before any DB writes.
	var desiredOptions []OptionSpec
	if req.Options != nil {
		desiredOptions = *req.Options
	} else {
		desiredOptions = optionSpecsFromExisting(existing.Options)
	}
	// ValidateMatrix runs ONLY when the caller supplies variants. That is
	// what makes the OptionValueInUse guard in applyOptionsDiff live rather
	// than dead code (#400): an options-only edit — req.Options set,
	// req.Variants nil, which is the ordinary shape when an admin edits the
	// Options section without touching the variant list — reaches
	// applyOptionsDiff with no prior validation, so the guard is the sole
	// thing standing between the request and deleting an option value a
	// stored variant still references.
	//
	// When variants ARE supplied, ValidateMatrix rejects that same scenario
	// first, on a different error, and the guard never sees it. Both halves
	// of that asymmetry are pinned by tests in service_aggregate_test.go.
	if req.Variants != nil {
		// Force currency on every incoming variant.
		vars := *req.Variants
		for i, v := range vars {
			if v.CurrencyCode == "" {
				vars[i].CurrencyCode = store.CurrencyCode
				continue
			}
			if v.CurrencyCode != store.CurrencyCode {
				return nil, apperrors.CurrencyMismatch(v.CurrencyCode, store.CurrencyCode)
			}
		}
		req.Variants = &vars

		specs := variantSpecsFromInputs(vars)
		if err := ValidateMatrix(desiredOptions, specs); err != nil {
			return nil, err
		}
	}

	// Scalar field map.
	fields := map[string]any{}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			return nil, apperrors.ValidationFailed("title", "title must not be empty")
		}
		fields["title"] = t
	}
	if req.Handle != nil {
		h := strings.TrimSpace(*req.Handle)
		if h == "" {
			return nil, apperrors.ValidationFailed("handle", "handle must not be empty")
		}
		fields["handle"] = h
	}
	if req.Description != nil {
		fields["description"] = Sanitize(*req.Description)
	}
	if req.Status != nil {
		if err := validateStatus(*req.Status); err != nil {
			return nil, err
		}
		fields["status"] = *req.Status
		if *req.Status == StatusActive {
			fields["published_at"] = gorm.Expr("coalesce(published_at, now())")
		}
	}
	if req.Tags != nil {
		fields["tags"] = *req.Tags
	}
	if req.SEOTitle != nil {
		fields["seo_title"] = *req.SEOTitle
	}
	if req.SEODescription != nil {
		fields["seo_description"] = *req.SEODescription
	}
	if req.PrimaryCategoryID != nil {
		fields["primary_category_id"] = *req.PrimaryCategoryID
	}
	if req.TaxCode != nil {
		fields["tax_code"] = *req.TaxCode
	}
	if req.TaxRateOverride != nil {
		fields["tax_rate_override"] = *req.TaxRateOverride
	}
	if req.TaxCategory != nil {
		fields["tax_category"] = *req.TaxCategory
	}
	if req.UpdatedBy != nil {
		fields["updated_by"] = *req.UpdatedBy
	}
	if len(fields) > 0 {
		fields["updated_at"] = gorm.Expr("now()")
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Scalar basics.
		if len(fields) > 0 {
			if err := s.repo.UpdateBasicsInTx(ctx, tx, req.ID, req.StoreID, req.TenantID, fields); err != nil {
				return err
			}
		}

		// 2. Options diff.
		var valueByTuple map[string]string
		if req.Options != nil {
			vbt, err := s.applyOptionsDiff(ctx, tx, existing, desiredOptions, req.Variants)
			if err != nil {
				return err
			}
			valueByTuple = vbt
		} else {
			valueByTuple = buildValueByTupleFromExisting(existing.Options)
		}

		// 3. Variants diff (delegates to ApplyVariantDiffInTx which soft-deletes removed).
		var newVariants []Variant
		if req.Variants != nil || req.RemovedVariantIDs != nil {
			added, err := s.applyVariantsDiff(ctx, tx, existing, req, valueByTuple)
			if err != nil {
				return err
			}
			newVariants = added
		}

		// 4. Seed stock for newly added variants, at the store's warehouse.
		var stockLocationID string
		if len(newVariants) > 0 {
			var locErr error
			stockLocationID, locErr = s.stockLocationFor(ctx, tx, req.TenantID, req.StoreID)
			if locErr != nil {
				return locErr
			}
		}
		for i := range newVariants {
			// Find the matching input by generated ID to read InitialStock.
			if req.Variants == nil {
				break
			}
			for _, in := range *req.Variants {
				if in.ID == "" && newVariants[i].SKU == in.SKU {
					if err := s.repo.UpdateVariantStockInTx(ctx, tx,
						newVariants[i].ID, stockLocationID, in.InitialStock); err != nil {
						return err
					}
					break
				}
			}
		}

		// 4b. Sync stock for updated variants. Variant.InventoryQuantity
		// is trigger-maintained from variant_stock, so step 3's scalar
		// UPDATE on product_variants doesn't move it. For every input
		// that matches an existing row (by ID or SKU), push the intended
		// InitialStock through UpdateVariantStockInTx so the trigger
		// brings inventory_quantity into line with the form. Newly added
		// variants are handled in step 4 above; skip them here.
		if req.Variants != nil {
			existingByID := make(map[string]Variant, len(existing.Variants))
			existingBySKU := make(map[string]Variant, len(existing.Variants))
			for _, v := range existing.Variants {
				// GORM now filters soft-deleted variants out of existing.Variants
				// itself (#395); this check is belt-and-braces, not the primary
				// guard — do not delete it as dead code.
				if v.DeletedAt.Valid {
					continue
				}
				existingByID[v.ID] = v
				existingBySKU[v.SKU] = v
			}
			newByID := make(map[string]struct{}, len(newVariants))
			for _, nv := range newVariants {
				newByID[nv.ID] = struct{}{}
			}
			// Resolved once for the loop below. Step 4 may have resolved it
			// already; EnsureDefaultForStore is idempotent, so asking twice
			// costs a lookup rather than a second warehouse.
			if stockLocationID == "" {
				var locErr error
				stockLocationID, locErr = s.stockLocationFor(ctx, tx, req.TenantID, req.StoreID)
				if locErr != nil {
					return locErr
				}
			}
			for _, in := range *req.Variants {
				var matched *Variant
				if in.ID != "" {
					if v, ok := existingByID[in.ID]; ok {
						matched = &v
					}
				}
				if matched == nil {
					if v, ok := existingBySKU[in.SKU]; ok {
						matched = &v
					}
				}
				if matched == nil {
					continue // new variant — already seeded in step 4
				}
				if _, isNew := newByID[matched.ID]; isNew {
					continue
				}
				if matched.InventoryQuantity == in.InitialStock {
					continue
				}
				if err := s.repo.UpdateVariantStockInTx(ctx, tx,
					matched.ID, stockLocationID, in.InitialStock); err != nil {
					return err
				}
			}
		}

		// 5. Category links replace.
		if req.CategoryIDs != nil {
			if err := s.repo.ReplaceCategoryLinksInTx(ctx, tx, req.ID, *req.CategoryIDs); err != nil {
				return err
			}
		}

		// 6. Single product.updated outbox event.
		return s.enqueue(ctx, tx, req.TenantID, req.StoreID, req.ID, outbox.EventProductUpdated)
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetByIDForStore(ctx, req.ID, req.StoreID, req.TenantID)
}

// applyOptionsDiff reconciles product_options and product_option_values
// against the desired spec. Returns the canonical value-by-tuple map
// (keyed "<option-name>=<value>" -> option_value.id) so the variants
// diff can look up FK targets.
//
// Matching:
//   - Options match by ID if provided, else by Name.
//   - Option values match by ID if provided, else by (option, value).
//
// Rename: if a matched option's Name changed, we UPDATE product_options.name.
// The join rows reference product_option_values.id (not name), so no
// join-row touch-up is needed.
//
// Value removal: before deleting an option value, check that no surviving
// variant in the desired state still references it. Orphan references
// are rejected with OptionValueInUse.
func (s *Service) applyOptionsDiff(
	ctx context.Context,
	tx *gorm.DB,
	existing *Aggregate,
	desired []OptionSpec,
	desiredVariants *[]VariantInput,
) (map[string]string, error) {
	tx = tx.WithContext(ctx)

	// Index existing options by ID and name.
	existingOptByID := make(map[string]Option, len(existing.Options))
	existingOptByName := make(map[string]Option, len(existing.Options))
	for _, o := range existing.Options {
		existingOptByID[o.ID] = o
		existingOptByName[o.Name] = o
	}

	// Build the set of (option, value) tuples referenced by surviving variants
	// so we can reject unsafe value removals. "Surviving" = present in the
	// desired variants list (if provided) or existing variants unchanged.
	referencedValues := make(map[string]struct{})
	if desiredVariants != nil {
		for _, v := range *desiredVariants {
			for _, ref := range v.OptionValues {
				referencedValues[ref.OptionName+"="+ref.Value] = struct{}{}
			}
		}
	} else {
		// Variants unchanged — use existing variant refs.
		valueIDToTuple := map[string]string{}
		for _, o := range existing.Options {
			for _, val := range o.Values {
				valueIDToTuple[val.ID] = o.Name + "=" + val.Value
			}
		}
		for _, v := range existing.Variants {
			for _, link := range v.OptionValueLinks {
				if t, ok := valueIDToTuple[link.OptionValueID]; ok {
					referencedValues[t] = struct{}{}
				}
			}
		}
	}

	// valueByTuple is the final map we return.
	valueByTuple := make(map[string]string)
	// Track which existing option/value IDs are still desired.
	keepOptionIDs := make(map[string]struct{})
	keepValueIDs := make(map[string]struct{})

	for i, spec := range desired {
		// Resolve to an existing option row: by ID, else by name.
		var existingOpt *Option
		if spec.Name == "" {
			return nil, apperrors.ValidationFailed("options", "option name must not be empty")
		}
		// Check by name first — IDs on OptionSpec aren't plumbed in yet,
		// but matching by name gives rename detection for free. If a
		// future OptionSpec adds an ID field, check it first.
		if eo, ok := existingOptByName[spec.Name]; ok {
			existingOpt = &eo
		}

		var optID string
		if existingOpt != nil {
			optID = existingOpt.ID
			keepOptionIDs[optID] = struct{}{}
			// Position update (and future name rename if we add ID-based matching).
			res := tx.Model(&Option{}).
				Where("id = ? AND product_id = ?", optID, existing.Product.ID).
				Updates(map[string]any{"name": spec.Name, "position": i})
			if res.Error != nil {
				return nil, fmt.Errorf("product: update option %q: %w", spec.Name, res.Error)
			}
		} else {
			optID = uuid.NewString()
			row := Option{
				ID:        optID,
				ProductID: existing.Product.ID,
				Name:      spec.Name,
				Position:  i,
			}
			if err := tx.Create(&row).Error; err != nil {
				return nil, fmt.Errorf("product: create option %q: %w", spec.Name, err)
			}
			keepOptionIDs[optID] = struct{}{}
		}

		// Index existing values for this option by (ID, value string).
		existingValByID := map[string]OptionValue{}
		existingValByStr := map[string]OptionValue{}
		if existingOpt != nil {
			for _, v := range existingOpt.Values {
				existingValByID[v.ID] = v
				existingValByStr[v.Value] = v
			}
		}

		for j, val := range spec.Values {
			var existingVal *OptionValue
			if val.ID != "" {
				if ev, ok := existingValByID[val.ID]; ok {
					existingVal = &ev
				}
			}
			if existingVal == nil {
				if ev, ok := existingValByStr[val.Value]; ok {
					existingVal = &ev
				}
			}

			var valID string
			if existingVal != nil {
				valID = existingVal.ID
				keepValueIDs[valID] = struct{}{}
				res := tx.Model(&OptionValue{}).
					Where("id = ? AND option_id = ?", valID, optID).
					Updates(map[string]any{"value": val.Value, "position": j})
				if res.Error != nil {
					return nil, fmt.Errorf("product: update option value %q: %w", val.Value, res.Error)
				}
			} else {
				valID = uuid.NewString()
				row := OptionValue{
					ID:       valID,
					OptionID: optID,
					Value:    val.Value,
					Position: j,
				}
				if err := tx.Create(&row).Error; err != nil {
					return nil, fmt.Errorf("product: create option value %q: %w", val.Value, err)
				}
				keepValueIDs[valID] = struct{}{}
			}
			valueByTuple[spec.Name+"="+val.Value] = valID
		}
	}

	// Delete option values no longer in the desired set — but only if no
	// surviving variant references them.
	for _, o := range existing.Options {
		for _, v := range o.Values {
			if _, ok := keepValueIDs[v.ID]; ok {
				continue
			}
			tuple := o.Name + "=" + v.Value
			if _, referenced := referencedValues[tuple]; referenced {
				return nil, apperrors.OptionValueInUse(o.Name, v.Value)
			}
			// Delete orphan join rows first, then the value row.
			if err := tx.Where("option_value_id = ?", v.ID).
				Delete(&VariantOptionValue{}).Error; err != nil {
				return nil, fmt.Errorf("product: delete orphan join rows: %w", err)
			}
			if err := tx.Delete(&OptionValue{}, "id = ?", v.ID).Error; err != nil {
				return nil, fmt.Errorf("product: delete option value: %w", err)
			}
		}
	}

	// Delete options no longer in the desired set.
	for _, o := range existing.Options {
		if _, ok := keepOptionIDs[o.ID]; ok {
			continue
		}
		if err := tx.Delete(&Option{}, "id = ?", o.ID).Error; err != nil {
			return nil, fmt.Errorf("product: delete option: %w", err)
		}
	}

	return valueByTuple, nil
}

// applyVariantsDiff computes the Add/Update/Remove variant diff and
// dispatches it through ApplyVariantDiffInTx. Returns the freshly
// created variant rows so the caller can seed stock.
func (s *Service) applyVariantsDiff(
	ctx context.Context,
	tx *gorm.DB,
	existing *Aggregate,
	req UpdateAggregateRequest,
	valueByTuple map[string]string,
) ([]Variant, error) {
	// Map existing live variants by their tuple key (option-name -> value).
	// Build tuple from VariantOptionValue links via a lookup on existing options.
	valueIDToRef := make(map[string]OptionValueRef)
	for _, o := range existing.Options {
		for _, v := range o.Values {
			valueIDToRef[v.ID] = OptionValueRef{OptionName: o.Name, Value: v.Value}
		}
	}
	existingByTuple := make(map[string]Variant)
	existingByID := make(map[string]Variant)
	for _, v := range existing.Variants {
		// GORM now filters soft-deleted variants out of existing.Variants
		// itself (#395); this check is belt-and-braces, not the primary
		// guard — do not delete it as dead code.
		if v.DeletedAt.Valid {
			continue
		}
		existingByID[v.ID] = v
		refs := make([]OptionValueRef, 0, len(v.OptionValueLinks))
		for _, link := range v.OptionValueLinks {
			if r, ok := valueIDToRef[link.OptionValueID]; ok {
				refs = append(refs, r)
			}
		}
		key := tupleKeyFromRefs(refs)
		existingByTuple[key] = v
	}

	var diff VariantDiff

	if req.Variants != nil {
		seen := make(map[string]struct{})
		for _, in := range *req.Variants {
			refs := in.OptionValues
			key := tupleKeyFromRefs(refs)

			// Match by explicit ID first, else by tuple.
			var matched *Variant
			if in.ID != "" {
				if ev, ok := existingByID[in.ID]; ok {
					matched = &ev
				}
			}
			if matched == nil {
				if ev, ok := existingByTuple[key]; ok {
					matched = &ev
				}
			}

			if matched != nil {
				seen[matched.ID] = struct{}{}
				// Build VariantDiff.Updates row.
				upd := Variant{
					ID:                matched.ID,
					ProductID:         existing.Product.ID,
					StoreID:           existing.Product.StoreID,
					SKU:               in.SKU,
					Barcode:           in.Barcode,
					Price:             in.Price,
					CompareAtPrice:    in.CompareAtPrice,
					CostPrice:         in.CostPrice,
					WeightGrams:       in.WeightGrams,
					InventoryPolicy:   defaultInventoryPolicy(in.InventoryPolicy),
					LowStockThreshold: in.LowStockThreshold,
					Position:          in.Position,
				}
				diff.Updates = append(diff.Updates, upd)
			} else {
				// New variant — build full row with OptionValueLinks.
				vid := in.ID
				if vid == "" {
					vid = uuid.NewString()
				}
				links := make([]VariantOptionValue, 0, len(refs))
				for _, ref := range refs {
					valID, ok := valueByTuple[ref.OptionName+"="+ref.Value]
					if !ok {
						return nil, apperrors.ValidationFailed("variants",
							fmt.Sprintf("variant references unknown option value %s=%s", ref.OptionName, ref.Value))
					}
					links = append(links, VariantOptionValue{OptionValueID: valID})
				}
				add := Variant{
					ID:                vid,
					ProductID:         existing.Product.ID,
					StoreID:           existing.Product.StoreID,
					SKU:               in.SKU,
					Barcode:           in.Barcode,
					Price:             in.Price,
					CompareAtPrice:    in.CompareAtPrice,
					CostPrice:         in.CostPrice,
					CurrencyCode:      in.CurrencyCode,
					WeightGrams:       in.WeightGrams,
					InventoryPolicy:   defaultInventoryPolicy(in.InventoryPolicy),
					LowStockThreshold: in.LowStockThreshold,
					Position:          in.Position,
					OptionValueLinks:  links,
				}
				diff.Adds = append(diff.Adds, add)
			}
		}
		// Any existing variant not seen becomes a Remove (soft-delete).
		for id := range existingByID {
			if _, ok := seen[id]; !ok {
				diff.Removes = append(diff.Removes, id)
			}
		}
	}

	// Explicit RemovedVariantIDs — merge into the removes set (dedup).
	if req.RemovedVariantIDs != nil {
		existingRemoves := make(map[string]struct{}, len(diff.Removes))
		for _, id := range diff.Removes {
			existingRemoves[id] = struct{}{}
		}
		for _, id := range *req.RemovedVariantIDs {
			if _, ok := existingRemoves[id]; ok {
				continue
			}
			if _, ok := existingByID[id]; !ok {
				continue // already gone / unknown — ignore
			}
			diff.Removes = append(diff.Removes, id)
			existingRemoves[id] = struct{}{}
		}
	}

	if err := s.repo.ApplyVariantDiffInTx(ctx, tx, existing.Product.ID, existing.Product.StoreID, diff); err != nil {
		return nil, err
	}
	return diff.Adds, nil
}

// ---------- helpers ----------

func tupleKeyFromRefs(refs []OptionValueRef) string {
	parts := make([]string, len(refs))
	for i, r := range refs {
		parts[i] = r.OptionName + "=" + r.Value
	}
	// Sort inline without pulling sort just for this — len(refs) <= 3.
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j-1] > parts[j]; j-- {
			parts[j-1], parts[j] = parts[j], parts[j-1]
		}
	}
	return strings.Join(parts, "|")
}

func optionSpecsFromExisting(opts []Option) []OptionSpec {
	out := make([]OptionSpec, 0, len(opts))
	for _, o := range opts {
		vals := make([]OptionValueSpec, 0, len(o.Values))
		for _, v := range o.Values {
			vals = append(vals, OptionValueSpec{ID: v.ID, Value: v.Value})
		}
		out = append(out, OptionSpec{Name: o.Name, Values: vals})
	}
	return out
}

func buildValueByTupleFromExisting(opts []Option) map[string]string {
	out := make(map[string]string)
	for _, o := range opts {
		for _, v := range o.Values {
			out[o.Name+"="+v.Value] = v.ID
		}
	}
	return out
}

func defaultInventoryPolicy(p string) string {
	if p == "" {
		return InventoryPolicyDeny
	}
	return p
}
