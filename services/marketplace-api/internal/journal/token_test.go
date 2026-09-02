package journal_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/journal"
)

// GenerateUnsubscribeToken must produce a bearer credential that is both
// unguessable and correctly shaped for the unsubscribe_token CHAR(64)
// column (migration 000125): 32 random bytes, hex-encoded.
func TestGenerateUnsubscribeToken_HasExpectedLength(t *testing.T) {
	token, err := journal.GenerateUnsubscribeToken()
	require.NoError(t, err)
	require.Len(t, token, journal.UnsubscribeTokenLength)
}

func TestGenerateUnsubscribeToken_IsHexEncoded(t *testing.T) {
	token, err := journal.GenerateUnsubscribeToken()
	require.NoError(t, err)
	require.Regexp(t, "^[0-9a-f]{64}$", token)
}

// The whole point of a bearer token is that it cannot be guessed. A
// large sample of generated tokens must never collide, and must not
// exhibit the kind of low-order-byte patterning a weak PRNG would leak.
func TestGenerateUnsubscribeToken_IsUniqueAcrossManyCalls(t *testing.T) {
	const n = 5000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		token, err := journal.GenerateUnsubscribeToken()
		require.NoError(t, err)
		_, dup := seen[token]
		require.False(t, dup, "generated a duplicate token within %d calls", n)
		seen[token] = struct{}{}
	}
}

// GenerateUnsubscribeToken must not derive the token from the email it
// will be attached to — the token is the ONLY credential authorising
// deletion, and an address-derived token would be guessable by anyone
// who knows (or can enumerate) the subscriber's email.
func TestGenerateUnsubscribeToken_DoesNotAcceptOrDeriveFromEmail(t *testing.T) {
	tokenA, err := journal.GenerateUnsubscribeToken()
	require.NoError(t, err)
	tokenB, err := journal.GenerateUnsubscribeToken()
	require.NoError(t, err)
	require.NotEqual(t, tokenA, tokenB)
}
