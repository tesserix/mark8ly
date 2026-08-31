package allocation

import (
	"errors"
	"fmt"
)

// ErrNoWarehouse means the store has nowhere to ship from. Allocation cannot
// answer that with a plan, and a caller must not treat it as "out of stock":
// the merchant has a configuration problem, not an inventory one.
var ErrNoWarehouse = errors.New("allocation: store has no warehouses")

// ErrInvalidInput means the caller passed Plan something it should never have
// produced — a non-positive line quantity, or the same warehouse ID twice.
// It is a caller bug, not a domain failure like CannotFillError, and is kept
// distinct so a caller mapping allocator errors to HTTP responses does not
// have to string-match to tell the two apart.
var ErrInvalidInput = errors.New("allocation: invalid input")

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

	seenWarehouse := make(map[string]struct{}, len(warehouses))
	for _, warehouse := range warehouses {
		if _, ok := seenWarehouse[warehouse.ID]; ok {
			return nil, fmt.Errorf("%w: warehouse %s appears more than once", ErrInvalidInput, warehouse.ID)
		}
		seenWarehouse[warehouse.ID] = struct{}{}
	}

	remaining := make([]int, len(lines))
	for i, line := range lines {
		if line.Quantity <= 0 {
			return nil, fmt.Errorf("%w: line %d (variant %s): quantity must be positive, got %d",
				ErrInvalidInput, i, line.VariantID, line.Quantity)
		}
		remaining[i] = line.Quantity
	}

	assignments := make([]Assignment, 0, len(lines))

	// used tracks, per variant and warehouse, how much of that warehouse's
	// availability has already been committed to an earlier line. Without
	// it, two lines carrying the same variant would each read avail.At
	// fresh and could double-spend the same units of stock.
	used := make(map[string]map[string]int)

	for _, warehouse := range warehouses {
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
				take = remaining[i]
			default:
				already := used[line.VariantID][warehouse.ID]
				// avail.At can be negative — PR 3's availability snapshot is
				// quantity minus live holds, not floored at zero. Clamp it
				// here rather than let a negative take reduce remaining[i]
				// (which would INCREASE what looks left to fill).
				free := avail.At(line.VariantID, warehouse.ID) - already
				if free < 0 {
					free = 0
				}
				take = min(remaining[i], free)
			}

			if take <= 0 {
				continue
			}
			assignments = append(assignments, Assignment{
				WarehouseID: warehouse.ID,
				VariantID:   line.VariantID,
				Quantity:    take,
			})
			remaining[i] -= take
			if !line.SellsPastZero {
				if used[line.VariantID] == nil {
					used[line.VariantID] = make(map[string]int)
				}
				used[line.VariantID][warehouse.ID] += take
			}
		}
	}

	for i, left := range remaining {
		if left > 0 {
			return nil, CannotFillError{VariantID: lines[i].VariantID, Short: left}
		}
	}

	return assignments, nil
}
