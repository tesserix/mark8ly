package storefront

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// stockAt is units of a variant available at one physical storage location.
type stockAt struct {
	LocationID string
	Units      int
}

// storageLocations records, per variant and warehouse, WHERE the units
// backing that warehouse's availability actually sit — possibly in more
// than one place while the sentinel and real rows coexist.
//
// It exists because of the expand phase: until PR 6 backfills
// variant_stock.location_id, units sit at product.DefaultLocationID while
// allocation reasons about real warehouse ids. A hold must be placed against
// the location(s) the rows actually have, or it locks nothing. A single
// warehouse can be backed by MORE than one storage location even before the
// backfill completes: PR 5 adds per-location stock editing, which can write
// a real warehouse-id row while the sentinel row for that same variant still
// exists. After the backfill every warehouse has exactly one contributing
// location and this collapses to the obvious case.
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

	// Which warehouse a stock row belongs to. Sentinel rows have no real
	// warehouse of their own, so they answer for the store's FIRST warehouse
	// in fill order — the one a single-warehouse store ships everything from
	// anyway. Real rows answer for themselves.
	byWarehouse := make(map[string]string, len(warehouses))
	for _, w := range warehouses {
		byWarehouse[w.ID] = w.ID
	}

	for _, r := range rows {
		warehouseID := r.LocationID
		if r.LocationID == product.DefaultLocationID {
			warehouseID = warehouses[0].ID
		} else if _, known := byWarehouse[r.LocationID]; !known {
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
		// Append rather than overwrite: the sentinel row and a real
		// warehouse-id row can both back the same warehouse (PR 5 writes
		// real rows before PR 6 backfills the sentinel away), and a hold
		// must be able to target every location the units actually sit at.
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
