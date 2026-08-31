package allocation_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/allocation"
)

func wh(id string, priority int) allocation.Warehouse {
	return allocation.Warehouse{ID: id, Priority: priority, CreatedAt: at(1)}
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name       string
		warehouses []allocation.Warehouse
		avail      allocation.Availability
		lines      []allocation.Line
		want       []allocation.Assignment
	}{
		{
			name:       "one warehouse fills the line",
			warehouses: []allocation.Warehouse{wh("a", 1)},
			avail:      allocation.Availability{"v1": {"a": 5}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 3}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 3},
			},
		},
		{
			name:       "the higher-priority warehouse is filled from first",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"a": 5, "b": 5}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 3}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 3},
			},
		},
		{
			name:       "a line no single warehouse can fill is split",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"a": 3, "b": 4}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 5}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 3},
				{WarehouseID: "b", VariantID: "v1", Quantity: 2},
			},
		},
		{
			name:       "a warehouse holding none of a variant is skipped, not zero-assigned",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"b": 4}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 4}},
			want: []allocation.Assignment{
				{WarehouseID: "b", VariantID: "v1", Quantity: 4},
			},
		},
		{
			name:       "several lines each allocate independently",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail: allocation.Availability{
				"v1": {"a": 1, "b": 5},
				"v2": {"a": 9},
			},
			lines: []allocation.Line{
				{VariantID: "v1", Quantity: 3},
				{VariantID: "v2", Quantity: 2},
			},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 1},
				{WarehouseID: "a", VariantID: "v2", Quantity: 2},
				{WarehouseID: "b", VariantID: "v1", Quantity: 2},
			},
		},
		{
			name:       "a sells-past-zero line goes whole to the first warehouse regardless of stock",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"b": 100}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 7, SellsPastZero: true}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 7},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := allocation.Plan(tc.warehouses, tc.avail, tc.lines)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// The allocator's central invariant: what it assigns for a line must add up to
// exactly what the line asked for. Under-assigning ships a short parcel;
// over-assigning takes stock the customer did not buy.
func TestPlan_AssignmentsSumToTheLineQuantity(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2), wh("c", 3)}
	avail := allocation.Availability{"v1": {"a": 2, "b": 2, "c": 2}}

	got, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 5}})
	require.NoError(t, err)

	// A total-only assertion is satisfiable by an arithmetically broken plan
	// (e.g. one that double-spends stock and still nets out to 5), so assert
	// the actual per-warehouse split too.
	require.Equal(t, []allocation.Assignment{
		{WarehouseID: "a", VariantID: "v1", Quantity: 2},
		{WarehouseID: "b", VariantID: "v1", Quantity: 2},
		{WarehouseID: "c", VariantID: "v1", Quantity: 1},
	}, got)

	total := 0
	for _, a := range got {
		total += a.Quantity
	}
	require.Equal(t, 5, total)
}

func TestPlan_ReturnsCannotFillWhenNoCombinationSatisfiesTheLine(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2)}
	avail := allocation.Availability{"v1": {"a": 2, "b": 1}}

	_, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 6}})

	var cannot allocation.CannotFillError
	require.True(t, errors.As(err, &cannot), "got %v", err)
	require.Equal(t, "v1", cannot.VariantID)
	require.Equal(t, 3, cannot.Short, "6 wanted, 3 available, so 3 short")
}

func TestPlan_RefusesAStoreWithNoWarehouses(t *testing.T) {
	_, err := allocation.Plan(nil, allocation.Availability{}, []allocation.Line{{VariantID: "v1", Quantity: 1}})
	require.ErrorIs(t, err, allocation.ErrNoWarehouse)
}

func TestPlan_RefusesANonPositiveLineQuantity(t *testing.T) {
	tests := []struct {
		name     string
		quantity int
	}{
		{name: "zero", quantity: 0},
		{name: "negative", quantity: -3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warehouses := []allocation.Warehouse{wh("a", 1)}
			avail := allocation.Availability{"v1": {"a": 5}}

			_, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: tc.quantity}})
			require.Error(t, err, "a line for non-positive units is a caller bug, not an empty allocation")
			require.ErrorIs(t, err, allocation.ErrInvalidInput)
		})
	}
}

// A caller passing the same warehouse twice would otherwise produce two
// assignments naming the same warehouse for one line, which violates
// order_allocations' UNIQUE (order_item_id, warehouse_id) constraint in PR 3.
func TestPlan_RefusesDuplicateWarehouseIDs(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("a", 2)}
	avail := allocation.Availability{"v1": {"a": 10}}

	_, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 3}})
	require.ErrorIs(t, err, allocation.ErrInvalidInput)
}

// Two lines carrying the same variant must be filled from the combined
// availability without double-spending it: each line reads avail.At fresh,
// so without a shared consumption budget both could see the same units as
// available and together take more than the warehouse actually holds.
func TestPlan_DuplicateVariantLinesShareAvailability(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2)}
	avail := allocation.Availability{"v1": {"a": 4, "b": 2}}
	lines := []allocation.Line{
		{VariantID: "v1", Quantity: 3},
		{VariantID: "v1", Quantity: 3},
	}

	got, err := allocation.Plan(warehouses, avail, lines)
	require.NoError(t, err)

	byWarehouse := map[string]int{}
	total := 0
	for _, a := range got {
		require.Equal(t, "v1", a.VariantID)
		byWarehouse[a.WarehouseID] += a.Quantity
		total += a.Quantity
	}
	require.Equal(t, 6, total)
	require.LessOrEqual(t, byWarehouse["a"], 4, "must not exceed warehouse a's availability")
	require.LessOrEqual(t, byWarehouse["b"], 2, "must not exceed warehouse b's availability")
}

// When the combined quantity of duplicate-variant lines exceeds all
// availability, Plan must report CannotFillError rather than over-assign.
func TestPlan_DuplicateVariantLinesExceedingAvailabilityCannotFill(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1)}
	avail := allocation.Availability{"v1": {"a": 4}}
	lines := []allocation.Line{
		{VariantID: "v1", Quantity: 3},
		{VariantID: "v1", Quantity: 3},
	}

	_, err := allocation.Plan(warehouses, avail, lines)

	var cannot allocation.CannotFillError
	require.True(t, errors.As(err, &cannot), "got %v", err)
	require.Equal(t, "v1", cannot.VariantID)
	require.Equal(t, 2, cannot.Short, "6 wanted, 4 available, so 2 short")
}

// Negative availability (an unfloored quantity-minus-holds snapshot) must
// never produce a negative-quantity assignment, and must never make the
// remaining quantity APPEAR to grow.
func TestPlan_NegativeAvailabilityProducesNoNegativeAssignment(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2)}
	avail := allocation.Availability{"v1": {"a": -3, "b": 20}}
	lines := []allocation.Line{{VariantID: "v1", Quantity: 5}}

	got, err := allocation.Plan(warehouses, avail, lines)
	require.NoError(t, err)

	total := 0
	for _, a := range got {
		require.Positive(t, a.Quantity, "no assignment may carry a non-positive quantity")
		require.NotEqual(t, "a", a.WarehouseID, "warehouse a has negative availability and must not be assigned")
		total += a.Quantity
	}
	require.Equal(t, 5, total)
	require.Equal(t, []allocation.Assignment{
		{WarehouseID: "b", VariantID: "v1", Quantity: 5},
	}, got)
}

// Plan must not mutate its inputs — the repo-wide immutability rule.
func TestPlan_DoesNotMutateItsInputs(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2)}
	avail := allocation.Availability{"v1": {"a": 2, "b": 5}}
	lines := []allocation.Line{{VariantID: "v1", Quantity: 4}}

	wantWarehouses := slices.Clone(warehouses)
	wantAvail := allocation.Availability{"v1": {"a": 2, "b": 5}}
	wantLines := slices.Clone(lines)

	_, err := allocation.Plan(warehouses, avail, lines)
	require.NoError(t, err)

	require.Equal(t, wantWarehouses, warehouses)
	require.Equal(t, wantAvail, avail)
	require.Equal(t, wantLines, lines)
}

func TestPlan_NoLinesIsNoAssignmentsNotAnError(t *testing.T) {
	got, err := allocation.Plan([]allocation.Warehouse{wh("a", 1)}, allocation.Availability{}, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
