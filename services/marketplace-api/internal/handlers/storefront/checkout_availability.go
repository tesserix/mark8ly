package storefront

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// storageLocations records, per variant and warehouse, the location_id the
// units are actually stored under.
//
// It exists because of the expand phase: until PR 6 backfills
// variant_stock.location_id, units sit at product.DefaultLocationID while
// allocation reasons about real warehouse ids. A hold must be placed against
// the location the row actually has, or it locks nothing. After the backfill
// the two values coincide and this map becomes an identity.
type storageLocations map[string]map[string]string

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
			storage[r.VariantID] = map[string]string{}
		}
		avail[r.VariantID][warehouseID] += r.Available
		storage[r.VariantID][warehouseID] = r.LocationID
	}

	return avail, storage, nil
}
