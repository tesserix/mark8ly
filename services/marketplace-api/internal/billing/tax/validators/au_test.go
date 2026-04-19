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

func TestAU_ABN_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/json/AbnDetails.aspx", r.URL.Path)
		require.Equal(t, "12345678901", r.URL.Query().Get("abn"))
		require.Equal(t, "test-guid", r.URL.Query().Get("guid"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"AbnStatus": "Active", "EntityName": "ACME PTY LTD",
		})
	}))
	defer srv.Close()
	v := validators.NewAU(srv.Client(), srv.URL).WithGUID("test-guid")
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "AU", TaxID: "12345678901", BusinessName: "Acme Pty Ltd",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	require.Equal(t, "ACME PTY LTD", res.RegistryName)
}

func TestAU_ABN_CancelledMappedToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"AbnStatus": "Cancelled", "EntityName": "Old Co",
		})
	}))
	defer srv.Close()
	v := validators.NewAU(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "AU", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestAU_ABN_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	v := validators.NewAU(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "AU", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestAU_ABN_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	v := validators.NewAU(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "AU", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestAU_ABN_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	client := &http.Client{Timeout: 30 * time.Millisecond}
	v := validators.NewAU(client, srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "AU", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestAU_ABN_BadFormat(t *testing.T) {
	v := validators.NewAU(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "AU", TaxID: "not-an-abn",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestAU_ABN_WrongCountry(t *testing.T) {
	v := validators.NewAU(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "12345678901",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestAU_Country(t *testing.T) {
	require.Equal(t, "AU", validators.NewAU(nil, "").Country())
}
