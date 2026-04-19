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

func TestIN_GSTN_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/commonapi/v1.1/search", r.URL.Path)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"gstin": "27AABCU9603R1ZM",
				"lgnm":  "ACME PRIVATE LIMITED",
				"sts":   "Active",
			},
		})
	}))
	defer srv.Close()

	v := validators.NewIN(srv.Client(), srv.URL).WithAuthToken("test-token")
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IN", TaxID: "27AABCU9603R1ZM", BusinessName: "Acme Private Limited",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	require.Equal(t, "ACME PRIVATE LIMITED", res.RegistryName)
}

func TestIN_GSTN_InactiveRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"gstin": "27AABCU9603R1ZM", "lgnm": "Old Co", "sts": "Cancelled",
			},
		})
	}))
	defer srv.Close()
	v := validators.NewIN(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IN", TaxID: "27AABCU9603R1ZM",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestIN_GSTN_404_MappedToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	v := validators.NewIN(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IN", TaxID: "27AABCU9603R1ZM",
	})
	require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestIN_GSTN_429_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	v := validators.NewIN(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IN", TaxID: "27AABCU9603R1ZM",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestIN_GSTN_5xx_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	v := validators.NewIN(srv.Client(), srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IN", TaxID: "27AABCU9603R1ZM",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestIN_GSTN_Timeout_MappedToUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	client := &http.Client{Timeout: 30 * time.Millisecond}
	v := validators.NewIN(client, srv.URL)
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "IN", TaxID: "27AABCU9603R1ZM",
	})
	require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestIN_GSTN_FormatRegex(t *testing.T) {
	v := validators.NewIN(http.DefaultClient, "http://unused")
	for _, bad := range []string{
		"",
		"tooshort",
		"27AABCU9603R1Z",  // 14 chars
		"99AABCU9603R1ZM", // state code 99 invalid
	} {
		_, err := v.Validate(context.Background(), tax.ValidationRequest{Country: "IN", TaxID: bad})
		require.ErrorIsf(t, err, tax.ErrInvalidFormat, "expected invalid for %q", bad)
	}
}

func TestIN_GSTN_WrongCountry(t *testing.T) {
	v := validators.NewIN(http.DefaultClient, "http://unused")
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "27AABCU9603R1ZM",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestIN_Country(t *testing.T) {
	require.Equal(t, "IN", validators.NewIN(nil, "").Country())
}
