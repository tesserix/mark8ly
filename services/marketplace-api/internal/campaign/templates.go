package campaign

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"regexp"
	"strings"
)

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

// CampaignTheme is the per-store branding surfaced in campaign emails.
// Populated from the storefront branding record (store_branding table)
// before rendering. Empty/zero fields fall back to the editorial
// Paper/Ink/Moss defaults so a merchant who never visited Branding still
// gets a polished email.
type CampaignTheme struct {
	StoreName       string // merchant-facing name, used for masthead wordmark + "from"
	LogoURL         string // optional — rendered as <img> when non-empty
	Tagline         string // short store tagline, shown under wordmark
	ColorBackground string // page background (paper)
	ColorText       string // body copy color
	ColorAccent     string // links + masthead wordmark accent
	ColorButtonBg   string // reserved for future CTA buttons
	ColorButtonText string // reserved for future CTA buttons
	HeadingFont     string // serif cascade for h1/h2/h3 (CSS font-family)
	BodyFont        string // sans cascade for body (CSS font-family)
	FooterTagline   string // editorial sign-off line
	FooterCopyright string // copyright footer (e.g. "© 2026 Acme Store")
	Unsubscribe     string // unsubscribe link (future)
}

// Default palette — matches packages/ui/src/styles/mark8ly-tokens.css and
// services/platform-api/internal/notification/templates/welcome.html so a
// campaign with no store branding still feels on-brand with the platform.
const (
	defaultBackground  = "#F7F6F2"
	defaultText        = "#0E0E0C"
	defaultAccent      = "#2D4A2B"
	defaultButtonBg    = "#0E0E0C"
	defaultButtonText  = "#FDFCF8"
	defaultHeadingFont = "Georgia, 'Source Serif 4', 'Times New Roman', serif"
	defaultBodyFont    = "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Source Sans 3', Helvetica, Arial, sans-serif"
	hairlineColor      = "#ECEAE3"
	supportingCopy     = "#45433E"
	mutedCopy          = "#7A766E"
	faintCopy          = "#A09C92"
)

// withDefaults fills empty fields on a theme with the editorial defaults.
// Called from the render helpers so theme consumers never need to pre-fill.
func (t CampaignTheme) withDefaults() CampaignTheme {
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

// campaignEnvelopeData is the template input. Body is merchant-authored
// TipTap HTML; we wrap it in template.HTML so html/template doesn't
// re-escape the tags the merchant deliberately added.
type campaignEnvelopeData struct {
	Subject        string
	Body           template.HTML
	TextBody       string
	Theme          CampaignTheme
	HairlineColor  string
	SupportingCopy string
	MutedCopy      string
	FaintCopy      string
}

var (
	campaignHTMLTemplate = template.Must(
		template.ParseFS(templateFS, "templates/campaign_envelope.html"),
	)
	campaignTextTemplate = template.Must(
		template.ParseFS(templateFS, "templates/campaign_envelope.txt"),
	)
)

// RenderCampaignHTML wraps the merchant body in the store's branded
// editorial envelope. The body is treated as trusted template.HTML.
func RenderCampaignHTML(subject, body string, theme CampaignTheme) (string, error) {
	var buf bytes.Buffer
	data := campaignEnvelopeData{
		Subject:        subject,
		Body:           template.HTML(body),
		Theme:          theme.withDefaults(),
		HairlineColor:  hairlineColor,
		SupportingCopy: supportingCopy,
		MutedCopy:      mutedCopy,
		FaintCopy:      faintCopy,
	}
	if err := campaignHTMLTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("campaign: render html envelope: %w", err)
	}
	return buf.String(), nil
}

// RenderCampaignText derives the plain-text body from the HTML body,
// then wraps it in the text envelope.
func RenderCampaignText(subject, htmlBody string, theme CampaignTheme) (string, error) {
	var buf bytes.Buffer
	data := campaignEnvelopeData{
		Subject:  subject,
		TextBody: htmlToPlainText(htmlBody),
		Theme:    theme.withDefaults(),
	}
	if err := campaignTextTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("campaign: render text envelope: %w", err)
	}
	return buf.String(), nil
}

// tagRE strips HTML tags for the plain-text fallback.
var tagRE = regexp.MustCompile(`<[^>]+>`)

// htmlToPlainText produces a minimal plain-text rendering by stripping
// tags and collapsing whitespace. Sufficient for spam-filter multi-part
// requirements; not a semantic HTML→text conversion.
func htmlToPlainText(html string) string {
	html = strings.NewReplacer(
		"</p>", "\n\n",
		"</h1>", "\n\n",
		"</h2>", "\n\n",
		"</h3>", "\n\n",
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</li>", "\n",
	).Replace(html)

	text := tagRE.ReplaceAllString(html, "")
	text = strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&#39;", "'",
		"&quot;", `"`,
	).Replace(text)

	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.Join(strings.Fields(l), " "))
	}
	joined := strings.Join(out, "\n")
	for strings.Contains(joined, "\n\n\n") {
		joined = strings.ReplaceAll(joined, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(joined)
}
