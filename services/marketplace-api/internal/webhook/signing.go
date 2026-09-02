package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// SignatureHeader carries the signature on every delivery.
const SignatureHeader = "X-Mark8ly-Signature"

// Sign returns "t=<unix>,v1=<hex>" over "<unix>.<body>" using secret.
//
// The timestamp is part of the SIGNED material, not merely a sibling header:
// that is what lets a merchant reject a replayed delivery by checking the
// timestamp is recent, knowing an attacker cannot rewrite it without
// invalidating v1. The format mirrors Stripe's so the verification recipe is
// already familiar to most integrators.
func Sign(secret string, ts time.Time, body []byte) string {
	unix := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unix))
	mac.Write([]byte("."))
	mac.Write(body)
	return fmt.Sprintf("t=%s,v1=%s", unix, hex.EncodeToString(mac.Sum(nil)))
}

// GenerateSecret returns 32 random bytes hex-encoded. Shown to the merchant
// once at creation and never returned again.
func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("webhook: generate secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
