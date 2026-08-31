package allocation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/allocation"
)

func at(day int) time.Time {
	return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
}

// The ordering must be TOTAL: any two warehouses must compare unequal on some
// key, or two requests could allocate the same order differently and a
// merchant would see the same cart ship from different cities.
func TestInPriorityOrder(t *testing.T) {
	tests := []struct {
		name  string
		input []allocation.Warehouse
		want  []string
	}{
		{
			name: "lower priority number ships first",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 2, CreatedAt: at(1)},
				{ID: "a", Priority: 1, CreatedAt: at(2)},
			},
			want: []string{"a", "b"},
		},
		{
			name: "equal priority breaks to the default warehouse",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 1, IsDefault: false, CreatedAt: at(1)},
				{ID: "a", Priority: 1, IsDefault: true, CreatedAt: at(2)},
			},
			want: []string{"a", "b"},
		},
		{
			name: "equal priority and default breaks to the older row",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 1, IsDefault: true, CreatedAt: at(2)},
				{ID: "a", Priority: 1, IsDefault: true, CreatedAt: at(1)},
			},
			want: []string{"a", "b"},
		},
		{
			name: "identical on every key breaks to the id, so the order is total",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 1, IsDefault: true, CreatedAt: at(1)},
				{ID: "a", Priority: 1, IsDefault: true, CreatedAt: at(1)},
			},
			want: []string{"a", "b"},
		},
		{
			name:  "empty stays empty",
			input: nil,
			want:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := allocation.InPriorityOrder(tc.input)
			ids := make([]string, 0, len(got))
			for _, w := range got {
				ids = append(ids, w.ID)
			}
			require.Equal(t, tc.want, ids)
		})
	}
}

// Repo style forbids mutating an input. A caller that ordered a slice for the
// allocator and then reused its own slice for display would otherwise see it
// silently reordered underneath them.
func TestInPriorityOrder_DoesNotMutateItsInput(t *testing.T) {
	input := []allocation.Warehouse{
		{ID: "b", Priority: 2, CreatedAt: at(1)},
		{ID: "a", Priority: 1, CreatedAt: at(2)},
	}

	_ = allocation.InPriorityOrder(input)

	require.Equal(t, "b", input[0].ID, "the caller's slice must be untouched")
	require.Equal(t, "a", input[1].ID)
}
