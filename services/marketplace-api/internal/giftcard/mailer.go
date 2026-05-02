package giftcard

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
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)



// Mailer sends gift-card lifecycle emails. The initial implementation
// only sends a "delivery" email when a gift card is issued with a
// recipient; future phases can add "balance low" / "expiry reminder"
// triggers behind the same interface.
type Mailer interface {
	SendDelivery(ctx context.Context, in DeliveryInput) error
}

// DeliveryInput is everything the delivery template needs. The send
// worker / handler fills it in once the gift card is persisted.
type DeliveryInput struct {
	Recipient string              // required — recipient email
	// TenantID is forwarded to SendGrid as a custom_arg for per-tenant
	// engagement attribution. Optional — empty values are omitted.
	TenantID  string
	Card      *GiftCard           // the issued card (code, balance, message, expiry)
	Theme     GiftCardEmailTheme  // store branding for masthead / colors / fonts
	StorefrontURL string          // e.g. https://acme.mark8ly.com — used for CTA
}

// GiftCardEmailTheme is the subset of store branding surfaced in the
// gift-card email. Kept as a plain struct (not a branding type) so the
// giftcard package doesn't depend on the branding package internals.
type GiftCardEmailTheme struct {
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

// Default palette mirrors packages/ui/src/styles/mark8ly-tokens.css so a
// gift card issued for a store with no custom branding still feels on-
// brand with the platform.
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

// withDefaults fills empty fields with the editorial defaults.
func (t GiftCardEmailTheme) withDefaults() GiftCardEmailTheme {
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

type deliveryData struct {
	// Subject removed — rendered via the loader from a separate
	// subject-template column. See RegisterFallbacks below.
	DisplayCode     string
	Balance         string
	CurrencyCode    string
	ExpiresAt       string
	SenderName      string
	RecipientName   string
	Message         template.HTML
	StorefrontURL   string
	Theme           GiftCardEmailTheme
	HairlineColor   string
	SupportingCopy  string
	MutedCopy       string
	FaintCopy       string
}

// Embedded template strings — kept as fallback for the loader.
var (
	deliveryEmbeddedHTML = mustReadGiftcardEmbedded("templates/gift_card_delivery.html")
	deliveryEmbeddedText = mustReadGiftcardEmbedded("templates/gift_card_delivery.txt")
)

func mustReadGiftcardEmbedded(path string) string {
	raw, err := templateFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("giftcard mailer: embedded template %s missing: %v", path, err))
	}
	return string(raw)
}

// DeliverySubjectTpl is the registered subject template. Operator
// can override per key in tesserix-home; this is the embedded fallback.
const DeliverySubjectTpl = "You received a gift card from {{.Theme.StoreName}}"

// DeliveryTemplateKey is the loader registry key for the delivery
// email. Public so tests can refer to it.
const DeliveryTemplateKey = "giftcard_delivery"

// RegisterFallbacks registers the giftcard delivery template's
// embedded fallback against the shared loader. Called once at boot.
func RegisterFallbacks(loader *emailtemplates.Loader) {
	loader.Register(DeliveryTemplateKey, emailtemplates.EmbeddedFallback{
		Subject:  DeliverySubjectTpl,
		HTMLBody: deliveryEmbeddedHTML,
		TextBody: deliveryEmbeddedText,
	})
}

func renderDelivery(ctx context.Context, loader *emailtemplates.Loader, in DeliveryInput) (string, string, string, error) {
	if in.Card == nil {
		return "", "", "", fmt.Errorf("giftcard mailer: nil card")
	}
	theme := in.Theme.withDefaults()

	data := deliveryData{
		DisplayCode:    FormatCodeDisplay(in.Card.Code),
		Balance:        formatMoney(in.Card.InitialBalance),
		CurrencyCode:   in.Card.CurrencyCode,
		ExpiresAt:      formatExpiry(in.Card.ExpiresAt),
		SenderName:     deref(in.Card.SenderName),
		RecipientName:  deref(in.Card.RecipientName),
		Message:        renderMessage(in.Card.Message),
		StorefrontURL:  in.StorefrontURL,
		Theme:          theme,
		HairlineColor:  emailHairline,
		SupportingCopy: emailSupporting,
		MutedCopy:      emailMuted,
		FaintCopy:      emailFaint,
	}

	if loader == nil {
		// Test path with no loader — render embedded directly via
		// a temporary loader.
		loader = emailtemplates.NewLoader(nil)
		RegisterFallbacks(loader)
	}
	r, err := loader.Render(ctx, DeliveryTemplateKey, data)
	if err != nil {
		return "", "", "", fmt.Errorf("giftcard mailer: %w", err)
	}
	return r.Subject, r.HTMLBody, r.TextBody, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatMoney(d decimal.Decimal) string {
	return d.StringFixed(2)
}

func formatExpiry(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("January 2, 2006")
}

// renderMessage turns the (plain-text) sender message into a safely
// HTML-escaped paragraph with <br> for line breaks.
func renderMessage(msg *string) template.HTML {
	if msg == nil || strings.TrimSpace(*msg) == "" {
		return ""
	}
	escaped := template.HTMLEscapeString(*msg)
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return template.HTML(escaped)
}

// -- LogMailer ---------------------------------------------------------

// LogMailer is a Mailer that logs instead of sending. Used when
// SENDGRID_API_KEY is unset (local dev, tests) so we don't silently
// drop delivery attempts.
type LogMailer struct {
	Logger *slog.Logger
}

// SendDelivery logs the would-be dispatch.
func (m *LogMailer) SendDelivery(_ context.Context, in DeliveryInput) error {
	if in.Card == nil {
		return fmt.Errorf("giftcard LogMailer: nil card")
	}
	m.Logger.Info("giftcard: delivery email (log-only)",
		"recipient", in.Recipient,
		"code", in.Card.Code,
		"balance", in.Card.InitialBalance.String(),
		"currency", in.Card.CurrencyCode,
		"store", in.Theme.StoreName)
	return nil
}

// -- SendGridMailer ----------------------------------------------------

// SendGridMailer sends the delivery email via SendGrid v3. Thin HTTP
// client (no SDK) consistent with campaign.SendGridDispatcher and
// platform-api's notification sender.
type SendGridMailer struct {
	apiKey string
	from   string
	client *http.Client
	logger *slog.Logger
	loader *emailtemplates.Loader
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

// WithLoader wires the shared template loader. When set, render uses
// DB-backed templates with embedded fallback. When nil, embedded only.
func (m *SendGridMailer) WithLoader(l *emailtemplates.Loader) *SendGridMailer {
	m.loader = l
	return m
}

// SendDelivery renders the gift-card delivery envelope and POSTs to
// SendGrid v3. Returns a non-nil error on any failure; callers should
// treat the error as non-fatal for the gift-card issue operation
// itself (the card is already persisted).
func (m *SendGridMailer) SendDelivery(ctx context.Context, in DeliveryInput) error {
	if m.apiKey == "" {
		return fmt.Errorf("giftcard: SendGrid API key not configured")
	}
	if in.Recipient == "" {
		return fmt.Errorf("giftcard: missing recipient")
	}

	subject, htmlBody, textBody, err := renderDelivery(ctx, m.loader, in)
	if err != nil {
		return err
	}

	customArgs := map[string]string{"product": "mark8ly", "kind": "giftcard"}
	if in.TenantID != "" {
		customArgs["tenant_id"] = in.TenantID
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
		CustomArgs: customArgs,
		TrackingSettings: &sgTrackingSettings{
			ClickTracking:        &sgClickTracking{Enable: &falsePtr, EnableText: &falsePtr},
			OpenTracking:         &sgOpenTracking{Enable: &falsePtr},
			SubscriptionTracking: &sgSubscriptionTracking{Enable: &falsePtr},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("giftcard: marshal sendgrid request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.sendgrid.com/v3/mail/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("giftcard: build sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("giftcard: sendgrid POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("giftcard: sendgrid returned %d: %s", resp.StatusCode, string(respBody))
}

// -- SendGrid wire types (minimum viable subset) -----------------------

type sgRequest struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	CustomArgs       map[string]string   `json:"custom_args,omitempty"`
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

// -- Helpers (unused but silence unused-import linter when editing) ----

var _ = regexp.MustCompile
var _ = uuid.Nil
