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

func TestSG_ACRA_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/uen/201912345A", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entityStatus": "Registered", "entityName": "ACME SG PTE LTD",
		})
	}))
	defer srv.Close()
	v := validators.NewSG(srv.Client(), srv.URL)
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "SG", TaxID: "201912345A", BusinessName: "Acme SG Pte Ltd",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	require.Equal(t, "ACME SG PTE LTD", res.RegistryName)
}

func TestSG_ACRA_StruckOffMappedToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entityStatus": "Struck Off", "entityName": "Old SG Co",
		})
	}))
	defer srv.Close()
	v := validators.NewSG(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "SG", TaxID: "201912345A",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestSG_ACRA_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	v := validators.NewSG(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "SG", TaxID: "201912345A",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestSG_ACRA_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	v := validators.NewSG(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "SG", TaxID: "201912345A",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestSG_ACRA_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	client := &http.Client{Timeout: 30 * time.Millisecond}
	v := validators.NewSG(client, srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "SG", TaxID: "201912345A",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestSG_ACRA_BadFormat(t *testing.T) {
	v := validators.NewSG(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "SG", TaxID: "not-a-uen",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestSG_ACRA_WrongCountry(t *testing.T) {
	v := validators.NewSG(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "201912345A",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestSG_Country(t *testing.T) {
	require.Equal(t, "SG", validators.NewSG(nil, "").Country())
}
