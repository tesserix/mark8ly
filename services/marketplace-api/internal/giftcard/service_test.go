package giftcard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCode(t *testing.T) {
	code, err := GenerateCode()
	require.NoError(t, err)
	assert.Len(t, code, 26, "code must be 26 characters (128-bit entropy)")

	// All characters should be valid base32 (A-Z, 2-7).
	for _, c := range code {
		assert.True(t, (c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7'),
			"invalid base32 character: %c", c)
	}

	// Two generated codes should be different (probabilistic).
	code2, err := GenerateCode()
	require.NoError(t, err)
	assert.NotEqual(t, code, code2, "two codes should differ")
}

func TestFormatCodeDisplay(t *testing.T) {
	// 26-char code gets formatted with dashes.
	code := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	formatted := FormatCodeDisplay(code)
	assert.Equal(t, "ABCD-EFGH-IJKL-MNOP-QRST-UVWX-YZ", formatted)

	// Non-26-char code is returned as-is.
	assert.Equal(t, "SHORT", FormatCodeDisplay("SHORT"))
}
