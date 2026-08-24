package tenantlifecycle_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
)

func TestSuspend_MapsStatusCodes(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		body    string
		wantErr error
	}{
		{"ok", 200, `{"data":{"tenant_id":"t1","status":"suspended","stores_affected":2,"changed":true}}`, nil},
		{"not found", 404, `{"error":"tenant_not_found"}`, tenantlifecycle.ErrNotFound},
		{"conflict", 409, `{"error":"invalid_status_transition"}`, tenantlifecycle.ErrConflict},
		{"server error", 500, `{"error":"internal_error"}`, tenantlifecycle.ErrUnavailable},
		{"bad gateway", 502, ``, tenantlifecycle.ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/internal/tenants/t1/suspend", r.URL.Path)
				require.NotEmpty(t, r.Header.Get("X-Internal-Auth"), "internal auth header must be sent")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := tenantlifecycle.NewClient(srv.URL, "secret", srv.Client()).
				Suspend(context.Background(), "t1")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got, "an error must never come back with a usable result")
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, got.StoresAffected)
			require.True(t, got.Changed)
			require.Equal(t, "suspended", got.Status)
		})
	}
}

// A 200 whose body is truncated or unparseable is an error, not a zero
// result. Conflating the two is how a caller ends up reporting "0 stores
// affected, changed: false" for a request that actually did something.
func TestSuspend_UnparseableBodyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{`))
	}))
	defer srv.Close()
	got, err := tenantlifecycle.NewClient(srv.URL, "secret", srv.Client()).
		Suspend(context.Background(), "t1")
	require.Error(t, err)
	require.Nil(t, got)
}
