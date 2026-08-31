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

// A 2-element case cannot pin the direction of the IsDefault tiebreak: with
// only two elements, flipping "IsDefault true sorts first" to "sorts last"
// still passes because the comparator is only ever evaluated in one
// direction. With four elements and mixed input order, the wrong direction
// changes the output.
func TestInPriorityOrder_IsDefaultTiebreakIsPinned(t *testing.T) {
	in := []allocation.Warehouse{
		{ID: "y", Priority: 1, IsDefault: true, CreatedAt: at(2)},
		{ID: "x", Priority: 1, IsDefault: false, CreatedAt: at(1)},
		{ID: "z", Priority: 1, IsDefault: false, CreatedAt: at(3)},
		{ID: "w", Priority: 1, IsDefault: true, CreatedAt: at(4)},
	}

	got := allocation.InPriorityOrder(in)
	ids := make([]string, 0, len(got))
	for _, w := range got {
		ids = append(ids, w.ID)
	}
	require.Equal(t, []string{"y", "w", "x", "z"}, ids)
}

// Ordering must be TOTAL: whatever order the same set of warehouses arrives
// in, InPriorityOrder must produce the identical result every time.
func TestInPriorityOrder_IsPermutationInvariant(t *testing.T) {
	base := []allocation.Warehouse{
		{ID: "y", Priority: 1, IsDefault: true, CreatedAt: at(2)},
		{ID: "x", Priority: 1, IsDefault: false, CreatedAt: at(1)},
		{ID: "z", Priority: 1, IsDefault: false, CreatedAt: at(3)},
		{ID: "w", Priority: 1, IsDefault: true, CreatedAt: at(4)},
	}

	permutations := [][]allocation.Warehouse{
		{base[0], base[1], base[2], base[3]},
		{base[3], base[2], base[1], base[0]},
		{base[1], base[3], base[0], base[2]},
		{base[2], base[0], base[3], base[1]},
	}

	var want []string
	for i, perm := range permutations {
		got := allocation.InPriorityOrder(perm)
		ids := make([]string, 0, len(got))
		for _, w := range got {
			ids = append(ids, w.ID)
		}
		if i == 0 {
			want = ids
			continue
		}
		require.Equal(t, want, ids, "input order must not change the result")
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
