package arbitrage

import (
	"testing"
)

func TestIsDevelopedMarket(t *testing.T) {
	developed := []string{"US", "GB", "DE", "FR", "AU", "NZ", "CA", "IE", "NL", "IT", "ES", "JP", "SG"}
	for _, c := range developed {
		if !IsDevelopedMarket(c) {
			t.Errorf("IsDevelopedMarket(%q) = false, want true", c)
		}
	}

	emerging := []string{"IN", "ID", "VN", "PH", "TH", "MY", "BR", "MX"}
	for _, c := range emerging {
		if IsDevelopedMarket(c) {
			t.Errorf("IsDevelopedMarket(%q) = true, want false", c)
		}
	}

	ambiguous := []string{"??", ""}
	for _, c := range ambiguous {
		if IsDevelopedMarket(c) {
			t.Errorf("IsDevelopedMarket(%q) = true, want false (ambiguous)", c)
		}
	}
}

func TestNormalizeCountry(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"us", "US"},
		{" US ", "US"},
		{"", "??"},
		{"USA", "??"},
		{"1", "??"},
		{"??", "??"},
		{"GB", "GB"},
	}
	for _, tc := range cases {
		got := NormalizeCountry(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeCountry(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsKnownCountry(t *testing.T) {
	if !IsKnownCountry("IN") {
		t.Error("IsKnownCountry('IN') = false, want true")
	}
	if !IsKnownCountry("us") {
		t.Error("IsKnownCountry('us') = false, want true (normalized)")
	}
	if IsKnownCountry("??") {
		t.Error("IsKnownCountry('??') = true, want false")
	}
	if IsKnownCountry("") {
		t.Error("IsKnownCountry('') = true, want false")
	}
}
