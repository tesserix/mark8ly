package mode

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		raw     string
		want    Mode
		wantErr bool
	}{
		{"", Both, false},
		{"admin", Admin, false},
		{"storefront", Storefront, false},
		{"both", Both, false},
		{"Admin", "", true}, // case-sensitive
		{"nonsense", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) err = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRunsAdminAndStorefront(t *testing.T) {
	tests := []struct {
		m              Mode
		runsAdmin      bool
		runsStorefront bool
	}{
		{Admin, true, false},
		{Storefront, false, true},
		{Both, true, true},
	}
	for _, tt := range tests {
		if got := tt.m.RunsAdmin(); got != tt.runsAdmin {
			t.Errorf("%v.RunsAdmin() = %v, want %v", tt.m, got, tt.runsAdmin)
		}
		if got := tt.m.RunsStorefront(); got != tt.runsStorefront {
			t.Errorf("%v.RunsStorefront() = %v, want %v", tt.m, got, tt.runsStorefront)
		}
	}
}
