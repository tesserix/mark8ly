package customererasure

import (
	"fmt"

	"github.com/google/uuid"
)

// Token is the value that replaces a subject's email everywhere the row must
// survive erasure. It is derived from the ERASURE REQUEST ID, not from the
// email: a hash of the email would still be a pseudonym of the personal data
// and brute-forceable against a known address list, whereas the request id is
// unrelated to the person and already exists.
//
// Deterministic per request, so two anonymised orders by the same subject
// still group together — which is what makes the financial record coherent
// after erasure — while identifying nobody.
//
// .invalid is reserved by RFC 2606 and can never be routed, so an anonymised
// address cannot accidentally receive mail.
func Token(requestID uuid.UUID) string {
	return fmt.Sprintf("erased+%s@erased.invalid", requestID.String())
}

// RedactedName replaces a person's name where the column is NOT NULL.
const RedactedName = "Erased customer"

// RedactedLine replaces a NOT NULL address line.
const RedactedLine = "[erased]"
