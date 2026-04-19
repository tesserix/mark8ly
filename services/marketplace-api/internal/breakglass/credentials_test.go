package breakglass

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGeneratePassword_20CharsWithAllClasses(t *testing.T) {
	reUpper := regexp.MustCompile(`[A-Z]`)
	reLower := regexp.MustCompile(`[a-z]`)
	reDigit := regexp.MustCompile(`[0-9]`)
	reSymbol := regexp.MustCompile(`[!#$%&*+\-=?@^_~]`)

	for i := 0; i < 100; i++ {
		p, err := GeneratePassword()
		require.NoError(t, err)
		require.Len(t, p, passwordLen)
		require.True(t, reUpper.MatchString(p), "missing upper in %q", p)
		require.True(t, reLower.MatchString(p), "missing lower in %q", p)
		require.True(t, reDigit.MatchString(p), "missing digit in %q", p)
		require.True(t, reSymbol.MatchString(p), "missing symbol in %q", p)

		// Ambiguous characters MUST NOT appear.
		require.NotContainsf(t, p, "I", "ambiguous 'I' present in %q", p)
		require.NotContainsf(t, p, "O", "ambiguous 'O' present in %q", p)
		require.NotContainsf(t, p, "l", "ambiguous 'l' present in %q", p)
		require.NotContainsf(t, p, "o", "ambiguous 'o' present in %q", p)
		require.NotContainsf(t, p, "0", "ambiguous '0' present in %q", p)
		require.NotContainsf(t, p, "1", "ambiguous '1' present in %q", p)
	}
}

func TestGeneratePassword_Entropy(t *testing.T) {
	// Two successive calls must not collide — would mean rand.Reader is
	// unseeded. Repeat collisions are astronomically unlikely at 20 chars.
	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		p, err := GeneratePassword()
		require.NoError(t, err)
		require.False(t, seen[p], "collision at iteration %d: %q", i, p)
		seen[p] = true
	}
}

func TestTOTP_GenerateVerifyRoundTrip(t *testing.T) {
	s, err := GenerateTOTPSecret()
	require.NoError(t, err)
	require.NotEmpty(t, s)

	code, err := TOTPCode(s, time.Now())
	require.NoError(t, err)
	require.Len(t, code, 6)
	require.True(t, VerifyTOTP(s, code, time.Now()))
}

func TestTOTP_RejectsOldCode(t *testing.T) {
	s, err := GenerateTOTPSecret()
	require.NoError(t, err)

	old, err := TOTPCode(s, time.Now().Add(-5*time.Minute))
	require.NoError(t, err)
	require.False(t, VerifyTOTP(s, old, time.Now()))
}

func TestTOTP_RejectsWrongCode(t *testing.T) {
	s, err := GenerateTOTPSecret()
	require.NoError(t, err)
	require.False(t, VerifyTOTP(s, "000000", time.Now()))
}

func TestOTPAuthURI_ShapeAndParams(t *testing.T) {
	tenantID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	uri := OTPAuthURI("BASE32SECRETXYZ", tenantID)

	require.True(t, strings.HasPrefix(uri, "otpauth://totp/"))
	require.Contains(t, uri, "issuer=Mark8ly")
	require.Contains(t, uri, "algorithm=SHA1")
	require.Contains(t, uri, "digits=6")
	require.Contains(t, uri, "period=30")
	require.Contains(t, uri, "secret=BASE32SECRETXYZ")
	require.Contains(t, uri, tenantID.String())
}
