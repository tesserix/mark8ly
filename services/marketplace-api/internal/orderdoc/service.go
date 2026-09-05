package orderdoc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/storeidentity"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service composes the mailer with the data lookups required to fill a
// DocumentInput from an order_id alone. Handlers call SendInvoice or
// SendReceipt and the service handles loading the order, branding,
// store and the deep link.
type Service struct {
	db                *gorm.DB
	mailer            Mailer
	orderRepo         order.Repository
	brandingSvc       *branding.Service
	storefrontURLBase string // template with {slug}
	// identity resolves the store's public sender identity (#718). It
	// replaces the bare `stores` lookup this service used to do — same
	// one query, plus the branding contact address the envelope needs.
	identity storeidentity.Loader
	logger   *slog.Logger
}

// NewService constructs a Service. Pass empty storefrontURLBase to
// disable the CTA link in emails (button still renders fallback).
func NewService(db *gorm.DB, mailer Mailer, orderRepo order.Repository, brandingSvc *branding.Service, storefrontURLBase string) *Service {
	return &Service{
		db:                db,
		mailer:            mailer,
		orderRepo:         orderRepo,
		brandingSvc:       brandingSvc,
		storefrontURLBase: storefrontURLBase,
		identity:          storeidentity.NewDBLoader(db),
	}
}

// WithLogger attaches a structured logger so the service can emit
// warnings when it skips a dispatch (missing customer email, missing
// store) instead of swallowing the failure into a generic error
// returned to the goroutine. Chainable.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	s.logger = l
	return s
}

// errMissingCustomerEmail is returned when an order has no recipient
// to send to. Wrapped (errors.Is) so callers can branch on it for
// user-facing messaging — admin "Email to customer" surfaces it as a
// 422 instead of the generic 502.
var errMissingCustomerEmail = errors.New("orderdoc: order has no customer email")

// IsMissingCustomerEmail reports whether the error chain contains a
// "no customer email" sentinel. Used by the admin email handler to
// produce an actionable error message ("This order has no customer
// email — add one and try again") instead of a generic gateway error.
func IsMissingCustomerEmail(err error) bool {
	return errors.Is(err, errMissingCustomerEmail)
}

// SendOptions carries optional tweaks to a manual invoice/receipt
// dispatch. AdminNote, when non-empty, renders a "Note from {store}"
// block in the email body so the admin can add a short personal
// message on resend without editing the canonical template.
type SendOptions struct {
	AdminNote string
}

// SendInvoice loads the order context and dispatches the invoice
// envelope. Safe to call from a fire-and-forget goroutine — failures
// are logged via the underlying mailer but do not return upstream.
func (s *Service) SendInvoice(ctx context.Context, orderID uuid.UUID) error {
	return s.SendInvoiceWithOptions(ctx, orderID, SendOptions{})
}

// SendInvoiceWithOptions is the explicit-options variant used by the
// admin "Email to customer" flow when the merchant attaches a personal
// note. Behaviour is identical to SendInvoice for an empty SendOptions.
func (s *Service) SendInvoiceWithOptions(ctx context.Context, orderID uuid.UUID, opts SendOptions) error {
	in, err := s.buildInput(ctx, orderID, false)
	if err != nil {
		return err
	}
	in.AdminNote = strings.TrimSpace(opts.AdminNote)
	return s.mailer.SendInvoice(ctx, in)
}

// SendReceipt loads the order context and dispatches the receipt
// envelope. Caller is responsible for ensuring the order has actually
// been delivered before invoking — the mailer doesn't re-validate.
func (s *Service) SendReceipt(ctx context.Context, orderID uuid.UUID) error {
	return s.SendReceiptWithOptions(ctx, orderID, SendOptions{})
}

// SendReceiptWithOptions is the explicit-options variant used by the
// admin "Email to customer" flow when the merchant attaches a personal
// note.
func (s *Service) SendReceiptWithOptions(ctx context.Context, orderID uuid.UUID, opts SendOptions) error {
	in, err := s.buildInput(ctx, orderID, true)
	if err != nil {
		return err
	}
	in.AdminNote = strings.TrimSpace(opts.AdminNote)
	return s.mailer.SendReceipt(ctx, in)
}

// SendCancellation loads the order context and dispatches the cancel
// envelope. `byCustomer` switches the subject + lede copy between
// "cancelled by you" and "cancelled by the merchant" tones.
func (s *Service) SendCancellation(ctx context.Context, orderID uuid.UUID, reason string, byCustomer bool) error {
	in, err := s.buildInput(ctx, orderID, false)
	if err != nil {
		return err
	}
	// Cancellation doesn't have its own document number — strip the
	// invoice-style INV- prefix that buildInput puts there.
	in.DocumentNumber = ""
	in.CancellationReason = strings.TrimSpace(reason)
	in.CancelledByCustomer = byCustomer
	return s.mailer.SendCancellation(ctx, in)
}

// SendRefund dispatches a "refund issued" email. The caller passes the
// amount of THIS refund and the running total after it (which the
// service.RecordRefund returns); we pull the rest from the order row.
func (s *Service) SendRefund(ctx context.Context, orderID uuid.UUID, refundAmount, totalRefundedAfter decimal.Decimal) error {
	in, err := s.buildInput(ctx, orderID, false)
	if err != nil {
		return err
	}
	in.DocumentNumber = ""
	in.RefundAmount = refundAmount
	in.TotalRefunded = totalRefundedAfter
	in.IsFullRefund = totalRefundedAfter.GreaterThanOrEqual(in.GrandTotal)
	return s.mailer.SendRefund(ctx, in)
}

// ShipmentInfo carries the per-shipment fields the dispatched email
// needs. Caller (admin shipments handler / carrier webhook) supplies
// it because the order row alone doesn't have carrier + tracking; the
// shipment row does.
type ShipmentInfo struct {
	Carrier           string
	TrackingNumber    string
	EstimatedDelivery string // human-readable, optional
}

// SendShipmentDispatched dispatches the customer-facing "your order
// has shipped" email. Caller passes shipment-specific fields the
// order context can't provide.
func (s *Service) SendShipmentDispatched(ctx context.Context, orderID uuid.UUID, info ShipmentInfo) error {
	in, err := s.buildInput(ctx, orderID, false)
	if err != nil {
		return err
	}
	in.DocumentNumber = ""
	in.Carrier = info.Carrier
	in.TrackingNumber = info.TrackingNumber
	in.EstimatedDelivery = info.EstimatedDelivery
	return s.mailer.SendShipmentDispatched(ctx, in)
}

func (s *Service) buildInput(ctx context.Context, orderID uuid.UUID, asReceipt bool) (DocumentInput, error) {
	o, items, _, err := s.orderRepo.GetByID(ctx, s.db, orderID)
	if err != nil {
		return DocumentInput{}, fmt.Errorf("orderdoc: load order: %w", err)
	}
	if o == nil {
		return DocumentInput{}, fmt.Errorf("orderdoc: order %s not found", orderID)
	}
	if o.CustomerEmail == "" {
		if s.logger != nil {
			s.logger.Warn("orderdoc: skipping dispatch — order has no customer email",
				"order_id", orderID,
				"order_number", o.OrderNumber)
		}
		return DocumentInput{}, fmt.Errorf("%w (order_id=%s)", errMissingCustomerEmail, orderID)
	}

	store, err := s.identity.Load(ctx, o.StoreID)
	if err != nil {
		return DocumentInput{}, fmt.Errorf("orderdoc: load store: %w", err)
	}
	// The loader returns a zero Store for a missing row rather than an
	// error, because its other callers are best-effort. Here the store is
	// load-bearing — it supplies the tenant id every downstream document
	// is scoped by — so the hard failure the previous First() produced is
	// kept. tenant_id is NOT NULL, so an empty one means no row.
	if store.TenantID == "" {
		return DocumentInput{}, fmt.Errorf("orderdoc: store %s not found", o.StoreID)
	}

	theme := Theme{StoreName: store.Name}
	b, berr := s.brandingSvc.GetByStoreID(ctx, o.StoreID)
	if berr != nil && !errors.Is(berr, apperrors.ErrNotFound) {
		return DocumentInput{}, fmt.Errorf("orderdoc: load branding: %w", berr)
	}
	if b != nil {
		theme.ColorBackground = b.ColorBackground
		theme.ColorText = b.ColorText
		theme.ColorAccent = b.ColorAccent
		theme.ColorButtonBg = b.ColorButtonBg
		theme.ColorButtonText = b.ColorButtonText
		theme.HeadingFont = fontFamilyFor(b.HeadingFont, true)
		theme.BodyFont = fontFamilyFor(b.BodyFont, false)
		if b.LogoURL != nil {
			theme.LogoURL = *b.LogoURL
		}
		if b.Tagline != nil {
			theme.Tagline = *b.Tagline
		}
		if b.FooterTagline != nil {
			theme.FooterTagline = *b.FooterTagline
		}
		if b.FooterCopyright != nil {
			theme.FooterCopyright = *b.FooterCopyright
		}
	}

	// Deep link to the customer's account order page on the storefront.
	// Falls back to empty so the email template can suppress the CTA.
	orderURL := ""
	documentURL := ""
	if store.Slug != "" && s.storefrontURLBase != "" {
		base := strings.TrimRight(strings.ReplaceAll(s.storefrontURLBase, "{slug}", store.Slug), "/")
		orderURL = base + "/account/orders/" + orderID.String()
		// Direct PDF download endpoints on the storefront (rendered by
		// React-PDF in apps/storefront/lib/invoices). Used as the CTA
		// on invoice + receipt emails for a one-click download instead
		// of dropping the buyer on the account page.
		if asReceipt {
			documentURL = base + "/api/orders/" + orderID.String() + "/receipt"
		} else {
			documentURL = base + "/api/orders/" + orderID.String() + "/invoice"
		}
	}

	in := DocumentInput{
		Recipient:      o.CustomerEmail,
		TenantID:       store.TenantID,
		DocumentNumber: documentNumber(asReceipt, o.OrderNumber),
		OrderID:        orderID.String(),
		StoreSlug:      store.Slug,
		// Feeds the customer-facing Reply-To (#718); empty is fine and
		// falls back to the platform address.
		StoreContactEmail: store.ContactEmail,
		OrderNumber:       o.OrderNumber,
		OrderURL:          orderURL,
		DocumentURL:       documentURL,
		PlacedAt:          o.PlacedAt,
		GrandTotal:        o.GrandTotal,
		CurrencyCode:      o.CurrencyCode,
		ItemCount:         len(items),
		Theme:             theme,
	}
	if asReceipt {
		in.DeliveredAt = s.lookupDeliveredAt(ctx, orderID, o.UpdatedAt)
	}
	return in, nil
}

// lookupDeliveredAt returns the real shipment.delivered_at for the
// order's most recently delivered shipment. Falls back to the order's
// updated_at when no delivered shipment row exists (legacy orders, or
// orders fulfilled without a shipment record). The receipt PDF on the
// storefront already prefers shipment.delivered_at — this brings the
// email body in line so the date that lands in the customer's inbox
// matches the date stamped on the attached PDF.
func (s *Service) lookupDeliveredAt(ctx context.Context, orderID uuid.UUID, fallback time.Time) *time.Time {
	var row struct {
		DeliveredAt *time.Time `gorm:"column:delivered_at"`
	}
	err := s.db.WithContext(ctx).
		Table("shipments").
		Select("delivered_at").
		Where("order_id = ? AND status = ? AND delivered_at IS NOT NULL", orderID, "delivered").
		Order("delivered_at DESC").
		Limit(1).
		Take(&row).Error
	if err == nil && row.DeliveredAt != nil {
		return row.DeliveredAt
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && s.logger != nil {
		s.logger.Warn("orderdoc: shipment delivered_at lookup failed; falling back to order.updated_at",
			"order_id", orderID, "err", err)
	}
	t := fallback
	return &t
}

// documentNumber derives the deterministic invoice/receipt number from
// the order_number. Mirrors apps/admin/lib/invoices/numbering.ts so the
// email and the PDF show identical IDs for the same order.
//
//	M-PLA-260417-01154 → INV-PLA-260417-01154 / RCP-PLA-260417-01154
func documentNumber(receipt bool, orderNumber string) string {
	prefix := "INV-"
	if receipt {
		prefix = "RCP-"
	}
	if strings.HasPrefix(orderNumber, "M-") && len(orderNumber) > 2 {
		return prefix + orderNumber[2:]
	}
	return prefix + orderNumber
}

// fontFamilyFor maps merchant font keys to email-safe CSS stacks. Same
// table the giftcard package uses; duplicated to avoid a cross-package
// import that would couple two unrelated email pipelines.
func fontFamilyFor(key string, heading bool) string {
	const (
		serifFallback = "'Source Serif 4', 'Times New Roman', serif"
		sansFallback  = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif"
	)
	switch key {
	case "source-serif-4":
		return "'Source Serif 4', Georgia, 'Times New Roman', serif"
	case "playfair-display":
		return "'Playfair Display', Georgia, 'Times New Roman', serif"
	case "lora":
		return "'Lora', Georgia, 'Times New Roman', serif"
	case "inter":
		return "'Inter', " + sansFallback
	case "source-sans-3":
		return "'Source Sans 3', " + sansFallback
	case "dm-sans":
		return "'DM Sans', " + sansFallback
	}
	if heading {
		return "Georgia, " + serifFallback
	}
	return sansFallback
}
