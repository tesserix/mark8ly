package storefront

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
)

// stockAt is units of a variant available at one physical storage location.
type stockAt struct {
	LocationID string
	Units      int
}

// storageLocations records, per variant and warehouse, WHERE the units
// backing that warehouse's availability actually sit.
//
// It exists because a hold must be placed against the location(s) the rows
// actually have, or it locks nothing. It was written for the expand phase,
// when units sat on a sentinel location while allocation reasoned about
// real warehouse ids and one warehouse could be backed by two rows. After
// #177 PR 6 that collapses to the obvious case — one warehouse, one row —
// and the indirection is kept because it is what makes a hold target a
// real location rather than the warehouse's id by assumption.
type storageLocations map[string]map[string][]stockAt

// loadAvailability builds the snapshot allocation.Plan reasons over.
//
// Availability is variant_stock.quantity minus OTHER carts' live holds — the
// same arithmetic stockhold.Available uses, and excluding the calling cart's
// own holds for the same reason: a cart must not be told its own reservation
// is competition.
//
// This is a READ. It takes no locks, and it is not what makes the decision
// safe: stockhold.Hold re-checks under SELECT ... FOR UPDATE, and that is the
// check that counts. A snapshot that is stale by the time it is used simply
// produces a plan whose holds fail, which is the same outcome a
// single-warehouse shopper gets today.
func loadAvailability(
	ctx context.Context,
	tx *gorm.DB,
	cartToken string,
	warehouses []allocation.Warehouse,
	variantIDs []string,
) (allocation.Availability, storageLocations, error) {
	avail := allocation.Availability{}
	storage := storageLocations{}
	if len(warehouses) == 0 || len(variantIDs) == 0 {
		return avail, storage, nil
	}

	var rows []struct {
		VariantID  string
		LocationID string
		Available  int
	}
	err := tx.WithContext(ctx).Raw(
		`SELECT vs.variant_id,
		        vs.location_id,
		        vs.quantity - COALESCE((
		            SELECT SUM(sh.qty) FROM stock_holds sh
		             WHERE sh.variant_id = vs.variant_id
		               AND sh.location_id = vs.location_id
		               AND sh.state = 'held'
		               AND sh.expires_at > now()
		               AND sh.cart_token <> ?), 0) AS available
		   FROM variant_stock vs
		  WHERE vs.variant_id IN ?`, cartToken, variantIDs).Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("storefront: load availability: %w", err)
	}

	// Which warehouse a stock row belongs to. Every row answers for itself
	// now: the sentinel tolerance that mapped location-less rows onto the
	// store's first warehouse was removed in #177 PR 6 step 2, once
	// migration 000123 had moved production's units onto real warehouses
	// and every write path resolved one.
	byWarehouse := make(map[string]string, len(warehouses))
	for _, w := range warehouses {
		byWarehouse[w.ID] = w.ID
	}

	for _, r := range rows {
		warehouseID := r.LocationID
		if _, known := byWarehouse[r.LocationID]; !known {
			// Stock at a location that is not one of this store's warehouses
			// — another store's row cannot appear here (variantIDs are this
			// order's), so this is a warehouse deleted out from under its
			// stock. Skip it rather than allocate from somewhere that no
			// longer exists.
			continue
		}

		if r.Available < 0 {
			// Holds can exceed stock if a merchant lowered the count while
			// reservations were live. Allocation clamps too, but reporting a
			// negative here would be nonsense on the way in.
			r.Available = 0
		}

		if avail[r.VariantID] == nil {
			avail[r.VariantID] = map[string]int{}
			storage[r.VariantID] = map[string][]stockAt{}
		}
		avail[r.VariantID][warehouseID] += r.Available
		// Append rather than overwrite. One warehouse is normally backed by
		// one row, but the shape is kept: a hold must be able to target
		// every location the units actually sit at, and collapsing to a
		// single row here would silently drop the second if that ever
		// stops being true.
		storage[r.VariantID][warehouseID] = append(
			storage[r.VariantID][warehouseID],
			stockAt{LocationID: r.LocationID, Units: r.Available},
		)
	}

	// Deterministic order: two runs over the same data must produce the same
	// plan, and map iteration order is not stable.
	for _, byWarehouse := range storage {
		for warehouseID, entries := range byWarehouse {
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].LocationID < entries[j].LocationID
			})
			byWarehouse[warehouseID] = entries
		}
	}

	return avail, storage, nil
}
