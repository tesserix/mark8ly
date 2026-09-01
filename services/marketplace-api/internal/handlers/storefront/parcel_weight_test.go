package storefront

import "testing"

func intp(v int) *int { return &v }

// A zero weight makes ShipEngine return NO rates at all
// ("Packages must weigh more than zero kg"), so an order that quoted
// fine at checkout fails on submit. This is the exact production
// failure on the-bondi-store: the storefront applied a 500g fallback
// when fetching the quote, the server applied none when placing the
// order.
func TestParcelWeightGrams(t *testing.T) {
	tests := []struct {
		name          string
		variantWeight *int
		configured    int
		want          int
	}{
		{"real variant weight wins", intp(220), 500, 220},
		{"nil variant weight uses the store's configured default", nil, 300, 300},
		{"zero variant weight uses the configured default", intp(0), 300, 300},
		{"negative variant weight is not trusted", intp(-5), 300, 300},
		{"no variant weight and no config falls back to 500", nil, 0, FallbackParcelWeightGrams},
		{"zero variant weight and no config falls back to 500", intp(0), 0, FallbackParcelWeightGrams},
		{"a negative configured value is not trusted either", nil, -10, FallbackParcelWeightGrams},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parcelWeightGrams(tc.variantWeight, tc.configured)
			if got != tc.want {
				t.Errorf("parcelWeightGrams(%v, %d) = %d, want %d",
					tc.variantWeight, tc.configured, got, tc.want)
			}
		})
	}
}

// The invariant that actually matters: whatever the inputs, the weight
// sent to a carrier is never zero or negative.
func TestParcelWeightGramsIsAlwaysPositive(t *testing.T) {
	for _, vw := range []*int{nil, intp(-1), intp(0), intp(1), intp(5000)} {
		for _, cfg := range []int{-1, 0, 1, 500} {
			if got := parcelWeightGrams(vw, cfg); got <= 0 {
				t.Errorf("parcelWeightGrams(%v, %d) = %d, want > 0", vw, cfg, got)
			}
		}
	}
}
