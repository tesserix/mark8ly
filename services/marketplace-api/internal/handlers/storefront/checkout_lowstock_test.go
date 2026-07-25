package storefront

import "testing"

// TestCrossedLowStock table-drives the crossedLowStock predicate: given the
// post-sale stock (current), the quantity just sold (qty), and the
// variant's low-stock threshold, does this particular sale look like the
// one that pushed inventory at/under the threshold?
func TestCrossedLowStock(t *testing.T) {
	tests := []struct {
		name      string
		current   int
		qty       int
		threshold int
		want      bool
	}{
		{
			name:      "crossed: pre-sale above threshold, post-sale at/under",
			current:   3,
			qty:       5,
			threshold: 5,
			want:      true,
		},
		{
			name:      "already below threshold before this sale - no re-fire",
			current:   1,
			qty:       1,
			threshold: 5,
			want:      false,
		},
		{
			name:      "above threshold, no crossing",
			current:   20,
			qty:       2,
			threshold: 5,
			want:      false,
		},
		{
			name:      "exact boundary: post-sale equals threshold counts as crossed",
			current:   5,
			qty:       1,
			threshold: 5,
			want:      true,
		},
		{
			name:      "qty=0 defensive case: current at threshold but nothing sold",
			current:   5,
			qty:       0,
			threshold: 5,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crossedLowStock(tt.current, tt.qty, tt.threshold)
			if got != tt.want {
				t.Errorf("crossedLowStock(current=%d, qty=%d, threshold=%d) = %v, want %v",
					tt.current, tt.qty, tt.threshold, got, tt.want)
			}
		})
	}
}
