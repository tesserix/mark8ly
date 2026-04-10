package loyalty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateReferralCode(t *testing.T) {
	code, err := GenerateReferralCode()
	require.NoError(t, err)
	assert.Len(t, code, 10)
	// All uppercase alphanumeric (base32)
	for _, c := range code {
		assert.True(t, (c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7'), "unexpected char: %c", c)
	}

	// Two codes should be different (probabilistic but effectively guaranteed)
	code2, err := GenerateReferralCode()
	require.NoError(t, err)
	assert.NotEqual(t, code, code2)
}
