// Package orderdoc dispatches lifecycle emails for an order.
//
// Four distinct hooks:
//   • Invoice       — fired when an order is confirmed (accepted)
//   • Receipt       — fired when a shipment transitions to delivered
//   • Cancellation  — fired when an order is cancelled (admin or customer)
//   • Refund        — fired when a refund is recorded against an order
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
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

// Kind discriminates the lifecycle email this package is dispatching.
type Kind string

const (
	KindInvoice      Kind = "invoice"
	KindReceipt      Kind = "receipt"
	KindCancellation Kind = "cancellation"
	KindRefund       Kind = "refund"
)

// Mailer is the contract between the order lifecycle and the underlying
// email transport. One method per intent so handler code reads cleanly.
type Mailer interface {
	SendInvoice(ctx context.Context, in DocumentInput) error
	SendReceipt(ctx context.Context, in DocumentInput) error
	SendCancellation(ctx context.Context, in DocumentInput) error
	SendRefund(ctx context.Context, in DocumentInput) error
}

// DocumentInput is everything the email template needs to render. The
// Service builder fills it from the order + branding records before
// handing off to the mailer.
type DocumentInput struct {
	// Recipient — required. Validated by the mailer; empty addresses
	// silently no-op so a missing customer email never crashes a confirm.
	Recipient string

	// Document identity — formatted "INV-PLA-260417-01154" / "RCP-...".
	// Empty for cancellation + refund (those reference the order_number
	// directly; no separate document number issued).
	DocumentNumber string

	// Order context — used in subject line + email body.
	OrderID      string // uuid string — needed for the storefront PDF fetch
	StoreSlug    string // also for the PDF fetch (URL templating)
	OrderNumber  string
	OrderURL     string // deep link to /account/orders/:id on the storefront
	// DocumentURL is the direct PDF download URL on the storefront
	// (/api/orders/:id/invoice or /api/orders/:id/receipt). Used as the
	// CTA on invoice + receipt emails so a single click downloads the
	// file instead of routing through the account page. Also kept
	// around as a fallback when PDF attach fails.
	DocumentURL  string
	PlacedAt     time.Time
	DeliveredAt  *time.Time // populated for receipts only

	// Money summary
	GrandTotal   decimal.Decimal
	CurrencyCode string
	ItemCount    int

	// Cancellation-specific
	CancellationReason string // free text from the cancel form, may be empty
	CancelledByCustomer bool  // true when self-service cancel; affects copy

	// Refund-specific
	RefundAmount    decimal.Decimal // amount of THIS refund
	TotalRefunded   decimal.Decimal // running total after this refund
	IsFullRefund    bool            // refunded == grand_total

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
	Subject         string
	Heading         string
	Lede            string
	CTAButtonLabel  string
	DocumentNumber  string
	OrderNumber     string
	// OrderURL points at the storefront's /account/orders/:id page —
	// used by Cancellation/Refund emails. Invoice + Receipt emails
	// override the CTA with DocumentURL (a direct PDF link) so a single
	// click downloads the file instead of dropping the buyer on the
	// account page.
	OrderURL        string
	DocumentURL     string
	PlacedAt        string
	DeliveredAt     string
	GrandTotal      string
	CurrencyCode    string
	ItemCount       int

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

	Theme           Theme
	HairlineColor   string
	SupportingCopy  string
	MutedCopy       string
	FaintCopy       string
}

var (
	invoiceHTMLTemplate = template.Must(
		template.ParseFS(templateFS, "templates/invoice_email.html"),
	)
	invoiceTextTemplate = template.Must(
		template.ParseFS(templateFS, "templates/invoice_email.txt"),
	)
	receiptHTMLTemplate = template.Must(
		template.ParseFS(templateFS, "templates/receipt_email.html"),
	)
	receiptTextTemplate = template.Must(
		template.ParseFS(templateFS, "templates/receipt_email.txt"),
	)
	cancellationHTMLTemplate = template.Must(
		template.ParseFS(templateFS, "templates/cancellation_email.html"),
	)
	cancellationTextTemplate = template.Must(
		template.ParseFS(templateFS, "templates/cancellation_email.txt"),
	)
	refundHTMLTemplate = template.Must(
		template.ParseFS(templateFS, "templates/refund_email.html"),
	)
	refundTextTemplate = template.Must(
		template.ParseFS(templateFS, "templates/refund_email.txt"),
	)
)

func render(kind Kind, in DocumentInput, hasAttachment bool) (subject, html, text string, err error) {
	theme := in.Theme.withDefaults()

	var heading, lede, cta string
	switch kind {
	case KindReceipt:
		subject = fmt.Sprintf("Receipt for order %s — delivered", in.OrderNumber)
		heading = "Your order has been delivered."
		lede = fmt.Sprintf("This is your receipt for order %s. Keep it for your records — and thank you for shopping with %s.", in.OrderNumber, theme.StoreName)
		cta = "View receipt"
	case KindCancellation:
		subject = fmt.Sprintf("Order %s has been cancelled", in.OrderNumber)
		heading = "Your order has been cancelled."
		if in.CancelledByCustomer {
			lede = fmt.Sprintf("Order %s has been cancelled at your request. If a payment was captured, the refund will follow shortly.", in.OrderNumber)
		} else {
			lede = fmt.Sprintf("Order %s has been cancelled by %s. If a payment was captured, the refund will follow shortly.", in.OrderNumber, theme.StoreName)
		}
		cta = "View order"
	case KindRefund:
		if in.IsFullRefund {
			subject = fmt.Sprintf("Refund issued for order %s — fully refunded", in.OrderNumber)
			heading = "Your refund has been issued in full."
		} else {
			subject = fmt.Sprintf("Partial refund issued for order %s", in.OrderNumber)
			heading = "A partial refund has been issued."
		}
		lede = fmt.Sprintf("We've issued a refund of %s %s against order %s. It typically takes 3–10 business days to appear on your statement, depending on your bank.", in.RefundAmount.StringFixed(2), in.CurrencyCode, in.OrderNumber)
		cta = "View order"
	default: // KindInvoice
		subject = fmt.Sprintf("Invoice for order %s — confirmed", in.OrderNumber)
		heading = "Your order is confirmed."
		lede = fmt.Sprintf("This is your invoice for order %s. We're getting it ready and will let you know when it ships.", in.OrderNumber)
		cta = "View invoice"
	}

	deliveredAt := ""
	if in.DeliveredAt != nil {
		deliveredAt = in.DeliveredAt.Format("January 2, 2006")
	}

	data := renderData{
		Subject:            subject,
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
		Theme:              theme,
		HairlineColor:      emailHairline,
		SupportingCopy:     emailSupporting,
		MutedCopy:          emailMuted,
		FaintCopy:          emailFaint,
	}

	var htmlTmpl, textTmpl *template.Template
	switch kind {
	case KindReceipt:
		htmlTmpl, textTmpl = receiptHTMLTemplate, receiptTextTemplate
	case KindCancellation:
		htmlTmpl, textTmpl = cancellationHTMLTemplate, cancellationTextTemplate
	case KindRefund:
		htmlTmpl, textTmpl = refundHTMLTemplate, refundTextTemplate
	default:
		htmlTmpl, textTmpl = invoiceHTMLTemplate, invoiceTextTemplate
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", "", fmt.Errorf("orderdoc: render html: %w", err)
	}
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", "", fmt.Errorf("orderdoc: render text: %w", err)
	}
	return subject, htmlBuf.String(), textBuf.String(), nil
}

// -- LogMailer ---------------------------------------------------------

// LogMailer logs the would-be dispatch instead of sending. Used when no
// SendGrid API key is configured (local dev, integration tests).
type LogMailer struct {
	Logger *slog.Logger
}

// SendInvoice logs the invoice email payload.
func (m *LogMailer) SendInvoice(_ context.Context, in DocumentInput) error {
	return m.log(KindInvoice, in)
}

// SendReceipt logs the receipt email payload.
func (m *LogMailer) SendReceipt(_ context.Context, in DocumentInput) error {
	return m.log(KindReceipt, in)
}

// SendCancellation logs the cancellation email payload.
func (m *LogMailer) SendCancellation(_ context.Context, in DocumentInput) error {
	return m.log(KindCancellation, in)
}

// SendRefund logs the refund email payload.
func (m *LogMailer) SendRefund(_ context.Context, in DocumentInput) error {
	return m.log(KindRefund, in)
}

func (m *LogMailer) log(kind Kind, in DocumentInput) error {
	m.Logger.Info("orderdoc: email (log-only)",
		"kind", string(kind),
		"recipient", in.Recipient,
		"document_number", in.DocumentNumber,
		"order_number", in.OrderNumber,
		"store", in.Theme.StoreName,
	)
	return nil
}

// -- SendGridMailer ----------------------------------------------------

// SendGridMailer sends order document emails via SendGrid v3. Mirrors
// the giftcard mailer pattern (thin HTTP, no SDK).
type SendGridMailer struct {
	apiKey     string
	from       string
	client     *http.Client
	logger     *slog.Logger
	pdfFetcher StorefrontPDFFetcher
}

// NewSendGridMailer constructs a SendGrid-backed Mailer.
func NewSendGridMailer(apiKey, from string, logger *slog.Logger) *SendGridMailer {
	return &SendGridMailer{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 15 * time.Second},
		logger: logger,
	}
}

// WithStorefrontPDFFetcher wires a fetcher that retrieves the rendered
// PDF (invoice / receipt) from the storefront over the internal-auth
// channel. When set, SendInvoice and SendReceipt attach the PDF to the
// outgoing email — the canonical document arrives directly in the
// buyer's inbox instead of asking them to click through to download it.
func (m *SendGridMailer) WithStorefrontPDFFetcher(f StorefrontPDFFetcher) *SendGridMailer {
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
func (m *SendGridMailer) SendInvoice(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindInvoice, in)
}

// SendReceipt renders + dispatches the receipt envelope.
func (m *SendGridMailer) SendReceipt(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindReceipt, in)
}

// SendCancellation renders + dispatches the cancellation envelope.
func (m *SendGridMailer) SendCancellation(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindCancellation, in)
}

// SendRefund renders + dispatches the refund-issued envelope.
func (m *SendGridMailer) SendRefund(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindRefund, in)
}

func (m *SendGridMailer) send(ctx context.Context, kind Kind, in DocumentInput) error {
	if m.apiKey == "" {
		return fmt.Errorf("orderdoc: SendGrid API key not configured")
	}
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
	var attachments []sgAttachment
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
			attachments = []sgAttachment{{
				Content:     base64.StdEncoding.EncodeToString(pdfBytes),
				Filename:    filename,
				Type:        "application/pdf",
				Disposition: "attachment",
			}}
			hasAttachment = true
		}
	}

	subject, htmlBody, textBody, err := render(kind, in, hasAttachment)
	if err != nil {
		return err
	}

	falsePtr := false
	payload := sgRequest{
		Personalizations: []sgPersonalization{{To: []sgAddress{{Email: in.Recipient}}}},
		From:             sgAddress{Email: m.from},
		Subject:          subject,
		Content: []sgContent{
			{Type: "text/plain", Value: textBody},
			{Type: "text/html", Value: htmlBody},
		},
		Attachments: attachments,
		TrackingSettings: &sgTrackingSettings{
			ClickTracking:        &sgClickTracking{Enable: &falsePtr, EnableText: &falsePtr},
			OpenTracking:         &sgOpenTracking{Enable: &falsePtr},
			SubscriptionTracking: &sgSubscriptionTracking{Enable: &falsePtr},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("orderdoc: marshal sendgrid request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("orderdoc: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("orderdoc: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("orderdoc: sendgrid returned %d: %s", resp.StatusCode, string(body))
}

// -- SendGrid wire types (subset) --------------------------------------

type sgRequest struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	Attachments      []sgAttachment      `json:"attachments,omitempty"`
	TrackingSettings *sgTrackingSettings `json:"tracking_settings,omitempty"`
}

// sgAttachment is SendGrid's attachment wire shape. Content MUST be
// base64-encoded; SendGrid rejects raw bytes with a 400.
type sgAttachment struct {
	Content     string `json:"content"`
	Filename    string `json:"filename"`
	Type        string `json:"type,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

type sgTrackingSettings struct {
	ClickTracking        *sgClickTracking        `json:"click_tracking,omitempty"`
	OpenTracking         *sgOpenTracking         `json:"open_tracking,omitempty"`
	SubscriptionTracking *sgSubscriptionTracking `json:"subscription_tracking,omitempty"`
}

type sgClickTracking struct {
	Enable     *bool `json:"enable,omitempty"`
	EnableText *bool `json:"enable_text,omitempty"`
}

type sgOpenTracking struct {
	Enable *bool `json:"enable,omitempty"`
}

type sgSubscriptionTracking struct {
	Enable *bool `json:"enable,omitempty"`
}

type sgPersonalization struct {
	To []sgAddress `json:"to"`
}

type sgAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
