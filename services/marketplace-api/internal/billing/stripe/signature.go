package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBadSignature    = errors.New("stripe: signature mismatch")
	ErrStaleSignature  = errors.New("stripe: signature too old")
	ErrMalformedHeader = errors.New("stripe: malformed Stripe-Signature header")
)

const maxSignatureAge = 5 * time.Minute

// VerifySignature validates Stripe's webhook signature scheme:
//
//	signed_payload = "<ts>.<raw_body>"; mac = HMAC-SHA256(secret, signed_payload)
//
// Rejects timestamps beyond +/- 5 min.
// Returns (eventID, eventType) on success.
func VerifySignature(rawBody []byte, header, secret string, now time.Time) (eventID, eventType string, err error) {
	ts, sigs, err := parseStripeSignatureHeader(header)
	if err != nil {
		return "", "", err
	}
	diff := now.Sub(time.Unix(ts, 0))
	if diff < 0 {
		diff = -diff
	}
	if diff > maxSignatureAge {
		return "", "", ErrStaleSignature
	}
	signedPayload := fmt.Sprintf("%d.%s", ts, rawBody)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	ok := false
	for _, s := range sigs {
		if hmac.Equal([]byte(expected), []byte(s)) {
			ok = true
			break
		}
	}
	if !ok {
		return "", "", ErrBadSignature
	}

	var e struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawBody, &e); err != nil {
		return "", "", fmt.Errorf("stripe: parse event body: %w", err)
	}
	return e.ID, e.Type, nil
}

// BuildSignatureForTesting produces a Stripe-Signature header value for tests.
// NEVER call this in production code — Stripe signs events on their side.
func BuildSignatureForTesting(rawBody []byte, secret string, now time.Time) string {
	ts := now.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", ts, rawBody)))
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func parseStripeSignatureHeader(h string) (ts int64, sigs []string, err error) {
	var t int64
	for _, part := range strings.Split(h, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return 0, nil, ErrMalformedHeader
		}
		switch kv[0] {
		case "t":
			t, err = strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return 0, nil, ErrMalformedHeader
			}
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if t == 0 || len(sigs) == 0 {
		return 0, nil, ErrMalformedHeader
	}
	return t, sigs, nil
}
