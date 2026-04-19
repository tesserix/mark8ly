package validators_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

// Sample VAT IDs (digits-only portion is fine for VIES regex).
var euSamples = map[string]string{
	"IE": "1234567X",
	"DE": "123456789",
	"FR": "12345678901",
	"IT": "12345678901",
	"ES": "A12345678",
	"NL": "123456789B01",
}

func TestEU_VIES_ValidAcrossCountries(t *testing.T) {
	for country, vat := range euSamples {
		t.Run(country, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, fmt.Sprintf("/ms/%s/vat/%s", country, vat), r.URL.Path)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"valid": true, "name": "ACME EU SA", "address": "1 Rue Example",
				})
			}))
			defer srv.Close()

			v := validators.NewEU(srv.Client(), srv.URL).WithCountry(country)
			res, err := v.Validate(context.Background(), tax.ValidationRequest{
				Country: country, TaxID: vat, BusinessName: "Acme EU SA",
			})
			require.NoError(t, err)
			require.True(t, res.Valid)
			require.Equal(t, "ACME EU SA", res.RegistryName)
		})
	}
}

func TestEU_VIES_PrivacyProtectedNoInformation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "name": "", "address": ""})
	}))
	defer srv.Close()

	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("DE")
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "DE", TaxID: "123456789",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	require.Equal(t, "", res.RegistryName)
}

func TestEU_VIES_InvalidMappedToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": false, "name": "", "address": ""})
	}))
	defer srv.Close()

	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("FR")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "FR", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestEU_VIES_404_MappedToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("IT")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IT", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestEU_VIES_503_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("ES")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "ES", TaxID: "A12345678",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestEU_VIES_504_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()
	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("NL")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "NL", TaxID: "123456789B01",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestEU_VIES_5xx_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("DE")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "DE", TaxID: "123456789",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestEU_VIES_Timeout_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	client := &http.Client{Timeout: 30 * time.Millisecond}
	v := validators.NewEU(client, srv.URL).WithCountry("IE")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IE", TaxID: "1234567X",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestEU_VIES_BadFormat(t *testing.T) {
	v := validators.NewEU(http.DefaultClient, "http://unused").WithCountry("DE")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "DE", TaxID: "??",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestEU_VIES_WrongCountry(t *testing.T) {
	v := validators.NewEU(http.DefaultClient, "http://unused").WithCountry("DE")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "FR", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestEU_VIES_UnsupportedCountry(t *testing.T) {
	v := validators.NewEU(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "ZZ", TaxID: "123456789",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestEU_WithCountry_ReturnsShallowCopy(t *testing.T) {
	v := validators.NewEU(nil, "")
	require.Equal(t, "", v.Country())
	bound := v.WithCountry("FR")
	require.Equal(t, "FR", bound.Country())
	require.Equal(t, "", v.Country(), "original must not mutate")
}

func TestEU_VIES_StripsCountryPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ms/DE/vat/123456789", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "name": "X"})
	}))
	defer srv.Close()
	v := validators.NewEU(srv.Client(), srv.URL).WithCountry("DE")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "DE", TaxID: "DE123456789",
	})
	require.NoError(t, err)
}
