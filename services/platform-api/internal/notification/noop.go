package notification

import "context"

// NoopSender silently discards every Email. Used in unit tests where the
// actual send is not under test.
//
// It still calls validate() so tests catch malformed Email structs.
type NoopSender struct{}

// Send returns nil after validating the Email structure.
func (NoopSender) Send(ctx context.Context, msg Email) error {
	return validate(msg)
}
