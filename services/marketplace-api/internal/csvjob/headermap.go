package csvjob

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CanonicalColumns are the column names ParseRow understands. A merchant's
// own export will rarely use them — "Variant Price", "Variant SKU" and so
// on — which is what the admin column mapper is for.
var CanonicalColumns = []string{
	colTitle, colHandle, colDescription, colStatus,
	colBasePrice, colSKU, colStock, colWeight, colCategorySlugs,
}

func isCanonicalColumn(name string) bool {
	for _, c := range CanonicalColumns {
		if c == name {
			return true
		}
	}
	return false
}

// ValidateColumnMapping checks a merchant-supplied mapping of CSV header →
// canonical column. An empty target means "skip this column".
//
// Two headers pointing at one canonical column is rejected rather than
// resolved by last-write-wins: silently importing the wrong column into
// price is worse than refusing the upload.
func ValidateColumnMapping(mapping map[string]string) error {
	claimed := make(map[string]string, len(mapping))
	for header, target := range mapping {
		if target == "" {
			continue
		}
		if !isCanonicalColumn(target) {
			return fmt.Errorf("csvjob: %q is not a column that can be imported", target)
		}
		if first, dup := claimed[target]; dup {
			return fmt.Errorf("csvjob: columns %q and %q are both mapped to %q", first, header, target)
		}
		claimed[target] = header
	}
	return nil
}

// ApplyColumnMapping rewrites a CSV's header row to canonical column names
// so the parser, which matches on header name, sees what the merchant
// chose in the mapper.
//
// Rewriting on the way into storage rather than carrying the mapping
// through to the worker keeps the import path single-shaped: whatever is
// in the bucket is already canonical, and replay, error CSVs and the
// parser all keep working unchanged.
//
// Only the header row is touched — data rows are streamed through byte for
// byte, so a 50,000-row file is not buffered.
func ApplyColumnMapping(r io.Reader, mapping map[string]string) (io.Reader, error) {
	if len(mapping) == 0 {
		return r, nil
	}
	if err := ValidateColumnMapping(mapping); err != nil {
		return nil, err
	}

	br := bufio.NewReader(r)

	// A BOM would otherwise become part of the first header's name and stop
	// it from ever matching — Excel writes one by default.
	if bom, err := br.Peek(3); err == nil && bytes.Equal(bom, []byte{0xEF, 0xBB, 0xBF}) {
		if _, err := br.Discard(3); err != nil {
			return nil, fmt.Errorf("csvjob: discard BOM: %w", err)
		}
	}

	header, err := csv.NewReader(newHeaderLimitedReader(br)).Read()
	if err != nil {
		return nil, fmt.Errorf("csvjob: read header row: %w", err)
	}

	rewritten := make([]string, len(header))
	for i, h := range header {
		target, ok := mapping[h]
		if !ok {
			// Not in the mapping at all: leave it as it is, so a CSV whose
			// headers are already canonical still works when the client
			// sends a partial mapping.
			rewritten[i] = h
			continue
		}
		// An explicitly skipped column is blanked rather than left alone —
		// otherwise a column the merchant chose to skip would still be
		// imported whenever its name happened to be canonical.
		rewritten[i] = target
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(rewritten); err != nil {
		return nil, fmt.Errorf("csvjob: write header row: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("csvjob: flush header row: %w", err)
	}

	return io.MultiReader(&buf, br), nil
}

// newHeaderLimitedReader hands encoding/csv just the first record.
//
// csv.Reader buffers ahead, so reading the header straight from the shared
// bufio.Reader would swallow the start of the data rows and they would
// never reach the bucket. This reads one line — honouring quoted newlines,
// which a header may legally contain — and then stops.
func newHeaderLimitedReader(br *bufio.Reader) io.Reader {
	var line []byte
	inQuotes := false
	for {
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		line = append(line, b)
		if b == '"' {
			inQuotes = !inQuotes
		}
		if b == '\n' && !inQuotes {
			break
		}
	}
	return strings.NewReader(string(line))
}

// SortedCanonicalColumns is the canonical set in a stable order, for tests
// and for anything that renders the list.
func SortedCanonicalColumns() []string {
	out := append([]string(nil), CanonicalColumns...)
	sort.Strings(out)
	return out
}
