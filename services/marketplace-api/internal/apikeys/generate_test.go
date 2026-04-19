package apikeys_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
)

func TestGenerate_LiveProducesValidKeyAndPrefix(t *testing.T) {
	got, err := apikeys.Generate(apikeys.EnvLive)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got.Plaintext, "mk8_live_"))
	require.Len(t, got.Prefix, 8)
	require.GreaterOrEqual(t, len(got.Plaintext), len("mk8_live_")+8+4)
	require.Regexp(t, `^mk8_live_[1-9A-HJ-NP-Za-km-z]{8}\*{4}[1-9A-HJ-NP-Za-km-z]{4}$`,
		apikeys.Display(got.Plaintext))
}

func TestGenerate_TestUsesTestPrefix(t *testing.T) {
	got, err := apikeys.Generate(apikeys.EnvTest)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got.Plaintext, "mk8_test_"))
}

func TestGenerate_FreshOutputsAreUnique(t *testing.T) {
	a, err := apikeys.Generate(apikeys.EnvLive)
	require.NoError(t, err)
	b, err := apikeys.Generate(apikeys.EnvLive)
	require.NoError(t, err)
	require.NotEqual(t, a.Plaintext, b.Plaintext)
	require.NotEqual(t, a.Prefix, b.Prefix)
}

func TestExtractPrefix_ValidInputs(t *testing.T) {
	gen, err := apikeys.Generate(apikeys.EnvLive)
	require.NoError(t, err)
	p, ok := apikeys.ExtractPrefix(gen.Plaintext)
	require.True(t, ok)
	require.Equal(t, gen.Prefix, p)
}

func TestExtractPrefix_RejectsJunk(t *testing.T) {
	for _, bad := range []string{
		"", "not-a-key", "mk8_live_", "mk8_live_SHORT",
		"sk_live_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
	} {
		_, ok := apikeys.ExtractPrefix(bad)
		require.Falsef(t, ok, "expected reject for %q", bad)
	}
}

func TestDisplay_HandlesShortInput(t *testing.T) {
	require.Equal(t, "", apikeys.Display("garbage"))
	require.Equal(t, "", apikeys.Display("mk8_live_short"))
}
