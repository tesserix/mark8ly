// identity.go — who a Message says it is from.
//
// Two identities exist and they are never interchangeable:
//
//   - StoreIdentity — customer-facing mail sent BY a merchant's store.
//     The customer bought from Nadia's Ceramics; the inbox line should
//     say so. Display name = the store name, local part = the store
//     slug, Reply-To = the merchant's contact address.
//
//   - PlatformIdentity — mail mark8ly sends TO a merchant as their
//     provider: dunning, trial reminders, payment-action, win-back,
//     auth. Dressing a failed-payment notice in the merchant's own
//     brand is misleading, so these keep the platform address and the
//     platform's own display name.
//
// There is deliberately NO default. Neither identity is applied by the
// transport or by any shared middleware — every mailer picks one at its
// send site, so switching billing mail to a store identity requires
// editing that mailer and tripping TestIdentitySplit_ByMailer.
//
// The sending DOMAIN is never chosen here: it is taken from whatever
// address EMAIL_FROM is configured with, so this works unchanged
// whether that is noreply@tesserix.app today or a mark8ly domain after
// tesserix/tesserix-k8s#1011.
package email

import (
	"strings"
	"unicode"
)

// PlatformDisplayName is the fallback inbox name. It matches the
// "Mark8ly" fallback the order-document and gift-card bodies already
// use for a store with no name set (orderdoc.Theme.withDefaults), so
// the envelope and the body copy agree.
const PlatformDisplayName = "Mark8ly"

// maxDisplayNameRunes keeps the display name well inside the 78-octet
// RFC 5322 line limit once a provider RFC 2047-encodes a non-ASCII name
// (worst case ~4 octets per rune plus the encoded-word wrapper).
const maxDisplayNameRunes = 64

// maxLocalPartLen is the RFC 5321 local-part maximum.
const maxLocalPartLen = 64

// Identity is the resolved sender identity for one Message.
type Identity struct {
	From     string
	FromName string
	ReplyTo  string
}

// Apply writes the identity onto a Message. Mailers build their envelope
// and then apply an identity, so the choice is one visible line at the
// send site rather than a field spread across a struct literal.
func (id Identity) Apply(msg *Message) {
	msg.From = id.From
	msg.FromName = id.FromName
	msg.ReplyTo = id.ReplyTo
}

// StoreSender is the merchant-controlled input to StoreIdentity. Every
// field is untrusted text from the stores / store_branding rows.
type StoreSender struct {
	// Name is stores.name — rendered in the recipient's inbox list.
	Name string
	// Slug is stores.slug — globally unique (uniqueIndex on the column),
	// which is what makes a slug-derived local part collision-free
	// between stores without a lookup.
	Slug string
	// ContactEmail is store_branding.support_email, or "" when the
	// merchant has set none.
	ContactEmail string
}

// PlatformIdentity returns the identity for mark8ly-to-merchant mail.
// displayName is the sending surface's own name ("Mark8ly Billing");
// pass "" to send with no display name at all.
//
// ReplyTo is deliberately empty. The platform From is an unattended
// noreply box, and pointing Reply-To back at it would promise a reply
// path that does not exist.
func PlatformIdentity(from, displayName string) Identity {
	return Identity{From: from, FromName: displayName}
}

// StoreIdentity derives a store's customer-facing sender identity,
// hosted on the domain of platformFrom.
//
// Every part degrades rather than fails: an underivable slug keeps the
// platform local part, a hostile or empty name becomes "Mark8ly", and a
// missing or unroutable contact address falls back to platformFrom, so
// Reply-To is always a real mailbox and never an empty header.
func StoreIdentity(platformFrom string, s StoreSender) Identity {
	id := Identity{
		From:     platformFrom,
		FromName: SafeDisplayName(s.Name),
		ReplyTo:  platformFrom,
	}

	if local := DeriveLocalPart(s.Slug); local != "" {
		if domain := addressDomain(platformFrom); domain != "" {
			id.From = local + "@" + domain
		}
	}

	if contact := strings.TrimSpace(s.ContactEmail); contact != "" {
		if err := ValidateRecipient(contact); err == nil {
			id.ReplyTo = contact
		}
	}
	return id
}

// reservedLocalParts are local parts a store must never be able to claim.
// A store slug is merchant-chosen, so without this a merchant could take
// `billing@` or `support@` on the platform's own verified domain and send
// mail that reads as coming from mark8ly itself.
var reservedLocalParts = map[string]bool{
	"abuse": true, "accounts": true, "admin": true, "administrator": true,
	"alerts": true, "billing": true, "dmarc": true, "help": true,
	"hostmaster": true, "info": true, "mail": true, "mark8ly": true,
	"no-reply": true, "noreply": true, "notifications": true,
	"payments": true, "postmaster": true, "root": true, "sales": true,
	"security": true, "store": true, "support": true, "tesserix": true,
	"webmaster": true,
}

// escapePrefix disambiguates a reserved slug from an ordinary one.
const escapePrefix = "store-"

// DeriveLocalPart normalises a store slug into an RFC-safe local part,
// returning "" when nothing usable survives (the caller then keeps the
// platform address).
//
// The mapping is injective, so two stores can never share an address:
// slugs are globally unique, an unreserved slug maps to itself, and a
// reserved one is prefixed with "store-" — a prefix also applied to any
// slug that already starts with it, so the escaped and unescaped spaces
// stay disjoint.
func DeriveLocalPart(slug string) string {
	normalised := normaliseSlug(slug)
	if normalised == "" {
		return ""
	}
	if reservedLocalParts[normalised] || strings.HasPrefix(normalised, escapePrefix) {
		normalised = escapePrefix + normalised
	}
	if len(normalised) > maxLocalPartLen {
		normalised = strings.Trim(normalised[:maxLocalPartLen], "-")
	}
	return normalised
}

// normaliseSlug lowercases and reduces to [a-z0-9-], collapsing runs of
// separators and trimming them from both ends. Anything outside the set
// — including a non-ASCII slug that would otherwise need SMTPUTF8 —
// becomes a separator rather than being passed through.
func normaliseSlug(slug string) string {
	var b strings.Builder
	lastDash := true // suppresses a leading dash
	for _, r := range strings.ToLower(strings.TrimSpace(slug)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Display-name characters split into two tiers, because neutralising
// everything produces worse results than rejecting some outright:
// `"Support" <a@b.com>` stripped down to "Support ab.com" still reads as
// a support sender in an inbox.
//
// fatalNameChars mean the name was shaped like an address or a quoted
// phrase. There is no honest store name that needs them, so the whole
// name is discarded rather than salvaged.
const fatalNameChars = `<>@"\`

// strippableNameChars are punctuation that only misbehaves inside an
// unquoted display name — Resend takes the name inline as
// `Name <addr>`, where a comma splits the address list. They are removed
// and the rest of the name kept, so "Acme, Inc." survives as "Acme Inc.".
const strippableNameChars = `,;:()[]`

// platformTokens are names no store may wear. A store called
// "Mark8ly Support" in the inbox line is a phishing sender regardless of
// what address it sends from, so the name is dropped entirely rather
// than trimmed — a partial match ("Mark8ly Threads") is indistinguishable
// from a deliberate one at the point we can check.
var platformTokens = []string{"mark8ly", "tesserix"}

// SafeDisplayName neutralises a merchant-controlled store name for use
// as an inbox display name, falling back to PlatformDisplayName when
// nothing safe survives.
//
// Known limit: this catches literal and spaced-out platform names, not
// homoglyphs or leetspeak ("M4rk8ly"). Those need a confusables table;
// the fallback here is the floor, not the ceiling.
func SafeDisplayName(name string) string {
	// Control characters go first and are dropped, not replaced — CR/LF/
	// NUL are the header-injection vector and must not survive as
	// whitespace either. Dropping them before the fatal-character scan
	// also means "Nadia\r\nBcc: victim@example.com" is judged on the
	// address it smuggles, not on the line break.
	var b strings.Builder
	for _, r := range name {
		if !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}

	if strings.ContainsAny(b.String(), fatalNameChars) {
		return PlatformDisplayName
	}

	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(strippableNameChars, r) {
			return -1
		}
		return r
	}, b.String())

	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" || impersonatesPlatform(cleaned) {
		return PlatformDisplayName
	}
	if runes := []rune(cleaned); len(runes) > maxDisplayNameRunes {
		cleaned = strings.TrimSpace(string(runes[:maxDisplayNameRunes]))
	}
	return cleaned
}

// impersonatesPlatform reports whether name contains a platform token
// once punctuation and spacing are stripped, so "M a r k 8 l y" and
// "Mark-8-ly" are caught alongside the literal string.
func impersonatesPlatform(name string) bool {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	flattened := b.String()
	for _, token := range platformTokens {
		if strings.Contains(flattened, token) {
			return true
		}
	}
	return false
}

// addressDomain returns the domain of an address, or "" if it has none.
func addressDomain(addr string) string {
	_, domain, found := strings.Cut(strings.TrimSpace(addr), "@")
	if !found || domain == "" || strings.Contains(domain, "@") {
		return ""
	}
	return domain
}
