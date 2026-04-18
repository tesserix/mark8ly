package email

import (
	"context"
	"log/slog"
)

// NoOpClient logs the intended send and returns nil. The production
// SendGrid/notification-service adapter lands later; until then this keeps
// crons unblocked.
type NoOpClient struct {
	Logger *slog.Logger
}

// Send logs the would-be email and returns nil without performing any I/O.
func (c NoOpClient) Send(_ context.Context, template TemplateID, to string, data map[string]any) error {
	if c.Logger == nil {
		return nil
	}
	c.Logger.Info("email: would send (no-op adapter)",
		"template", string(template), "to", to, "data", data)
	return nil
}
