package apikeys_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
)

func TestAllScopes_ContainsExpectedSetV1(t *testing.T) {
	all := apikeys.AllScopes()
	for _, want := range []string{
		"products:read", "products:write",
		"orders:read", "orders:write",
		"customers:read", "customers:write",
		"categories:read", "categories:write",
		"coupons:read", "coupons:write",
	} {
		require.Contains(t, all, apikeys.Scope(want))
	}
	require.NotContains(t, all, apikeys.Scope("admin:all"))
	require.NotContains(t, all, apikeys.Scope("tenant:admin"))
}

func TestValidateScopes_AcceptsKnownSet(t *testing.T) {
	require.NoError(t, apikeys.ValidateScopes([]string{"products:read", "orders:read"}))
	require.NoError(t, apikeys.ValidateScopes(nil))
}

func TestValidateScopes_RejectsUnknown(t *testing.T) {
	require.Error(t, apikeys.ValidateScopes([]string{"products:read", "delete:everything"}))
	require.Error(t, apikeys.ValidateScopes([]string{"admin:all"}))
}

func TestIsReadOnlyScope(t *testing.T) {
	require.True(t, apikeys.IsReadOnlyScope("products:read"))
	require.False(t, apikeys.IsReadOnlyScope("products:write"))
	require.False(t, apikeys.IsReadOnlyScope("unknown"))
}

func TestAllReadOnly(t *testing.T) {
	require.True(t, apikeys.AllReadOnly([]string{"products:read", "orders:read"}))
	require.False(t, apikeys.AllReadOnly([]string{"products:read", "orders:write"}))
	require.True(t, apikeys.AllReadOnly(nil))
}
