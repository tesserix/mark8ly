package breakglass

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoginRateLimitKey_EdgeCases pins the shape of the ONE function both
// internal/handlers/admin (login: records failures, resets on success) and
// internal/handlers/platformadmin (clear-lockout: resets the same bucket)
// call to derive a LoginRateLimiter bucket key from an ip_hash.
//
// Before LoginRateLimitKey existed, each package kept its own private copy
// of this logic with a comment promising they'd stay byte-identical — a
// promise nothing enforced. Now there is exactly one implementation, so
// drift between the two call sites is impossible by construction; this
// test instead pins the shape itself (truncate-at-16-bytes, including the
// boundary and degenerate inputs) so a future edit to LoginRateLimitKey
// that changes that shape fails loudly here rather than silently breaking
// clear-lockout for both routes at once.
func TestLoginRateLimitKey_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{
			name: "empty ip_hash",
			in:   []byte{},
			want: "",
		},
		{
			name: "nil ip_hash",
			in:   nil,
			want: "",
		},
		{
			name: "short ip_hash (under 16 bytes) is kept in full",
			in:   []byte{0x01, 0x02, 0x03},
			want: string([]byte{0x01, 0x02, 0x03}),
		},
		{
			name: "exactly 16 bytes is kept in full",
			in:   bytes16(0xAA),
			want: string(bytes16(0xAA)),
		},
		{
			name: "over 16 bytes (a real HMAC-SHA256 output, 32 bytes) is truncated to 16",
			in:   bytes32(0xBB),
			want: string(bytes32(0xBB)[:16]),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, LoginRateLimitKey(tc.in))
		})
	}
}

// TestLoginRateLimitKey_Deterministic pins that the function is a pure
// function of its input — the login path and the clear-lockout path must
// derive the SAME key from the SAME ip_hash every time, or a failure
// recorded on one request is invisible to a clear-lockout call made
// moments later.
func TestLoginRateLimitKey_Deterministic(t *testing.T) {
	h := bytes32(0xCC)
	require.Equal(t, LoginRateLimitKey(h), LoginRateLimitKey(h))
	require.Equal(t, LoginRateLimitKey(h), LoginRateLimitKey(append([]byte(nil), h...)))
}

func bytes16(b byte) []byte {
	out := make([]byte, 16)
	for i := range out {
		out[i] = b
	}
	return out
}

func bytes32(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}
