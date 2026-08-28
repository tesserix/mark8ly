package emailevents_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/emailevents"
)

// signLikeResend produces the header set a provider would send, so these
// tests exercise the real scheme rather than the verifier's own arithmetic
// reflected back at it.
func signLikeResend(t *testing.T, secret, id string, ts time.Time, body []byte) (string, string, string) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(secret[len("whsec_"):])
	require.NoError(t, err)

	tss := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, raw)
	mac.Write([]byte(id + "." + tss + "."))
	mac.Write(body)
	return id, tss, "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func testSecret(t *testing.T) string {
	t.Helper()
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte("a-32-byte-signing-key-for-tests!"))
}

func TestVerify_AcceptsAGenuineSignature(t *testing.T) {
	secret := testSecret(t)
	body := []byte(`{"type":"email.delivered"}`)
	now := time.Now()
	id, ts, sig := signLikeResend(t, secret, "msg_1", now, body)

	require.NoError(t, emailevents.Verify(body, id, ts, sig, secret, now))
}

// The whole point of the endpoint. A body altered after signing must fail,
// or an attacker rewrites the event and we record their version.
func TestVerify_RejectsATamperedBody(t *testing.T) {
	secret := testSecret(t)
	now := time.Now()
	id, ts, sig := signLikeResend(t, secret, "msg_1", now, []byte(`{"type":"email.delivered"}`))

	err := emailevents.Verify([]byte(`{"type":"email.bounced"}`), id, ts, sig, secret, now)
	require.ErrorIs(t, err, emailevents.ErrBadSignature)
}

func TestVerify_RejectsAnotherSecret(t *testing.T) {
	body := []byte(`{"type":"email.delivered"}`)
	now := time.Now()
	id, ts, sig := signLikeResend(t, testSecret(t), "msg_1", now, body)

	other := "whsec_" + base64.StdEncoding.EncodeToString([]byte("a-DIFFERENT-32-byte-signing-key!"))
	require.ErrorIs(t, emailevents.Verify(body, id, ts, sig, other, now), emailevents.ErrBadSignature)
}

// A captured delivery replayed later must not be accepted. Without a
// freshness bound the signature stays valid forever.
func TestVerify_RejectsAStaleDelivery(t *testing.T) {
	secret := testSecret(t)
	body := []byte(`{"type":"email.delivered"}`)
	signedAt := time.Now().Add(-30 * time.Minute)
	id, ts, sig := signLikeResend(t, secret, "msg_1", signedAt, body)

	require.ErrorIs(t, emailevents.Verify(body, id, ts, sig, secret, time.Now()),
		emailevents.ErrStaleSignature)
}

// Clock skew cuts both ways: a timestamp slightly in the FUTURE is normal,
// far in the future is not.
func TestVerify_RejectsAFutureDatedDelivery(t *testing.T) {
	secret := testSecret(t)
	body := []byte(`{"type":"email.delivered"}`)
	signedAt := time.Now().Add(30 * time.Minute)
	id, ts, sig := signLikeResend(t, secret, "msg_1", signedAt, body)

	require.ErrorIs(t, emailevents.Verify(body, id, ts, sig, secret, time.Now()),
		emailevents.ErrStaleSignature)
}

// The signature header carries a space-separated list of versioned
// signatures; a v1 entry anywhere in it must be honoured, and an unknown
// version alone must not pass.
func TestVerify_HandlesMultipleSignaturesAndIgnoresUnknownVersions(t *testing.T) {
	secret := testSecret(t)
	body := []byte(`{"type":"email.delivered"}`)
	now := time.Now()
	id, ts, sig := signLikeResend(t, secret, "msg_1", now, body)

	require.NoError(t, emailevents.Verify(body, id, ts, "v2,bogus "+sig, secret, now))
	require.ErrorIs(t, emailevents.Verify(body, id, ts, "v2,"+sig[3:], secret, now),
		emailevents.ErrBadSignature)
}

// An unconfigured secret must refuse, never accept. A verifier that passes
// when it has nothing to check is worse than no verifier.
func TestVerify_RefusesWhenUnconfigured(t *testing.T) {
	body := []byte(`{"type":"email.delivered"}`)
	now := time.Now()
	id, ts, sig := signLikeResend(t, testSecret(t), "msg_1", now, body)

	require.ErrorIs(t, emailevents.Verify(body, id, ts, sig, "", now), emailevents.ErrNotConfigured)
}

func TestVerify_RejectsMalformedHeaders(t *testing.T) {
	secret := testSecret(t)
	body := []byte(`{}`)
	now := time.Now()

	for _, tc := range []struct{ name, id, ts, sig string }{
		{"no id", "", "1", "v1,x"},
		{"no timestamp", "msg_1", "", "v1,x"},
		{"timestamp not a number", "msg_1", "not-a-number", "v1,x"},
		{"no signature", "msg_1", "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, emailevents.Verify(body, tc.id, tc.ts, tc.sig, secret, now))
		})
	}
}
