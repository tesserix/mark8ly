package tax

import (
	"regexp"
	"strings"
)

// NameMatch mirrors the tax_id_name_match enum from migration 040.
type NameMatch string

const (
	NameMatched    NameMatch = "matched"
	NameUnmatched  NameMatch = "unmatched"
	NameNotChecked NameMatch = "not_checked"
)

// CompareNames runs a normalized-Levenshtein fuzzy match. Returns NameNotChecked
// when either side is empty (e.g. VIES privacy-protected response, or attestation-
// only US/CA where the validator mirrors the submitted name back).
//
// Tolerance: ≤10% Levenshtein edit distance with a minimum 2-char budget for
// short names. Corporate suffixes (Pty Ltd ↔ Proprietary Limited, Sdn Bhd ↔
// Sendirian Berhad, etc.) are canonicalised before comparison.
func CompareNames(submitted, registry string) NameMatch {
	if submitted == "" || registry == "" {
		return NameNotChecked
	}
	a := normalize(submitted)
	b := normalize(registry)
	if a == b {
		return NameMatched
	}
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return NameNotChecked
	}
	threshold := max(maxLen/10, 2)
	if levenshtein(a, b) <= threshold {
		return NameMatched
	}
	return NameUnmatched
}

var (
	punctRegex      = regexp.MustCompile(`[^\p{L}\p{N}\s]`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// corporateSuffixes maps short forms to their canonical long form. Keys are
// matched against the trailing token; the longest match wins.
var corporateSuffixes = []struct{ abbrev, canon string }{
	{"sdn bhd", "sendirian berhad"},
	{"pty ltd", "proprietary limited"},
	{"pty limited", "proprietary limited"},
	{"pvt ltd", "private limited"},
	{"private ltd", "private limited"},
	{"plc", "public limited company"},
	{"llc", "limited liability company"},
	{"gmbh", "gesellschaft mit beschrankter haftung"},
	{"bv", "besloten vennootschap"},
	{"ltd", "limited"},
	{"inc", "incorporated"},
	{"corp", "corporation"},
	{"co", "company"},
	{"sa", "societe anonyme"},
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = punctRegex.ReplaceAllString(s, " ")
	s = whitespaceRegex.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	for _, suf := range corporateSuffixes {
		if strings.HasSuffix(s, " "+suf.abbrev) {
			s = strings.TrimSuffix(s, " "+suf.abbrev) + " " + suf.canon
		} else if s == suf.abbrev {
			s = suf.canon
		}
	}
	return strings.TrimSpace(s)
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(x, y, z int) int {
	if x <= y && x <= z {
		return x
	}
	if y <= z {
		return y
	}
	return z
}
