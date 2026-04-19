package sso

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttrMap_Resolve_SimpleKeys(t *testing.T) {
	claims := map[string]any{
		"email":      "u@x",
		"given_name": "Jane",
		"groups":     []any{"admins", "staff"},
	}
	m := AttrMapping{
		"email":     "claims.email",
		"firstName": "claims.given_name",
		"role":      "claims.groups[0]",
	}
	out, err := m.Resolve(map[string]any{"claims": claims})
	require.NoError(t, err)
	require.Equal(t, "u@x", out["email"])
	require.Equal(t, "Jane", out["firstName"])
	require.Equal(t, "admins", out["role"])
}

func TestAttrMap_Resolve_MissingPath_Skipped(t *testing.T) {
	out, err := AttrMapping{"firstName": "claims.given_name"}.Resolve(
		map[string]any{"claims": map[string]any{"email": "x"}},
	)
	require.NoError(t, err)
	_, ok := out["firstName"]
	require.False(t, ok)
}

func TestAttrMap_Resolve_NestedPath(t *testing.T) {
	claims := map[string]any{
		"profile": map[string]any{
			"name": map[string]any{"first": "Jane"},
		},
	}
	m := AttrMapping{"firstName": "claims.profile.name.first"}
	out, err := m.Resolve(map[string]any{"claims": claims})
	require.NoError(t, err)
	require.Equal(t, "Jane", out["firstName"])
}

func TestAttrMap_Validate_RejectsNonClaimsRoot(t *testing.T) {
	err := AttrMapping{"role": "profile.role"}.Validate()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidAttrMap))
}

func TestAttrMap_Validate_RejectsInvalidSegment(t *testing.T) {
	err := AttrMapping{"role": "claims.1bad"}.Validate()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidAttrMap))
}

func TestAttrMap_Validate_RejectsEmptyPathAfterClaims(t *testing.T) {
	require.Error(t, AttrMapping{"role": "claims."}.Validate())
}

func TestAttrMap_Validate_AllowsBracketIndex(t *testing.T) {
	require.NoError(t, AttrMapping{"role": "claims.groups[0]"}.Validate())
	require.NoError(t, AttrMapping{"role": "claims.groups[42]"}.Validate())
}

func TestAttrMap_Resolve_OutOfRangeIndex_Skipped(t *testing.T) {
	out, err := AttrMapping{"role": "claims.groups[5]"}.Resolve(
		map[string]any{"claims": map[string]any{"groups": []any{"a", "b"}}},
	)
	require.NoError(t, err)
	require.NotContains(t, out, "role")
}
