// Package email is a minimal facade for transactional email sending. Adapters
// (SendGrid, notification-service) implement Client; dunning + trial crons
// depend on the Client interface so real email wiring can be swapped in
// without touching caller code.
package email

import "context"

// TemplateID is a stable identifier for a transactional template.
type TemplateID string

const (
	TemplateDunningDay5           TemplateID = "dunning_day_5"
	TemplateDunningDay7           TemplateID = "dunning_day_7"
	TemplatePaymentActionReminder TemplateID = "payment_action_reminder"
)

// Client is the narrow interface every caller uses.
type Client interface {
	Send(ctx context.Context, template TemplateID, to string, data map[string]any) error
}
