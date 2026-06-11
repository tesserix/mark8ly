// Package orderdoc dispatches lifecycle emails for an order.
//
// Four distinct hooks:
//   - Invoice       — fired when an order is confirmed (accepted)
//   - Receipt       — fired when a shipment transitions to delivered
//   - Cancellation  — fired when an order is cancelled (admin or customer)
//   - Refund        — fired when a refund is recorded against an order
//
// Emails contain a deep link back to the customer's account order page,
// where they can download the actual PDF (rendered by the storefront's
// /api/orders/:id/{invoice,receipt} routes). Embedding the PDF in the
// email is intentionally NOT done here — it would require duplicating
// the React-PDF generator in Go, which would drift from the canonical
// document and double the maintenance burden. Single source of truth
// wins.
package orderdoc

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// Kind discriminates the lifecycle email this package is dispatching.
type Kind string

const (
	KindInvoice            Kind = "invoice"
	KindReceipt            Kind = "receipt"
	KindCancellation       Kind = "cancellation"
	KindRefund             Kind = "refund"
	KindShipmentDispatched Kind = "shipment_dispatched"
)

// Mailer is the contract between the order lifecycle and the underlying
// email transport. One method per intent so handler code reads cleanly.
type Mailer interface {
	SendInvoice(ctx context.Context, in DocumentInput) error
	SendReceipt(ctx context.Context, in DocumentInput) error
	SendCancellation(ctx context.Context, in DocumentInput) error
	SendRefund(ctx context.Context, in DocumentInput) error
	// SendShipmentDispatched fires when a shipment transitions to
	// in_transit (admin marks it shipped, OR carrier webhook reports
	// pickup). Customer-facing — gives them tracking visibility before
	// the package arrives. Wave 1.5 attribution flows via custom_args
	// kind=shipment_dispatched.
	SendShipmentDispatched(ctx context.Context, in DocumentInput) error
}

// DocumentInput is everything the email template needs to render. The
// Service builder fills it from the order + branding records before
// handing off to the mailer.
type DocumentInput struct {
	// Recipient — required. Validated by the mailer; empty addresses
	// silently no-op so a missing customer email never crashes a confirm.
	Recipient string

	// TenantID, when set, is forwarded to the email provider as a custom_arg
	// so notification-service can attribute open/click/bounce events back
	// to the right tenant in tesserix-home dashboards. The orderdoc
	// service populates it from the loaded order's tenant.
	TenantID string

	// Document identity — formatted "INV-PLA-260417-01154" / "RCP-...".
	// Empty for cancellation + refund (those reference the order_number
	// directly; no separate document number issued).
	DocumentNumber string

	// Order context — used in subject line + email body.
	OrderID     string // uuid string — needed for the storefront PDF fetch
	StoreSlug   string // also for the PDF fetch (URL templating)
	OrderNumber string
	OrderURL    string // deep link to /account/orders/:id on the storefront
	// DocumentURL is the direct PDF download URL on the storefront
	// (/api/orders/:id/invoice or /api/orders/:id/receipt). Used as the
	// CTA on invoice + receipt emails so a single click downloads the
	// file instead of routing through the account page. Also kept
	// around as a fallback when PDF attach fails.
	DocumentURL string
	PlacedAt    time.Time
	DeliveredAt *time.Time // populated for receipts only

	// Money summary
	GrandTotal   decimal.Decimal
	CurrencyCode string
	ItemCount    int

	// Cancellation-specific
	CancellationReason  string // free text from the cancel form, may be empty
	CancelledByCustomer bool   // true when self-service cancel; affects copy

	// Refund-specific
	RefundAmount  decimal.Decimal // amount of THIS refund
	TotalRefunded decimal.Decimal // running total after this refund
	IsFullRefund  bool            // refunded == grand_total

	// Shipment-dispatched-specific. Carrier + TrackingNumber are
	// usually present (admin sets them when generating the label);
	// EstimatedDelivery is best-effort (carriers don't always provide
	// it on the in_transit transition).
	Carrier           string
	TrackingNumber    string
	EstimatedDelivery string // human-readable, e.g. "Apr 18, 2026"

	// AdminNote, when set, renders a "Note from {store}" block in the
	// email body. Used by the admin "Email to customer" resend flow so
	// the merchant can attach a short personal message ("Thanks for the
	// quick reorder", "We've waived the shipping fee" etc.) without
	// editing the canonical template. Whitespace-trimmed by the caller
	// — empty string suppresses the block entirely.
	AdminNote string

	// Brand surface
	Theme Theme
}

// Theme is the subset of store branding rendered in the email chrome.
// Plain struct so this package doesn't depend on the branding package.
type Theme struct {
	StoreName       string
	LogoURL         string
	Tagline         string
	ColorBackground string
	ColorText       string
	ColorAccent     string
	ColorButtonBg   string
	ColorButtonText string
	HeadingFont     string
	BodyFont        string
	FooterTagline   string
	FooterCopyright string
}

// Editorial defaults — same palette as the giftcard mailer so unbranded
// stores still feel like the platform. Contrast bumped so the
// supporting / muted / faint copy stays readable on the white email
// canvas — the previous values (#7A766E and #A09C92) reduced the body
// text to a near-invisible grey on Gmail's whitewashed render.
const (
	emailHairline      = "#D7D4CB"
	emailSupporting    = "#1F1E1B"
	emailMuted         = "#3C3A35"
	emailFaint         = "#55524C"
	defaultBackground  = "#F7F6F2"
	defaultText        = "#0E0E0C"
	defaultAccent      = "#2D4A2B"
	defaultButtonBg    = "#0E0E0C"
	defaultButtonText  = "#FDFCF8"
	defaultHeadingFont = "Georgia, 'Source Serif 4', 'Times New Roman', serif"
	defaultBodyFont    = "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Source Sans 3', Helvetica, Arial, sans-serif"
)

func (t Theme) withDefaults() Theme {
	if t.StoreName == "" {
		t.StoreName = "Mark8ly"
	}
	if t.ColorBackground == "" {
		t.ColorBackground = defaultBackground
	}
	if t.ColorText == "" {
		t.ColorText = defaultText
	}
	if t.ColorAccent == "" {
		t.ColorAccent = defaultAccent
	}
	if t.ColorButtonBg == "" {
		t.ColorButtonBg = defaultButtonBg
	}
	if t.ColorButtonText == "" {
		t.ColorButtonText = defaultButtonText
	}
	if t.HeadingFont == "" {
		t.HeadingFont = defaultHeadingFont
	}
	if t.BodyFont == "" {
		t.BodyFont = defaultBodyFont
	}
	return t
}

// -- Templating --------------------------------------------------------

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

type renderData struct {
	// Subject is removed — it's now rendered by the loader from a
	// separate subject-template column. Heading / Lede / CTAButtonLabel
	// stay here because they're computed in Go (business-state-driven
	// copy that depends on Kind + IsFullRefund + CancelledByCustomer).
	Heading        string
	Lede           string
	CTAButtonLabel string
	DocumentNumber string
	OrderNumber    string
	// OrderURL points at the storefront's /account/orders/:id page —
	// used by Cancellation/Refund emails. Invoice + Receipt emails
	// override the CTA with DocumentURL (a direct PDF link) so a single
	// click downloads the file instead of dropping the buyer on the
	// account page.
	OrderURL     string
	DocumentURL  string
	PlacedAt     string
	DeliveredAt  string
	GrandTotal   string
	CurrencyCode string
	ItemCount    int

	// HasAttachment is true when the rendered PDF was successfully
	// fetched from the storefront and will be attached to the outgoing
	// email. The invoice + receipt templates branch on this so the body
	// copy never lies — when false they switch from "PDF attached" to
	// a "Download from your account" CTA.
	HasAttachment bool

	// AdminNote is rendered as a "Note from {store}" block when the
	// admin "Email to customer" resend flow supplies a non-empty
	// message. Empty string suppresses the block.
	AdminNote string

	// Cancellation rendering
	CancellationReason string

	// Refund rendering
	RefundAmount  string
	TotalRefunded string
	IsFullRefund  bool

	// Shipment-dispatched rendering — passthrough of DocumentInput's
	// shipment fields so the template can branch on TrackingNumber +
	// Carrier presence and surface them prominently.
	Carrier           string
	TrackingNumber    string
	EstimatedDelivery string

	Theme          Theme
	HairlineColor  string
	SupportingCopy string
	MutedCopy      string
	FaintCopy      string
}

// Embedded template strings — kept as fallback for the loader. We
// register these at startup; loader prefers the DB row when present.
var (
	invoiceEmbeddedHTML            = mustReadEmbedded("templates/invoice_email.html")
	invoiceEmbeddedText            = mustReadEmbedded("templates/invoice_email.txt")
	receiptEmbeddedHTML            = mustReadEmbedded("templates/receipt_email.html")
	receiptEmbeddedText            = mustReadEmbedded("templates/receipt_email.txt")
	cancellationEmbeddedHTML       = mustReadEmbedded("templates/cancellation_email.html")
	cancellationEmbeddedText       = mustReadEmbedded("templates/cancellation_email.txt")
	refundEmbeddedHTML             = mustReadEmbedded("templates/refund_email.html")
	refundEmbeddedText             = mustReadEmbedded("templates/refund_email.txt")
	shipmentDispatchedEmbeddedHTML = mustReadEmbedded("templates/shipment_dispatched_email.html")
	shipmentDispatchedEmbeddedText = mustReadEmbedded("templates/shipment_dispatched_email.txt")
)

func mustReadEmbedded(path string) string {
	raw, err := templateFS.ReadFile(path)
	if err != nil {
		// embed.FS guarantees these files at compile time; missing at
		// runtime indicates a programming error, not user error.
		panic(fmt.Sprintf("orderdoc: embedded template %s missing: %v", path, err))
	}
	return string(raw)
}

// Subject templates — Go-template strings rendered against renderData.
// Operator can override per key in tesserix-home; embedded versions
// here are the fallback.
const (
	invoiceSubjectTpl            = "Invoice for order {{.OrderNumber}} — confirmed"
	receiptSubjectTpl            = "Receipt for order {{.OrderNumber}} — delivered"
	cancellationSubjectTpl       = "Order {{.OrderNumber}} has been cancelled"
	refundSubjectTpl             = "{{if .IsFullRefund}}Refund issued for order {{.OrderNumber}} — fully refunded{{else}}Partial refund issued for order {{.OrderNumber}}{{end}}"
	shipmentDispatchedSubjectTpl = "Your order {{.OrderNumber}} has shipped"
)

// templateKey converts a Kind into the loader's registry key. Public
// so tests can refer to the same constants.
func templateKey(kind Kind) string {
	return "orderdoc_" + string(kind) + "_email"
}

// RegisterFallbacks registers every orderdoc template's embedded
// fallback against the shared loader. Called once at boot.
func RegisterFallbacks(loader *emailtemplates.Loader) {
	loader.Register(templateKey(KindInvoice), emailtemplates.EmbeddedFallback{
		Subject:  invoiceSubjectTpl,
		HTMLBody: invoiceEmbeddedHTML,
		TextBody: invoiceEmbeddedText,
	})
	loader.Register(templateKey(KindReceipt), emailtemplates.EmbeddedFallback{
		Subject:  receiptSubjectTpl,
		HTMLBody: receiptEmbeddedHTML,
		TextBody: receiptEmbeddedText,
	})
	loader.Register(templateKey(KindCancellation), emailtemplates.EmbeddedFallback{
		Subject:  cancellationSubjectTpl,
		HTMLBody: cancellationEmbeddedHTML,
		TextBody: cancellationEmbeddedText,
	})
	loader.Register(templateKey(KindRefund), emailtemplates.EmbeddedFallback{
		Subject:  refundSubjectTpl,
		HTMLBody: refundEmbeddedHTML,
		TextBody: refundEmbeddedText,
	})
	loader.Register(templateKey(KindShipmentDispatched), emailtemplates.EmbeddedFallback{
		Subject:  shipmentDispatchedSubjectTpl,
		HTMLBody: shipmentDispatchedEmbeddedHTML,
		TextBody: shipmentDispatchedEmbeddedText,
	})
}

// buildRenderData computes the per-kind renderData. Subject is no
// longer computed here — it's a template string in DB / embedded
// fallback, rendered by the loader. Heading / Lede / CTAButtonLabel
// remain computed because they're business-state-driven.
func buildRenderData(kind Kind, in DocumentInput, hasAttachment bool) renderData {
	theme := in.Theme.withDefaults()

	var heading, lede, cta string
	switch kind {
	case KindReceipt:
		heading = "Your order has been delivered."
		lede = fmt.Sprintf("This is your receipt for order %s. Keep it for your records — and thank you for shopping with %s.", in.OrderNumber, theme.StoreName)
		cta = "View receipt"
	case KindCancellation:
		heading = "Your order has been cancelled."
		if in.CancelledByCustomer {
			lede = fmt.Sprintf("Order %s has been cancelled at your request. If a payment was captured, the refund will follow shortly.", in.OrderNumber)
		} else {
			lede = fmt.Sprintf("Order %s has been cancelled by %s. If a payment was captured, the refund will follow shortly.", in.OrderNumber, theme.StoreName)
		}
		cta = "View order"
	case KindRefund:
		if in.IsFullRefund {
			heading = "Your refund has been issued in full."
		} else {
			heading = "A partial refund has been issued."
		}
		lede = fmt.Sprintf("We've issued a refund of %s %s against order %s. It typically takes 3–10 business days to appear on your statement, depending on your bank.", in.RefundAmount.StringFixed(2), in.CurrencyCode, in.OrderNumber)
		cta = "View order"
	case KindShipmentDispatched:
		heading = "Your order is on the way."
		if in.TrackingNumber != "" {
			lede = fmt.Sprintf("Order %s has shipped from %s. Use the tracking number below to follow its journey.", in.OrderNumber, theme.StoreName)
		} else {
			lede = fmt.Sprintf("Order %s has shipped from %s. Tracking details will follow once the carrier provides them.", in.OrderNumber, theme.StoreName)
		}
		cta = "Track order"
	default: // KindInvoice
		heading = "Your order is confirmed."
		lede = fmt.Sprintf("This is your invoice for order %s. We're getting it ready and will let you know when it ships.", in.OrderNumber)
		cta = "View invoice"
	}

	deliveredAt := ""
	if in.DeliveredAt != nil {
		deliveredAt = in.DeliveredAt.Format("January 2, 2006")
	}

	return renderData{
		Heading:            heading,
		Lede:               lede,
		CTAButtonLabel:     cta,
		DocumentNumber:     in.DocumentNumber,
		OrderNumber:        in.OrderNumber,
		OrderURL:           in.OrderURL,
		DocumentURL:        in.DocumentURL,
		PlacedAt:           in.PlacedAt.Format("January 2, 2006"),
		DeliveredAt:        deliveredAt,
		GrandTotal:         in.GrandTotal.StringFixed(2),
		CurrencyCode:       in.CurrencyCode,
		ItemCount:          in.ItemCount,
		HasAttachment:      hasAttachment,
		AdminNote:          in.AdminNote,
		CancellationReason: in.CancellationReason,
		RefundAmount:       in.RefundAmount.StringFixed(2),
		TotalRefunded:      in.TotalRefunded.StringFixed(2),
		IsFullRefund:       in.IsFullRefund,
		Carrier:            in.Carrier,
		TrackingNumber:     in.TrackingNumber,
		EstimatedDelivery:  in.EstimatedDelivery,
		Theme:              theme,
		HairlineColor:      emailHairline,
		SupportingCopy:     emailSupporting,
		MutedCopy:          emailMuted,
		FaintCopy:          emailFaint,
	}
}

// render is the loader-backed renderer. Returns the same triple
// (subject, html, text) the outgoing envelope expects. When loader is
// nil (e.g. unit tests that don't want a DB) it uses the embedded
// template strings directly.
func render(ctx context.Context, loader *emailtemplates.Loader, kind Kind, in DocumentInput, hasAttachment bool) (subject, html, text string, err error) {
	data := buildRenderData(kind, in, hasAttachment)

	if loader == nil {
		// Test path with no loader — render the embedded fallback
		// inline by registering against a temporary loader.
		loader = emailtemplates.NewLoader(nil)
		RegisterFallbacks(loader)
	}

	r, err := loader.Render(ctx, templateKey(kind), data)
	if err != nil {
		return "", "", "", fmt.Errorf("orderdoc: %w", err)
	}
	return r.Subject, r.HTMLBody, r.TextBody, nil
}

// -- DocumentMailer ----------------------------------------------------

// DocumentMailer sends order document emails through the shared
// internal/email transport (SendGrid primary, Resend fallback; log-only
// in dev). Mirrors the giftcard mailer pattern.
type DocumentMailer struct {
	sender     email.Sender
	from       string
	logger     *slog.Logger
	pdfFetcher StorefrontPDFFetcher
	loader     *emailtemplates.Loader
}

// NewDocumentMailer constructs a Mailer on top of the shared transport.
func NewDocumentMailer(sender email.Sender, from string, logger *slog.Logger) *DocumentMailer {
	return &DocumentMailer{
		sender: sender,
		from:   from,
		logger: logger,
	}
}

// WithLoader wires the shared template loader. When set, render() uses
// the DB-backed templates with embedded fallback. When nil, render()
// uses embedded directly (test ergonomics).
func (m *DocumentMailer) WithLoader(l *emailtemplates.Loader) *DocumentMailer {
	m.loader = l
	return m
}

// WithStorefrontPDFFetcher wires a fetcher that retrieves the rendered
// PDF (invoice / receipt) from the storefront over the internal-auth
// channel. When set, SendInvoice and SendReceipt attach the PDF to the
// outgoing email — the canonical document arrives directly in the
// buyer's inbox instead of asking them to click through to download it.
func (m *DocumentMailer) WithStorefrontPDFFetcher(f StorefrontPDFFetcher) *DocumentMailer {
	m.pdfFetcher = f
	return m
}

// StorefrontPDFFetcher retrieves a rendered invoice or receipt PDF
// from the storefront for the given order. Implementations call the
// storefront's /api/internal/orders/:id/{invoice,receipt} routes with
// the shared MARKETPLACE_INTERNAL_AUTH_SECRET so they can render PDFs
// without a customer session cookie.
type StorefrontPDFFetcher interface {
	FetchInvoicePDF(ctx context.Context, in DocumentInput) ([]byte, error)
	FetchReceiptPDF(ctx context.Context, in DocumentInput) ([]byte, error)
}

// SendInvoice renders + dispatches the invoice envelope.
func (m *DocumentMailer) SendInvoice(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindInvoice, in)
}

// SendReceipt renders + dispatches the receipt envelope.
func (m *DocumentMailer) SendReceipt(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindReceipt, in)
}

// SendCancellation renders + dispatches the cancellation envelope.
func (m *DocumentMailer) SendCancellation(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindCancellation, in)
}

// SendRefund renders + dispatches the refund-issued envelope.
func (m *DocumentMailer) SendRefund(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindRefund, in)
}

// SendShipmentDispatched renders + dispatches the customer-facing
// shipment-dispatched envelope (no PDF attachment, just tracking).
func (m *DocumentMailer) SendShipmentDispatched(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindShipmentDispatched, in)
}

func (m *DocumentMailer) send(ctx context.Context, kind Kind, in DocumentInput) error {
	if in.Recipient == "" {
		return fmt.Errorf("orderdoc: missing recipient")
	}

	// Best-effort PDF attachment — when the fetcher is wired and we're
	// sending an invoice or receipt, fetch the rendered PDF from the
	// storefront and attach. Failures here are logged but don't block
	// the email — the buyer still receives the HTML body and can view
	// the document on their account page if a re-fetch is needed.
	// We resolve the attachment FIRST so the template can branch its
	// "PDF attached" copy on the actual outcome instead of optimistic
	// language that turns into a small lie when the fetch flakes.
	var attachments []email.Attachment
	hasAttachment := false
	if (kind == KindInvoice || kind == KindReceipt) && m.pdfFetcher != nil {
		var pdfBytes []byte
		var pdfErr error
		if kind == KindInvoice {
			pdfBytes, pdfErr = m.pdfFetcher.FetchInvoicePDF(ctx, in)
		} else {
			pdfBytes, pdfErr = m.pdfFetcher.FetchReceiptPDF(ctx, in)
		}
		if pdfErr != nil {
			if m.logger != nil {
				m.logger.Warn("orderdoc: pdf fetch failed; sending email without attachment",
					"kind", string(kind),
					"order_number", in.OrderNumber,
					"err", pdfErr)
			}
		} else if len(pdfBytes) > 0 {
			filename := in.DocumentNumber + ".pdf"
			if in.DocumentNumber == "" {
				filename = "document-" + in.OrderNumber + ".pdf"
			}
			attachments = []email.Attachment{{
				Filename:    filename,
				ContentType: "application/pdf",
				Content:     pdfBytes,
			}}
			hasAttachment = true
		}
	}

	subject, htmlBody, textBody, err := render(ctx, m.loader, kind, in, hasAttachment)
	if err != nil {
		return err
	}

	customArgs := map[string]string{"product": "mark8ly", "kind": string(kind)}
	if in.TenantID != "" {
		customArgs["tenant_id"] = in.TenantID
	}

	if err := m.sender.Send(ctx, email.Message{
		From:        m.from,
		To:          in.Recipient,
		Subject:     subject,
		HTMLBody:    htmlBody,
		TextBody:    textBody,
		CustomArgs:  customArgs,
		Attachments: attachments,
	}); err != nil {
		return fmt.Errorf("orderdoc: send %s email: %w", string(kind), err)
	}
	return nil
}
