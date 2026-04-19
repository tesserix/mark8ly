package validators_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestUK_VAT_HMRCReturnsValid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/organisations/vat/check-vat-number/lookup/GB123456789", r.URL.Path)
		require.Equal(t, "application/vnd.hmrc.2.0+json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"target": map[string]any{
				"name":      "ACME WIDGETS LTD",
				"vatNumber": "123456789",
			},
		})
	}))
	defer srv.Close()

	v := validators.NewUK(srv.Client(), srv.URL)
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "GB123456789", BusinessName: "Acme Widgets Ltd",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	require.Equal(t, "ACME WIDGETS LTD", res.RegistryName)
}

func TestUK_VAT_HMRCNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "NOT_FOUND"})
	}))
	defer srv.Close()

	v := validators.NewUK(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "GB999999999",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestUK_VAT_HMRC5xx_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	v := validators.NewUK(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "GB123456789",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestUK_VAT_Timeout_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 30 * time.Millisecond}
	v := validators.NewUK(client, srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "GB123456789",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestUK_VAT_BadFormat(t *testing.T) {
	v := validators.NewUK(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "not-a-vat",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestUK_VAT_WrongCountry(t *testing.T) {
	v := validators.NewUK(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "US", TaxID: "GB123456789",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestUK_Country(t *testing.T) {
	require.Equal(t, "GB", validators.NewUK(nil, "").Country())
}
