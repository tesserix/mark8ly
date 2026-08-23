package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

// ChallengePrefix labels our TXT record so merchants (and we) can tell it
// apart from the other verification tokens that accumulate on an apex.
const ChallengePrefix = "mark8ly-domain-verification="

// challengeLabel is prepended to the domain being claimed. An underscore
// label cannot collide with a real hostname.
const challengeLabel = "_mark8ly-challenge"

// ChallengeHost is the name the merchant publishes the TXT record at.
func ChallengeHost(domain string) string {
	return challengeLabel + "." + canonicalDomain(domain)
}

// ChallengeToken derives the value the merchant must publish. It is an
// HMAC over (tenant, domain) rather than a stored random string: the
// token is reproducible at verify time, so proving ownership needs no
// extra column and no expiry bookkeeping.
//
// Binding the tenant into the digest is the point — a token issued to
// one tenant proves nothing for another, so a merchant cannot publish a
// record they were legitimately given and have it satisfy a claim made
// on someone else's behalf.
func ChallengeToken(secret string, tenantID uuid.UUID, domain string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tenantID.String()))
	mac.Write([]byte{0})
	mac.Write([]byte(canonicalDomain(domain)))
	return ChallengePrefix + hex.EncodeToString(mac.Sum(nil))
}

// ChallengeMatches reports whether any TXT record carries the token.
// Comparison is constant-time so a mismatch leaks nothing by timing.
func ChallengeMatches(records []string, token string) bool {
	want := []byte(token)
	for _, rec := range records {
		got := []byte(strings.Trim(strings.TrimSpace(rec), `"`))
		if hmac.Equal(got, want) {
			return true
		}
	}
	return false
}

func canonicalDomain(d string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))
}
