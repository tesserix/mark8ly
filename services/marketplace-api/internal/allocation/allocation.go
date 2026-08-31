// Package allocation decides which warehouse ships how much of each line of
// an order (#177).
//
// It is deliberately pure: no database, no context, no clock. The inputs are
// a store's warehouses in the order they should be filled from, a snapshot of
// what is available where, and the lines to place. Everything that makes the
// decision hard to reason about — locking, transactions, the fact that the
// snapshot can be stale by the time it is used — lives in the caller.
//
// The plan this package returns is ADVISORY. stockhold.Hold's per-location
// availability check under SELECT ... FOR UPDATE remains the thing that
// actually stops two orders taking the last unit; a plan that loses that race
// simply fails, exactly as a single-warehouse order does today.
package allocation

import (
	"cmp"
	"slices"
	"time"
)

// Warehouse is one candidate origin, reduced to what the decision needs.
type Warehouse struct {
	ID        string
	Priority  int
	IsDefault bool
	CreatedAt time.Time
}

// Line is one order line to place.
type Line struct {
	VariantID string
	Quantity  int
	// SellsPastZero mirrors product_variants.inventory_policy == "continue":
	// the merchant sells this variant past zero on purpose, so availability
	// does not constrain where it ships from.
	SellsPastZero bool
}

// Availability is units on hand per variant, per warehouse: avail[variantID][warehouseID].
// A missing entry means zero — callers do not have to enumerate empty pairs.
type Availability map[string]map[string]int

// At returns the units of variantID available at warehouseID.
func (a Availability) At(variantID, warehouseID string) int {
	return a[variantID][warehouseID]
}

// Assignment is one unit of the answer: ship Quantity of VariantID from
// WarehouseID. A line split across two warehouses produces two Assignments.
type Assignment struct {
	WarehouseID string
	VariantID   string
	Quantity    int
}

// InPriorityOrder returns a NEW slice ordered the way allocation fills:
// merchant-set priority first, then the store's default warehouse, then the
// older row, then the id.
//
// The final id tiebreak exists to make the ordering TOTAL. Without it two
// warehouses created in the same second with equal priority could compare
// equal, and two identical carts could then allocate differently — a merchant
// would see the same order ship from different cities with nothing to explain
// it.
//
// The input slice is not modified.
func InPriorityOrder(warehouses []Warehouse) []Warehouse {
	ordered := slices.Clone(warehouses)
	if ordered == nil {
		ordered = []Warehouse{}
	}
	slices.SortStableFunc(ordered, func(a, b Warehouse) int {
		if c := cmp.Compare(a.Priority, b.Priority); c != 0 {
			return c
		}
		// IsDefault true sorts first, so invert the boolean comparison.
		if a.IsDefault != b.IsDefault {
			if a.IsDefault {
				return -1
			}
			return 1
		}
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return ordered
}
