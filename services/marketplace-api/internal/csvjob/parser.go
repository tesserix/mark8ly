package csvjob

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// ProductDraft is the parsed result of a single CSV row, ready for
// submission to the product service's Create method.
type ProductDraft struct {
	Title         string
	Handle        string
	Description   string
	Status        string
	BasePrice     decimal.Decimal
	SKU           string
	Stock         int
	Weight        string
	CategorySlugs []string
}

// csvColumn is the canonical header name for each supported column.
const (
	colTitle         = "title"
	colHandle        = "handle"
	colDescription   = "description"
	colStatus        = "status"
	colBasePrice     = "base_price"
	colSKU           = "sku"
	colStock         = "stock"
	colWeight        = "weight"
	colCategorySlugs = "category_slugs"
)

// headerIndex maps canonical column names to their position in the CSV row.
type headerIndex map[string]int

// buildHeaderIndex builds a column-name → position map from the header row.
func buildHeaderIndex(headers []string) headerIndex {
	idx := make(headerIndex, len(headers))
	for i, h := range headers {
		idx[strings.TrimSpace(strings.ToLower(h))] = i
	}
	return idx
}

// HandleTracker detects duplicate handles within a single import batch.
type HandleTracker struct {
	seen map[string]int // handle → first row number (1-based)
}

// NewHandleTracker creates a new HandleTracker.
func NewHandleTracker() *HandleTracker {
	return &HandleTracker{seen: make(map[string]int)}
}

// Check records a handle and returns an error if it was already seen.
// rowNum is 1-based (the data row number, not the CSV line number).
func (t *HandleTracker) Check(handle string, rowNum int) error {
	lower := strings.ToLower(handle)
	if firstRow, dup := t.seen[lower]; dup {
		return fmt.Errorf("duplicate handle %q (first seen at row %d)", handle, firstRow)
	}
	t.seen[lower] = rowNum
	return nil
}

// ParseRow converts a single CSV data row into a ProductDraft.
// headers is the first row of the CSV; row is the data row.
// rowNum is 1-based for error messages. tracker detects handle dupes.
func ParseRow(headers []string, row []string, rowNum int, tracker *HandleTracker) (ProductDraft, error) {
	idx := buildHeaderIndex(headers)
	get := func(col string) string {
		i, ok := idx[col]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	var draft ProductDraft
	var errs []string

	// Required: title
	draft.Title = get(colTitle)
	if draft.Title == "" {
		errs = append(errs, "title is required")
	}

	// Required: handle
	draft.Handle = get(colHandle)
	if draft.Handle == "" {
		errs = append(errs, "handle is required")
	}

	// Duplicate handle check within this import.
	if draft.Handle != "" && tracker != nil {
		if err := tracker.Check(draft.Handle, rowNum); err != nil {
			errs = append(errs, err.Error())
		}
	}

	draft.Description = get(colDescription)
	draft.Status = get(colStatus)
	if draft.Status == "" {
		draft.Status = "draft"
	}
	draft.SKU = get(colSKU)
	draft.Weight = get(colWeight)

	// base_price — decimal
	if priceStr := get(colBasePrice); priceStr != "" {
		p, err := decimal.NewFromString(priceStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("base_price %q is not a valid decimal", priceStr))
		} else if p.IsNegative() {
			errs = append(errs, "base_price must not be negative")
		} else {
			draft.BasePrice = p
		}
	}

	// stock — integer
	if stockStr := get(colStock); stockStr != "" {
		s, err := strconv.Atoi(stockStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("stock %q is not a valid integer", stockStr))
		} else if s < 0 {
			errs = append(errs, "stock must not be negative")
		} else {
			draft.Stock = s
		}
	}

	// category_slugs — pipe-separated
	if slugs := get(colCategorySlugs); slugs != "" {
		for _, s := range strings.Split(slugs, "|") {
			s = strings.TrimSpace(s)
			if s != "" {
				draft.CategorySlugs = append(draft.CategorySlugs, s)
			}
		}
	}

	if len(errs) > 0 {
		return ProductDraft{}, fmt.Errorf("row %d: %s", rowNum, strings.Join(errs, "; "))
	}
	return draft, nil
}
