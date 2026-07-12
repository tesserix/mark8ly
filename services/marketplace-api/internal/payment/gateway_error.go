package payment

import (
	"errors"
	"fmt"
	"net/http"
)

// GatewayError is a structured error returned by a Gateway when the provider
// responds with a non-success HTTP status. It lets the refund saga decide
// whether re-driving the same request can ever succeed.
type GatewayError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("%s: gateway responded %d: %s", e.Provider, e.StatusCode, e.Body)
}

// Permanent reports whether a retry with the same request is futile. Client
// errors (4xx) mean the request itself is wrong — bad payment id, malformed
// amount, refund exceeds capturable, already fully refunded — and will never
// succeed on retry. The exceptions are 408 Request Timeout and 429 Too Many
// Requests, which are transient. 5xx and transport errors are transient.
func (e *GatewayError) Permanent() bool {
	switch e.StatusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return false
	}
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// IsPermanentGatewayError reports whether err (or any error it wraps) is a
// permanent gateway failure. The refund saga uses this to move a stuck ledger
// row to 'failed' instead of re-driving it forever. A non-GatewayError
// (network blip, context cancellation, decode error) is treated as transient
// by returning false — the safe default is to retry.
func IsPermanentGatewayError(err error) bool {
	var ge *GatewayError
	if errors.As(err, &ge) {
		return ge.Permanent()
	}
	return false
}
