// Package product — personalisation_text_style.go: the fonts and colours a
// merchant may offer on a text personalisation field.
//
// Fonts are a CLOSED catalogue, not free family names. The storefront
// hosts exactly these and renders the buyer's preview in them, and the
// merchant reads the chosen id back off the order; a free-text family
// would preview in whatever the browser fell back to and tell the
// merchant nothing they could produce. Adding a font is a change here AND
// in the storefront's font map (apps/storefront/lib/personalisation-fonts.ts),
// which the storefront's test pins to this list.
//
// Colours are the merchant's own, named swatches. "Gold foil" is what an
// engraver produces; #C9A227 is only how the preview approximates it.
package product

import (
	"regexp"
	"strings"
)

// TextFont is one font a merchant may offer.
type TextFont struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// TextFonts is the catalogue, in the order the admin lists it. The ids are
// stored on fields and snapshotted onto orders, so they never change once
// shipped; a label may.
var TextFonts = []TextFont{
	{ID: "classic", Label: "Classic serif"},
	{ID: "modern", Label: "Modern sans"},
	{ID: "script", Label: "Script"},
	{ID: "handwritten", Label: "Handwritten"},
	{ID: "block", Label: "Block capitals"},
	{ID: "typewriter", Label: "Typewriter"},
}

// IsTextFont reports whether id is in the catalogue.
func IsTextFont(id string) bool {
	for _, f := range TextFonts {
		if f.ID == id {
			return true
		}
	}
	return false
}

// TextFontLabel is the merchant-facing name for a font id. An id no longer
// in the catalogue (a font retired after the order) is returned as-is, so
// an old order still says what was asked for.
func TextFontLabel(id string) string {
	for _, f := range TextFonts {
		if f.ID == id {
			return f.Label
		}
	}
	return id
}

// DescribeTextStyle renders a buyer's choices as "Script, Gold foil" for a
// packing slip or an email. Empty when there was nothing to choose.
func DescribeTextStyle(font, colourLabel *string) string {
	parts := make([]string, 0, 2)
	if font != nil && *font != "" {
		parts = append(parts, TextFontLabel(*font))
	}
	if colourLabel != nil && *colourLabel != "" {
		parts = append(parts, *colourLabel)
	}
	return strings.Join(parts, ", ")
}

// TextColour is one colour a merchant offers, as stored in the field's
// text_colours jsonb array.
type TextColour struct {
	Label string `json:"label"`
	Hex   string `json:"hex"`
}

// Bounds on the style lists. Small on purpose: these render as a row of
// swatches and a short menu under one text box, and a buyer choosing
// between twenty fonts for a name on a mug is choosing nothing.
const (
	MaxTextFontsPerField   = 6
	MaxTextColoursPerField = 12
	maxTextColourLabel     = 60
)

var hexColourRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// NormaliseTextColour trims the label and upper-cases the hex, which is the
// form the order snapshot's CHECK accepts. ok is false when either is
// unusable.
func NormaliseTextColour(c TextColour) (TextColour, bool) {
	label := strings.TrimSpace(c.Label)
	hex := strings.TrimSpace(c.Hex)
	if label == "" || len(label) > maxTextColourLabel || !hexColourRe.MatchString(hex) {
		return TextColour{}, false
	}
	return TextColour{Label: label, Hex: strings.ToUpper(hex)}, true
}
