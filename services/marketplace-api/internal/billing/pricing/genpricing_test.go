package pricing

import (
	"os"
	"strings"
	"testing"
)

// committedPricingDataPath is the committed generated file's path relative
// to this package. cmd/genpricing lives at ./cmd/genpricing, so this
// package's directory is services/marketplace-api/internal/billing/pricing;
// the TS file lives at repo_root/packages/ui/src/subscription/pricing-data.ts.
const committedPricingDataPath = "../../../../../packages/ui/src/subscription/pricing-data.ts"

// TestGenerateTSMatchesCommittedFile is the staleness guard: it regenerates
// pricing-data.ts in memory from the catalog and compares it to the
// committed file. If anyone hand-edits catalog.go, display_extras.go, or
// the committed TS file without regenerating, this fails.
func TestGenerateTSMatchesCommittedFile(t *testing.T) {
	generated, err := GenerateTS()
	if err != nil {
		t.Fatalf("GenerateTS: %v", err)
	}

	// Guard against a guard that silently compares empty to empty: fail
	// loudly if generation produced something implausibly small rather than
	// a real file, before it ever gets to the byte comparison below.
	const minPlausibleBytes = 2000
	if len(generated) < minPlausibleBytes {
		t.Fatalf("GenerateTS produced only %d bytes (want at least %d) — generation is broken, "+
			"not just stale; refusing to compare against the committed file", len(generated), minPlausibleBytes)
	}

	committedBytes, err := os.ReadFile(committedPricingDataPath)
	if err != nil {
		t.Fatalf("reading committed %s: %v (staleness check cannot run without it)", committedPricingDataPath, err)
	}
	committed := string(committedBytes)

	// Same non-triviality guard on the file we read off disk, so a bad
	// relative path that happens to resolve to an empty or near-empty file
	// fails loudly instead of comparing empty to empty.
	if len(committed) < minPlausibleBytes {
		t.Fatalf("committed file at %s is only %d bytes (want at least %d) — "+
			"did committedPricingDataPath resolve to the wrong file?", committedPricingDataPath, len(committed), minPlausibleBytes)
	}

	if generated != committed {
		t.Fatalf("packages/ui/src/subscription/pricing-data.ts is stale: generated output does not match "+
			"the committed file.\n\nRegenerate with:\n\t%s\n\nand commit the result.\n\n"+
			"--- generated (%d bytes) vs committed (%d bytes) ---\n%s",
			RegenerateCommand, len(generated), len(committed), diffPreview(generated, committed))
	}
}

// diffPreview returns a short human-readable pointer to the first byte
// where generated and committed diverge, since Go's testing package has no
// built-in diff and a full 5KB dump is unreadable in CI logs.
func diffPreview(generated, committed string) string {
	genLines := strings.Split(generated, "\n")
	comLines := strings.Split(committed, "\n")
	for i := 0; i < len(genLines) && i < len(comLines); i++ {
		if genLines[i] != comLines[i] {
			return "first differing line (1-indexed line " + itoa(i+1) + "):\n" +
				"  generated: " + genLines[i] + "\n" +
				"  committed: " + comLines[i]
		}
	}
	if len(genLines) != len(comLines) {
		return "line counts differ: generated has " + itoa(len(genLines)) + " lines, committed has " + itoa(len(comLines))
	}
	return "(byte-identical lines, but string comparison still differs — check trailing bytes)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
