package onboarding

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// Acquisition is the campaign context a merchant arrived with (#992).
//
// The onboarding app captures it in the browser on every page view and
// sends it with the session create request; nothing here is trusted until
// SanitizeAcquisition has been over it. Two touches are kept so the
// first-touch / last-touch question is a reporting choice rather than a
// schema change: First is the earliest arrival the browser remembered in
// its window, Last is the most recent. An untagged arrival still records
// its landing path and referrer so "direct" and "referral" are explicit
// outcomes, never an invented campaign.
type Acquisition struct {
	First *Touch `json:"first,omitempty"`
	Last  *Touch `json:"last,omitempty"`
}

// Touch is one arrival. Every field is optional and allowlisted.
type Touch struct {
	Source   string `json:"utm_source,omitempty"`
	Medium   string `json:"utm_medium,omitempty"`
	Campaign string `json:"utm_campaign,omitempty"`
	Content  string `json:"utm_content,omitempty"`
	Term     string `json:"utm_term,omitempty"`
	// LandingPath is the path of the first page viewed, query and fragment
	// removed. A full URL or anything not starting with "/" is dropped.
	LandingPath string `json:"landing_path,omitempty"`
	// Referrer is scheme://host/path of document.referrer, query, fragment
	// and userinfo removed. Anything that is not an http(s) URL is dropped.
	Referrer string `json:"referrer,omitempty"`
	// CapturedAt is RFC 3339 as the browser reported it; invalid values are
	// dropped rather than replaced with a server time, so a missing capture
	// time stays visibly missing.
	CapturedAt string `json:"captured_at,omitempty"`
}

// maxTouchFieldLen bounds every stored string. UTM values in the wild are
// short; anything longer is either a mistake or an attempt to use the
// column as free storage, and either way the tail is not attribution.
const maxTouchFieldLen = 200

// SanitizeAcquisition reduces whatever the client sent to the allowlisted
// shape above. It never returns an error: attribution is never allowed to
// block a signup, so malformed input simply yields nil and the session is
// created without it. The second return reports whether anything survived.
func SanitizeAcquisition(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var in Acquisition
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, false
	}
	out := Acquisition{
		First: sanitizeTouch(in.First),
		Last:  sanitizeTouch(in.Last),
	}
	if out.First == nil && out.Last == nil {
		return nil, false
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func sanitizeTouch(t *Touch) *Touch {
	if t == nil {
		return nil
	}
	out := Touch{
		Source:      cleanField(t.Source),
		Medium:      cleanField(t.Medium),
		Campaign:    cleanField(t.Campaign),
		Content:     cleanField(t.Content),
		Term:        cleanField(t.Term),
		LandingPath: cleanPath(t.LandingPath),
		Referrer:    cleanReferrer(t.Referrer),
		CapturedAt:  cleanTimestamp(t.CapturedAt),
	}
	if out == (Touch{}) {
		return nil
	}
	return &out
}

// cleanField trims, strips control characters, and caps the length.
func cleanField(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()
	if len(s) > maxTouchFieldLen {
		s = s[:maxTouchFieldLen]
	}
	return s
}

// cleanPath keeps only an absolute path with query and fragment removed.
func cleanPath(s string) string {
	s = cleanField(s)
	if s == "" || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return ""
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// cleanReferrer keeps scheme://host/path of an http(s) URL and nothing
// else. The query is where a referring site's own tokens live; it is not
// attribution and is never stored.
func cleanReferrer(s string) string {
	s = cleanField(s)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	clean := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	out := clean.String()
	if len(out) > maxTouchFieldLen {
		out = out[:maxTouchFieldLen]
	}
	return out
}

func cleanTimestamp(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Classification values. Match the CHECK constraint in migration 0019.
//
// external is the default and the only bucket the "genuine merchant"
// counts include. The other three exist so the funnel can report them
// separately instead of letting fixtures and dogfooding inflate the
// number that decides whether a campaign worked.
const (
	ClassificationExternal = "external"
	ClassificationInternal = "internal"
	ClassificationTest     = "test"
	ClassificationDemo     = "demo"
)

// IsClassification reports whether s is one of the four permitted values.
// Used as the allowlist for the funnel's `classification` query parameter.
func IsClassification(s string) bool {
	switch s {
	case ClassificationExternal, ClassificationInternal, ClassificationTest, ClassificationDemo:
		return true
	}
	return false
}

// testEmailDomains are the reserved example domains and local TLDs that
// only ever appear in fixtures. Hard-coded rather than configured: these
// are IANA-reserved and cannot be a real merchant.
var testEmailDomains = map[string]bool{
	"example.com": true,
	"example.org": true,
	"example.net": true,
}

var testEmailTLDs = []string{".test", ".invalid", ".localhost", ".local", ".example"}

// Classifier decides a session's classification from facts the server
// holds. The zero value classifies reserved test domains as test and
// everything else as external, so a service wired without configuration
// still reports honestly.
type Classifier struct {
	internalDomains map[string]bool
	demoEmails      map[string]bool
	demoSlugs       map[string]bool
}

// NewClassifier builds a Classifier from comma-separated configuration.
// Entries are trimmed and lowercased; empty entries are ignored.
func NewClassifier(internalDomains, demoEmails, demoSlugs []string) Classifier {
	return Classifier{
		internalDomains: toSet(internalDomains),
		demoEmails:      toSet(demoEmails),
		demoSlugs:       toSet(demoSlugs),
	}
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			set[v] = true
		}
	}
	return set
}

// ByEmail classifies at session creation, when the email is all we have.
// Order matters: an explicit demo email wins over its domain, a reserved
// test domain wins over an internal one, and anything unrecognised is
// external.
func (c Classifier) ByEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if c.demoEmails[email] {
		return ClassificationDemo
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ClassificationExternal
	}
	domain := email[at+1:]
	if testEmailDomains[domain] {
		return ClassificationTest
	}
	for _, tld := range testEmailTLDs {
		if strings.HasSuffix(domain, tld) {
			return ClassificationTest
		}
	}
	if c.internalDomains[domain] {
		return ClassificationInternal
	}
	return ClassificationExternal
}

// AtCompletion re-checks once the slug is known. A demo slug makes the
// session demo whatever its email said; otherwise the creation-time
// answer stands.
func (c Classifier) AtCompletion(current, slug string) string {
	if c.demoSlugs[strings.ToLower(strings.TrimSpace(slug))] {
		return ClassificationDemo
	}
	if current == "" {
		return ClassificationExternal
	}
	return current
}
