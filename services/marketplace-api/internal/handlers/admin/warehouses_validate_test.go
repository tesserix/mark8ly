package admin

import "testing"

func TestValidateWarehouseWriteRequiresRegionWhereCarriersDo(t *testing.T) {
	base := WarehouseWriteRequest{
		Name: "Bondi", Line1: "1 Campbell Pde", City: "Bondi Beach",
		PostalCode: "2026", Phone: "+61290000000",
	}

	cases := []struct {
		name    string
		country string
		region  string
		wantErr bool
	}{
		{"AU without a state", "AU", "", true},
		{"AU with blank state", "au", "  ", true},
		{"AU with a state", "AU", "NSW", false},
		{"US without a state", "US", "", true},
		{"NZ without a region", "NZ", "", false},
		{"GB without a region", "GB", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			req.CountryCode = tc.country
			req.Region = tc.region
			err := validateWarehouseWrite(req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateWarehouseWrite(%s, %q) error = %v, wantErr %v",
					tc.country, tc.region, err, tc.wantErr)
			}
		})
	}
}
