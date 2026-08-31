package allocation_test

import (
	"errors"
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
	warehouses := []allocation.Warehouse{wh("a", 1)}
	avail := allocation.Availability{"v1": {"a": 5}}

	_, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 0}})
	require.Error(t, err, "a line for zero units is a caller bug, not an empty allocation")
}

func TestPlan_NoLinesIsNoAssignmentsNotAnError(t *testing.T) {
	got, err := allocation.Plan([]allocation.Warehouse{wh("a", 1)}, allocation.Availability{}, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
