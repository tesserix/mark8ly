package tenantlifecycle_test

import (
	"context"
	"errors"
	"io"
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

func TestTeardown_DecodesResult(t *testing.T) {
	var gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotAuth = string(b), r.Header.Get("X-Internal-Auth")
		require.Equal(t, "/internal/tenants/t-1/teardown", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"tenant_id":"t-1","tenant_name":"The Bondi Store","store_ids":["s-1"],"store_slugs":["the-bondi-store"]}}`))
	}))
	defer srv.Close()

	res, err := tenantlifecycle.NewClient(srv.URL, "shh", nil).Teardown(context.Background(), "t-1", []string{"the-bondi-store"})

	require.NoError(t, err)
	require.JSONEq(t, `{"store_slugs":["the-bondi-store"]}`, gotBody)
	require.Equal(t, "shh", gotAuth)
	require.Equal(t, "The Bondi Store", res.TenantName)
	require.Equal(t, []string{"s-1"}, res.StoreIDs)
}

func TestTeardown_EmptySlugSetIsSentAsAnArrayNotNull(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"data":{"tenant_id":"t-1","store_ids":[],"store_slugs":[]}}`))
	}))
	defer srv.Close()

	_, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})

	require.NoError(t, err)
	// A nil slice marshals to null, which upstream reads as ABSENT and
	// refuses with 400. The two are one character apart on the wire.
	require.JSONEq(t, `{"store_slugs":[]}`, gotBody)
}

func TestTeardown_409CarriesTheExpectedSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"confirmation_mismatch","expected":["a","b"]}`))
	}))
	defer srv.Close()

	_, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{"wrong"})

	var me *tenantlifecycle.ConfirmationMismatchError
	require.True(t, errors.As(err, &me), "want *ConfirmationMismatchError, got %T", err)
	require.Equal(t, []string{"a", "b"}, me.Expected)
}

func TestTeardown_404IsNotFound_500IsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, tenantlifecycle.ErrNotFound},
		{http.StatusInternalServerError, tenantlifecycle.ErrUnavailable},
		{http.StatusServiceUnavailable, tenantlifecycle.ErrUnavailable},
		{http.StatusBadGateway, tenantlifecycle.ErrUnavailable},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		_, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})
		require.ErrorIs(t, err, tc.want, "status %d", tc.status)
		srv.Close()
	}
}

// A 200 with a broken body must be an error, never a zero result — the
// failure mode this package's doc comment was written about.
func TestTeardown_TruncatedBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"tenant_id":`))
	}))
	defer srv.Close()

	res, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})
	require.Error(t, err)
	require.Nil(t, res)
}
