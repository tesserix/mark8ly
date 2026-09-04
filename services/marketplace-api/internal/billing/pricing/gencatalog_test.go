package pricing

import (
	"os"
	"testing"
)

// committedCatalogDataPath is the committed generated file. It sits in this
// package's own directory, unlike genpricing's output, which lives in
// packages/ui.
const committedCatalogDataPath = "catalog_data.go"

// TestGenerateCatalogDataMatchesCommittedFile is the staleness guard for
// catalog_data.go: it re-renders the file in memory and compares it byte for
// byte against the committed copy.
//
// What this proves, and what it does not. The generator's input today is the
// pair of tables catalog_data.go itself declares, so this cannot detect a
// wrong AMOUNT — an edited amount is simply re-rendered and still matches.
// What it does catch is a hand-edit that departs from the canonical
// rendering: a row in the wrong position, a group whose prose was dropped or
// reworded, a trailing annotation that no longer matches its own value, a
// file that stopped being gofmt-clean. In short, it enforces "regenerate,
// do not hand-edit".
//
// The amounts themselves are guarded elsewhere: internal/billing/consolecatalog
// compares this catalog against the console, which owns it.
func TestGenerateCatalogDataMatchesCommittedFile(t *testing.T) {
	generated, err := GenerateCatalogData()
	if err != nil {
		t.Fatalf("GenerateCatalogData: %v", err)
	}

	// Guard against a guard that silently compares empty to empty: fail
	// loudly if generation produced something implausibly small rather than
	// a real file, before it ever gets to the byte comparison below. The
	// committed file is ~9KB; anything under 4KB has lost whole groups.
	const minPlausibleBytes = 4000
	if len(generated) < minPlausibleBytes {
		t.Fatalf("GenerateCatalogData produced only %d bytes (want at least %d) — generation is broken, "+
			"not just stale; refusing to compare against the committed file", len(generated), minPlausibleBytes)
	}

	committedBytes, err := os.ReadFile(committedCatalogDataPath)
	if err != nil {
		t.Fatalf("reading committed %s: %v (staleness check cannot run without it)", committedCatalogDataPath, err)
	}
	committed := string(committedBytes)

	// Same non-triviality guard on the file read off disk, so a path that
	// resolves to an empty or near-empty file fails loudly instead of
	// comparing empty to empty.
	if len(committed) < minPlausibleBytes {
		t.Fatalf("committed file at %s is only %d bytes (want at least %d) — "+
			"did committedCatalogDataPath resolve to the wrong file?", committedCatalogDataPath, len(committed), minPlausibleBytes)
	}

	if generated != committed {
		t.Fatalf("%s is stale: generated output does not match the committed file.\n\n"+
			"Regenerate with:\n\t%s\n\nand commit the result.\n\n"+
			"--- generated (%d bytes) vs committed (%d bytes) ---\n%s",
			committedCatalogDataPath, RegenerateCatalogCommand, len(generated), len(committed),
			diffPreview(generated, committed))
	}
}
