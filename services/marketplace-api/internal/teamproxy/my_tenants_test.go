package teamproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// ListMyTenants is the mobile app's ONLY route to a tenant id after a
// Zitadel sign-in (#686): every mobile admin route is tenant-gated, and a
// Zitadel token carries no tenant claim, so without this the client has
// nothing to put in X-Acting-Tenant-Id.
func TestListMyTenants_CallsPublicRouteWithUID(t *testing.T) {
	var gotPath, gotQuery, gotInternalAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("uid")
		gotInternalAuth = r.Header.Get("X-Internal-Auth")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"tenant_id":"t-1","name":"Mumbai Spice Co","role":"owner"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "s3cret", srv.Client())
	got, err := c.ListMyTenants(context.Background(), "389396765696066342")
	require.NoError(t, err)

	// platform-api mounts listMyTenants on the PUBLIC /api/v1 group, not
	// the /internal one, so the path must not be guessed from the
	// /internal/* siblings on this client.
	require.Equal(t, "/api/v1/users/me/tenants", gotPath)
	require.Equal(t, "389396765696066342", gotQuery)
	// Harmless on a public route, but the client sends it uniformly and
	// platform-api ignores it there.
	require.Equal(t, "s3cret", gotInternalAuth)

	require.Len(t, got, 1)
	require.Equal(t, "t-1", got[0].TenantID)
	require.Equal(t, "Mumbai Spice Co", got[0].Name)
	require.Equal(t, "owner", got[0].Role)
}

// Zero tenants is a legitimate answer (a signed-in user who has not
// onboarded), and must be an empty list rather than an error — the client
// distinguishes "no stores yet" from "lookup failed" and shows very
// different screens.
func TestListMyTenants_EmptyListIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "", srv.Client()).ListMyTenants(context.Background(), "uid")
	require.NoError(t, err)
	require.Empty(t, got)
}

// An email is a valid identity key here: platform-api's ListMemberTenants
// resolves either an email- or a uid-keyed FGA tuple, and mark8ly writes
// BOTH for the same human. The client must pass it through untouched —
// platform-api owns the lowercasing.
func TestListMyTenants_PassesEmailKeyThroughUnchanged(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("uid")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", srv.Client()).ListMyTenants(context.Background(), "Demo+India@Mark8ly.com")
	require.NoError(t, err)
	require.Equal(t, "Demo+India@Mark8ly.com", gotQuery)
}

func TestListMyTenants_PlatformErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal","message":"boom"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", srv.Client()).ListMyTenants(context.Background(), "uid")
	require.Error(t, err)
}
