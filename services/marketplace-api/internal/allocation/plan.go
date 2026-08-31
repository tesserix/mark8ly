package allocation

import (
	"errors"
	"fmt"
)

// ErrNoWarehouse means the store has nowhere to ship from. Allocation cannot
// answer that with a plan, and a caller must not treat it as "out of stock":
// the merchant has a configuration problem, not an inventory one.
var ErrNoWarehouse = errors.New("allocation: store has no warehouses")

// CannotFillError names the first line no combination of warehouses can
// satisfy, and by how much it falls short.
//
// It carries the variant because a bare failure is unactionable on a cart of
// six items — the storefront needs to know which line to mark, exactly as
// storefront's outOfStockError does.
type CannotFillError struct {
	VariantID string
	Short     int
}

func (e CannotFillError) Error() string {
	return fmt.Sprintf("allocation: variant %s is %d short across all warehouses", e.VariantID, e.Short)
}

// Plan decides which warehouse ships how much of each line.
//
// warehouses must already be in fill order — pass them through
// InPriorityOrder. Taking an ordered slice rather than sorting internally is
// deliberate: a later ordering rule (nearest-pincode, say) becomes a
// different function feeding this one, not a change to this one.
//
// The algorithm walks warehouses in order and takes min(remaining, available)
// for every unsatisfied line at each. That fills from the merchant's first
// warehouse down, which is the sentence you can say to a merchant. It does
// NOT minimise the number of parcels: a true minimum is a bin-packing problem,
// and one fewer parcel in a rare case is worth less than an explainable rule.
//
// Assignments come back grouped by warehouse in fill order, and by line order
// within a warehouse, so the same inputs always produce the same slice.
func Plan(warehouses []Warehouse, avail Availability, lines []Line) ([]Assignment, error) {
	if len(lines) == 0 {
		return []Assignment{}, nil
	}
	if len(warehouses) == 0 {
		return nil, ErrNoWarehouse
	}

	remaining := make([]int, len(lines))
	for i, line := range lines {
		if line.Quantity <= 0 {
			return nil, fmt.Errorf("allocation: line %d (variant %s): quantity must be positive, got %d",
				i, line.VariantID, line.Quantity)
		}
		remaining[i] = line.Quantity
	}

	assignments := make([]Assignment, 0, len(lines))

	for w, warehouse := range warehouses {
		for i, line := range lines {
			if remaining[i] == 0 {
				continue
			}

			take := 0
			switch {
			case line.SellsPastZero:
				// The merchant sells this past zero on purpose, so stock does
				// not constrain it. Assign it whole to the first warehouse —
				// leaving it unassigned because no location has units would
				// produce an order line that ships from nothing.
				if w != 0 {
					continue
				}
				take = remaining[i]
			default:
				take = min(remaining[i], avail.At(line.VariantID, warehouse.ID))
			}

			if take == 0 {
				continue
			}
			assignments = append(assignments, Assignment{
				WarehouseID: warehouse.ID,
				VariantID:   line.VariantID,
				Quantity:    take,
			})
			remaining[i] -= take
		}
	}

	for i, left := range remaining {
		if left > 0 {
			return nil, CannotFillError{VariantID: lines[i].VariantID, Short: left}
		}
	}

	return assignments, nil
}
