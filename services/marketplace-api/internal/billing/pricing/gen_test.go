package pricing

import "testing"

// TestAnnualMonthlyEquivalent exercises annualMonthlyEquivalent's rounding
// directly. The full-file staleness comparison in genpricing_test.go never
// exercises math.Round's tie-breaking, because no annual value in the
// committed catalog today has remainder 6 mod 12 (see gen.go's doc comment
// on annualMonthlyEquivalent). Once the console owns the catalog, new
// prices arrive freely and this rounding rule becomes load-bearing, so it
// is covered here independent of any specific catalog contents.
func TestAnnualMonthlyEquivalent(t *testing.T) {
	tests := []struct {
		name       string
		annual     int64
		wantMonths int64
	}{
		{
			name:       "exact division",
			annual:     1200, // 1200 / 12 = 100.0
			wantMonths: 100,
		},
		{
			name:       "rounds down below the half point",
			annual:     1205, // 1205 / 12 = 100.41666...
			wantMonths: 100,
		},
		{
			name:       "rounds up above the half point",
			annual:     1208, // 1208 / 12 = 100.66666...
			wantMonths: 101,
		},
		{
			name: "exact .5 tie rounds away from zero (annual % 12 == 6)",
			// 1206 / 12 = 100.5 exactly. math.Round rounds half away from
			// zero, so a positive .5 tie rounds UP. This is the case the
			// full-file comparison against the committed catalog can never
			// exercise, because none of today's annual amounts have
			// remainder 6 mod 12.
			annual:     1206,
			wantMonths: 101,
		},
		{
			name:       "real catalog value: Starter USD",
			annual:     18200,
			wantMonths: 1517,
		},
		{
			name:       "real catalog value: Pro USD",
			annual:     118800,
			wantMonths: 9900,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := annualMonthlyEquivalent(tt.annual)
			if got != tt.wantMonths {
				t.Errorf("annualMonthlyEquivalent(%d) = %d, want %d", tt.annual, got, tt.wantMonths)
			}
		})
	}
}
