//go:build integration

package shipping_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// P18 §4.1 / §4.1.1 integration test — proves migration 000074 seeded IE,
// NZ, and VN into shipping_zones with the expected carrier + service +
// currency mapping. Uses NewTx so the seed rows (committed by migration
// outside the test tx) are visible and any test writes roll back cleanly.

func TestShippingZoneLookup_NewCountries(t *testing.T) {
	db := testdb.NewTx(t)
	repo := shipping.NewZoneRepository(db)

	cases := []struct {
		country     string
		wantCarrier string
		wantService string
		wantCurr    string
	}{
		{"IE", "shipengine", "an_post_parcel", "EUR"},
		{"NZ", "shipengine", "nz_post_tracked", "NZD"},
		{"VN", "ninjavan", "ninjavan_standard", "VND"},
	}

	for _, tc := range cases {
		t.Run(tc.country, func(t *testing.T) {
			z, err := repo.GetByCountry(context.Background(), tc.country)
			if err != nil {
				t.Fatalf("GetByCountry(%s): %v", tc.country, err)
			}
			if z.CarrierID != tc.wantCarrier {
				t.Errorf("CarrierID = %q, want %q", z.CarrierID, tc.wantCarrier)
			}
			if z.DefaultServiceCode != tc.wantService {
				t.Errorf("DefaultServiceCode = %q, want %q", z.DefaultServiceCode, tc.wantService)
			}
			if z.Currency != tc.wantCurr {
				t.Errorf("Currency = %q, want %q", z.Currency, tc.wantCurr)
			}
		})
	}
}

func TestShippingZoneLookup_LowercaseCountryCode_Normalized(t *testing.T) {
	db := testdb.NewTx(t)
	repo := shipping.NewZoneRepository(db)

	z, err := repo.GetByCountry(context.Background(), "ie")
	if err != nil {
		t.Fatalf("GetByCountry(ie): %v", err)
	}
	if z.CountryCode != "IE" {
		t.Errorf("CountryCode = %q, want IE (case normalization)", z.CountryCode)
	}
}

func TestShippingZoneLookup_UnknownCountry_ReturnsErrZoneNotFound(t *testing.T) {
	db := testdb.NewTx(t)
	repo := shipping.NewZoneRepository(db)

	_, err := repo.GetByCountry(context.Background(), "ZZ")
	if err == nil {
		t.Fatal("GetByCountry(ZZ) = nil, want ErrZoneNotFound")
	}
	if !errors.Is(err, shipping.ErrZoneNotFound) {
		t.Errorf("GetByCountry(ZZ) err = %v, want wraps ErrZoneNotFound", err)
	}
}

func TestShippingZoneLookup_EmptyCountry_ReturnsError(t *testing.T) {
	db := testdb.NewTx(t)
	repo := shipping.NewZoneRepository(db)

	if _, err := repo.GetByCountry(context.Background(), ""); err == nil {
		t.Fatal("GetByCountry(\"\") = nil, want error")
	}
}
