// Package orderdoc dispatches invoice and receipt emails for an order.
//
// Two distinct lifecycle hooks:
//   • Invoice — fired the moment an order is confirmed (accepted) by
//     the merchant. Tells the customer "we've taken your order, here's
//     the paperwork".
//   • Receipt — fired the moment a shipment transitions to delivered.
//     Confirms the transaction is complete.
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
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

// Kind discriminates between the two documents this package mails.
type Kind string

const (
	KindInvoice Kind = "invoice"
	KindReceipt Kind = "receipt"
)

// Mailer is the contract between the order/shipment lifecycle and the
// underlying email transport. Two methods (rather than one with a Kind)
// so handler code reads as intent: "SendInvoice" / "SendReceipt".
type Mailer interface {
	SendInvoice(ctx context.Context, in DocumentInput) error
	SendReceipt(ctx context.Context, in DocumentInput) error
}

// DocumentInput is everything the email template needs to render. The
// Service builder fills it from the order + branding records before
// handing off to the mailer.
type DocumentInput struct {
	// Recipient — required. Validated by the mailer; empty addresses
	// silently no-op so a missing customer email never crashes a confirm.
	Recipient string

	// Document identity — formatted "INV-PLA-260417-01154" / "RCP-...".
	DocumentNumber string

	// Order context — used in subject line + email body.
	OrderNumber  string
	OrderURL     string // deep link to /account/orders/:id on the storefront
	PlacedAt     time.Time
	DeliveredAt  *time.Time // populated for receipts only

	// Money summary
	GrandTotal   decimal.Decimal
	CurrencyCode string
	ItemCount    int

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
// stores still feel like the platform.
const (
	emailHairline      = "#ECEAE3"
	emailSupporting    = "#45433E"
	emailMuted         = "#7A766E"
	emailFaint         = "#A09C92"
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
	OrderURL        string
	PlacedAt        string
	DeliveredAt     string
	GrandTotal      string
	CurrencyCode    string
	ItemCount       int
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
)

func render(kind Kind, in DocumentInput) (subject, html, text string, err error) {
	theme := in.Theme.withDefaults()

	var heading, lede, cta string
	if kind == KindReceipt {
		subject = fmt.Sprintf("Receipt for order %s — delivered", in.OrderNumber)
		heading = "Your order has been delivered."
		lede = fmt.Sprintf("This is your receipt for order %s. Keep it for your records — and thank you for shopping with %s.", in.OrderNumber, theme.StoreName)
		cta = "View receipt"
	} else {
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
		Subject:        subject,
		Heading:        heading,
		Lede:           lede,
		CTAButtonLabel: cta,
		DocumentNumber: in.DocumentNumber,
		OrderNumber:    in.OrderNumber,
		OrderURL:       in.OrderURL,
		PlacedAt:       in.PlacedAt.Format("January 2, 2006"),
		DeliveredAt:    deliveredAt,
		GrandTotal:     in.GrandTotal.StringFixed(2),
		CurrencyCode:   in.CurrencyCode,
		ItemCount:      in.ItemCount,
		Theme:          theme,
		HairlineColor:  emailHairline,
		SupportingCopy: emailSupporting,
		MutedCopy:      emailMuted,
		FaintCopy:      emailFaint,
	}

	htmlTmpl := invoiceHTMLTemplate
	textTmpl := invoiceTextTemplate
	if kind == KindReceipt {
		htmlTmpl = receiptHTMLTemplate
		textTmpl = receiptTextTemplate
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
	apiKey string
	from   string
	client *http.Client
	logger *slog.Logger
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

// SendInvoice renders + dispatches the invoice envelope.
func (m *SendGridMailer) SendInvoice(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindInvoice, in)
}

// SendReceipt renders + dispatches the receipt envelope.
func (m *SendGridMailer) SendReceipt(ctx context.Context, in DocumentInput) error {
	return m.send(ctx, KindReceipt, in)
}

func (m *SendGridMailer) send(ctx context.Context, kind Kind, in DocumentInput) error {
	if m.apiKey == "" {
		return fmt.Errorf("orderdoc: SendGrid API key not configured")
	}
	if in.Recipient == "" {
		return fmt.Errorf("orderdoc: missing recipient")
	}
	subject, htmlBody, textBody, err := render(kind, in)
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
	TrackingSettings *sgTrackingSettings `json:"tracking_settings,omitempty"`
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
