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
		// The contract is "200, 404 and 409 are the only recognised
		// statuses; anything else is ErrUnavailable". Without a case
		// outside {200, 404, 409, 5xx} the `default:` branch is never
		// pinned to that answer for a NON-5xx status (#342). 400 is the
		// error-shaped one; 204 is the success-shaped one — a 2xx that is
		// still not a result, which is the easier of the two to get wrong.
		{"bad request", 400, `{"error":"bad_request"}`, tenantlifecycle.ErrUnavailable},
		{"no content", 204, ``, tenantlifecycle.ErrUnavailable},
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
	// nil and []string{} are the two values this property discriminates
	// between: a nil slice marshals to `null`, which upstream reads as an
	// ABSENT confirmation and refuses with 400. Only the nil case can fail
	// if the guard is removed, so only the nil case actually tests it —
	// but both are asserted, because both are legal inputs that must reach
	// upstream as the same thing.
	for _, tc := range []struct {
		name  string
		slugs []string
	}{
		{"nil slice", nil},
		{"empty non-nil slice", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				_, _ = w.Write([]byte(`{"data":{"tenant_id":"t-1","store_ids":[],"store_slugs":[]}}`))
			}))
			defer srv.Close()

			_, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", tc.slugs)

			require.NoError(t, err)
			require.JSONEq(t, `{"store_slugs":[]}`, gotBody)
		})
	}
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
		body   string
		want   error
	}{
		// A 404 carrying the handler's error envelope IS "no such tenant".
		{http.StatusNotFound, `{"error":"tenant_not_found","message":"tenant does not exist"}`, tenantlifecycle.ErrNotFound},
		{http.StatusInternalServerError, "", tenantlifecycle.ErrUnavailable},
		{http.StatusServiceUnavailable, "", tenantlifecycle.ErrUnavailable},
		{http.StatusBadGateway, "", tenantlifecycle.ErrUnavailable},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})
		require.ErrorIs(t, err, tc.want, "status %d", tc.status)
		srv.Close()
	}
}

// A 404 from the HANDLER and a 404 from an UNMOUNTED ROUTE are the two
// values this property discriminates between, and conflating them is a
// safety bug, not a nicety.
//
// Gin answers a bare `404 page not found` for a path it has no route for.
// During a rolling deploy where marketplace-api ships ahead of platform-api
// that is what a teardown gets. Mapped to ErrNotFound it surfaces to the
// operator as `404 tenant_not_found`, which this API's contract defines as
// "including already purged" — so an operator processing a GDPR erasure
// reads "already destroyed" and closes the ticket on a live tenant.
//
// A bare body must therefore be ErrUpstreamRouteMissing, which wraps
// ErrUnavailable so the handler answers 503: honest about not knowing.
func TestTeardown_Bare404IsRouteMissingNotTenantMissing(t *testing.T) {
	for _, tc := range []struct {
		name         string
		body         string
		wantIs       error
		wantNotFound bool
	}{
		{
			name:         "gin unmatched route",
			body:         "404 page not found",
			wantIs:       tenantlifecycle.ErrUpstreamRouteMissing,
			wantNotFound: false,
		},
		{
			name:         "empty body",
			body:         "",
			wantIs:       tenantlifecycle.ErrUpstreamRouteMissing,
			wantNotFound: false,
		},
		{
			name:         "well-formed JSON with no error code",
			body:         `{}`,
			wantIs:       tenantlifecycle.ErrUpstreamRouteMissing,
			wantNotFound: false,
		},
		{
			name:         "handler error envelope",
			body:         `{"error":"tenant_not_found","message":"tenant does not exist"}`,
			wantIs:       tenantlifecycle.ErrNotFound,
			wantNotFound: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := tenantlifecycle.NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})
			require.ErrorIs(t, err, tc.wantIs)
			require.Equal(t, tc.wantNotFound, errors.Is(err, tenantlifecycle.ErrNotFound),
				"a bare 404 must NOT read as tenant_not_found; an enveloped one must")
			if !tc.wantNotFound {
				require.ErrorIs(t, err, tenantlifecycle.ErrUnavailable,
					"ErrUpstreamRouteMissing must wrap ErrUnavailable so the handler answers 503")
			}
		})
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
