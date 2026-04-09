package product

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// maxHandleLen caps generated handles per spec §4 (handle column is
// varchar(200)).
const maxHandleLen = 200

// GenerateHandle converts a human title to a URL-safe handle. Unicode-aware:
// combining marks are stripped (e.g. "Café" -> "cafe"), spaces become single
// dashes, punctuation is dropped. The output is lowercase ASCII [a-z0-9-].
func GenerateHandle(title string) string {
	title = norm.NFKD.String(title)
	var b strings.Builder
	var lastDash bool
	for _, r := range title {
		switch {
		case unicode.IsMark(r):
			continue
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == '&':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > maxHandleLen {
		out = out[:maxHandleLen]
	}
	return out
}

// SuggestNextHandle tries base, then base-2, base-3, ... up to 50. If all
// are taken, falls back to base + "-" + 6 random hex chars.
func SuggestNextHandle(base string, taken func(candidate string) bool) string {
	if !taken(base) {
		return base
	}
	for i := 2; i <= 50; i++ {
		candidate := base + "-" + itoa(i)
		if !taken(candidate) {
			return candidate
		}
	}
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return base + "-zzzzzz"
	}
	return base + "-" + hex.EncodeToString(buf[:])
}
