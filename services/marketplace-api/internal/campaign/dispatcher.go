package campaign

import (
	"context"

	"github.com/mark8ly/marketplace-api/internal/email"
)

// Dispatcher delivers fully-rendered campaign emails. The send worker
// loads the store branding once per campaign dispatch and renders the
// editorial envelope before calling Send — the dispatcher itself is a
// thin transport adapter and does no theming work.
//
// The production implementation is EmailDispatcher, which rides the
// shared internal/email transport (SendGrid primary, Resend fallback;
// log-only when no provider keys are configured — main decides once via
// email.NewFromConfig).
type Dispatcher interface {
	Send(ctx context.Context, msg OutboundEmail) error
}

// OutboundEmail is one rendered campaign email ready to hand to a
// provider. The html and text bodies have already been wrapped in the
// store's branded envelope by the send worker.
type OutboundEmail struct {
	Recipient string
	Subject   string
	HTMLBody  string
	TextBody  string
	// TenantID, when non-empty, is forwarded to the provider as a custom_arg
	// for per-tenant attribution in tesserix-home dashboards. The send
	// worker fills it from the campaign's tenant_id column.
	TenantID string
	// CampaignID, when non-empty, joins provider engagement events to the
	// originating campaign so future per-campaign metrics can be derived
	// without re-keying recipient strings against the database.
	CampaignID string
	// Sender is the store's customer-facing identity (#718). A campaign
	// is marketing mail FROM the merchant's store, so it wears the store
	// name and the store's Reply-To. The send worker fills it from the
	// campaign theme it already loads; a zero value degrades to the
	// platform identity.
	Sender email.StoreSender
}
