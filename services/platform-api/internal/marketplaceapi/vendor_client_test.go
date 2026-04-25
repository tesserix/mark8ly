package marketplaceapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVendorClient_EnsureSelfVendor_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/internal/tenants/tenant-abc/ensure-self-vendor", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var got map[string]string
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, "Acme", got["name"])
		require.Equal(t, "acme", got["slug"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":        "vendor-1",
				"tenant_id": "tenant-abc",
				"name":      "Acme",
				"slug":      "acme",
				"is_self":   true,
				"status":    "active",
			},
		})
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL)
	v, err := c.EnsureSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.NoError(t, err)
	require.Equal(t, "vendor-1", v.ID)
	require.Equal(t, "Acme", v.Name)
	require.True(t, v.IsSelf)
}

func TestVendorClient_EnsureSelfVendor_SendsInternalAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "shared-secret", r.Header.Get("X-Internal-Auth"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":        "vendor-1",
				"tenant_id": "tenant-abc",
				"name":      "Acme",
				"slug":      "acme",
				"is_self":   true,
				"status":    "active",
			},
		})
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL, "shared-secret")
	_, err := c.EnsureSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.NoError(t, err)
}

func TestVendorClient_EnsureSelfVendor_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","message":"down"}`))
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL)
	_, err := c.EnsureSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestVendorClient_EnsureSelfVendor_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // immediately kill the server so the client gets a connection refused

	c := NewVendorClient(srv.URL)
	_, err := c.EnsureSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.Error(t, err)
}

func TestVendorClient_UpdateSelfVendor_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/internal/tenants/tenant-abc/self-vendor", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var got map[string]string
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, "Acme", got["name"])
		require.Equal(t, "acme", got["slug"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":        "vendor-1",
				"tenant_id": "tenant-abc",
				"name":      "Acme",
				"slug":      "acme",
				"is_self":   true,
				"status":    "active",
			},
		})
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL)
	v, err := c.UpdateSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.NoError(t, err)
	require.Equal(t, "Acme", v.Name)
	require.Equal(t, "acme", v.Slug)
}

func TestVendorClient_UpdateSelfVendor_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"self-vendor not found"}`))
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL)
	_, err := c.UpdateSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}
