// Package providerlog provides safe logging helpers for third-party
// provider responses. Never logs raw response bodies which may contain
// PII or payment card data.
package providerlog

import "fmt"

// SanitizeProviderError returns a log-safe string for a failed provider
// HTTP call. The response body is never included — only the provider name,
// status code, and body length.
func SanitizeProviderError(provider string, statusCode int, body []byte) string {
	return fmt.Sprintf("%s: HTTP %d (response body redacted, %d bytes)", provider, statusCode, len(body))
}
