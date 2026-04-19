package breakglass

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Password composition keeps each class distinct so rejection-sampling
// terminates quickly. The ambiguous pairs I/O, l/o, 0/1 are excluded so
// the password survives being read over the phone by an on-call
// responder without transcription errors.
const (
	passwordLen = 20

	upperPool  = "ABCDEFGHJKLMNPQRSTUVWXYZ"       // no I, O
	lowerPool  = "abcdefghijkmnpqrstuvwxyz"       // no l, o
	digitPool  = "23456789"                       // no 0, 1
	symbolPool = "!#$%&*+-=?@^_~"
)

// combinedPool lists every legal character — used during the random
// walk so the rejection loop still produces uniformly random output.
var combinedPool = upperPool + lowerPool + digitPool + symbolPool

// GeneratePassword returns a 20-char CSPRNG password guaranteed to
// contain at least one character from each class (upper, lower,
// digit, symbol). Rejection-samples until the invariant holds —
// expected iterations under 2 for passwordLen=20.
func GeneratePassword() (string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		buf := make([]byte, passwordLen)
		for i := range buf {
			idx, err := randInt(len(combinedPool))
			if err != nil {
				return "", err
			}
			buf[i] = combinedPool[idx]
		}
		p := string(buf)
		if hasAllClasses(p) {
			return p, nil
		}
	}
	return "", errors.New("breakglass: exhausted attempts generating password")
}

// hasAllClasses checks that p contains at least one of each class.
func hasAllClasses(p string) bool {
	var up, lo, di, sy bool
	for _, c := range p {
		switch {
		case strings.ContainsRune(upperPool, c):
			up = true
		case strings.ContainsRune(lowerPool, c):
			lo = true
		case strings.ContainsRune(digitPool, c):
			di = true
		case strings.ContainsRune(symbolPool, c):
			sy = true
		}
	}
	return up && lo && di && sy
}

// GenerateTOTPSecret produces a fresh RFC 6238 TOTP secret (32 bytes
// base32). Returned as a string in the exact shape pquerna/otp expects
// for downstream TOTPCode / VerifyTOTP calls.
func GenerateTOTPSecret() (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Mark8ly",
		AccountName: "break-glass",
		SecretSize:  32,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", err
	}
	return key.Secret(), nil
}

// TOTPCode produces the 6-digit code for `secret` at wall-clock `t`.
// Used by tests and by P16's QR bootstrap flow.
func TOTPCode(secret string, t time.Time) (string, error) {
	return totp.GenerateCode(secret, t)
}

// VerifyTOTP returns true if code is valid for secret at `at`. Skew is
// hard-coded to 1 (±30s window) — §12.4 says mandatory TOTP, not
// lenient. Anything wider is a well-known drift exploit.
func VerifyTOTP(secret, code string, at time.Time) bool {
	ok, err := totp.ValidateCustom(code, secret, at, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false
	}
	return ok
}

// OTPAuthURI returns the otpauth:// URI that P16 renders as a QR code
// during first-time enrolment. Format per RFC 6238 / Google Authenticator:
//
//	otpauth://totp/Mark8ly:break-glass-{tenantID}?secret=...&issuer=Mark8ly
//	  &algorithm=SHA1&digits=6&period=30
func OTPAuthURI(secret string, tenantID uuid.UUID) string {
	label := fmt.Sprintf("Mark8ly:break-glass-%s", tenantID.String())
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", "Mark8ly")
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + url.PathEscape(label) + "?" + q.Encode()
}

// randInt returns a uniform random int in [0, max) using crypto/rand.
func randInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
