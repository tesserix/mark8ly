package branding

// support_email.go — validation for the merchant's customer-facing
// contact address.
//
// This value is not merely stored. It is published in a Reply-To header
// on mail sent to that store's customers (#748) and interpolated into a
// mailto: link on the closed-store page. So it is validated at the write
// boundary rather than at each send site.

import (
	"net/mail"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// supportEmailMaxLen matches the column width (VARCHAR(255), migration
// 000069). Rejecting here turns a driver-level truncation error into a
// field-scoped validation message.
const supportEmailMaxLen = 255

// normaliseSupportEmail validates a candidate support address and returns
// the value to persist. An empty or whitespace-only input is the merchant
// clearing the field and is returned as "" — see the package doc on the
// empty case.
//
// Three checks, each for a distinct reason:
//
//  1. mail.ParseAddress with an exact round-trip. This rejects the
//     display-name form ("Nadia <n@x.com>") and anything carrying CR/LF,
//     because the value lands in a mail header verbatim and a header is
//     not a place to accept a caller-supplied structure.
//  2. email.ValidateRecipient — the SAME guard email.StoreIdentity applies
//     before it will use the address as Reply-To. Using a different (looser)
//     spelling here would let a merchant save an address that the sender
//     then silently discards, falling back to the platform with no
//     explanation. What can be saved is exactly what will be used.
//  3. Length, against the column width.
func normaliseSupportEmail(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	if len(v) > supportEmailMaxLen {
		return "", apperrors.ValidationFailed("support_email", "must be 255 characters or fewer")
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Name != "" || addr.Address != v {
		return "", apperrors.ValidationFailed("support_email",
			"must be a plain email address, e.g. hello@yourstore.com")
	}
	if err := email.ValidateRecipient(v); err != nil {
		return "", apperrors.ValidationFailed("support_email",
			"must be an address that can receive mail")
	}
	return v, nil
}
