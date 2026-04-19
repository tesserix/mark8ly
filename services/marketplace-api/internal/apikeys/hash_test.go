package apikeys_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
)

func TestHashAndVerify_RoundTrip(t *testing.T) {
	plaintext := "mk8_live_testtesttesttesttesttesttesttest12345"
	h, err := apikeys.Hash(plaintext)
	require.NoError(t, err)
	require.NotEqual(t, plaintext, h)
	require.NoError(t, apikeys.Verify(h, plaintext))
	require.Error(t, apikeys.Verify(h, plaintext+"x"))
}

func TestVerifyDummy_AlwaysErrors(t *testing.T) {
	require.Error(t, apikeys.VerifyDummy("anything"))
	require.Error(t, apikeys.VerifyDummy(""))
}
