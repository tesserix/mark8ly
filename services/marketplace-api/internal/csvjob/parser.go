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
	WeightGrams   *int
	LengthCM      *decimal.Decimal
	WidthCM       *decimal.Decimal
	HeightCM      *decimal.Decimal
	CategorySlugs []string
	// Options are this row's option1..3 name/value pairs, by position;
	// an unused option is empty. Each row is one variant; rows sharing a
	// handle are one product.
	Options [maxOptions]OptionPair
}

// OptionPair is one optionN_name / optionN_value cell pair.
type OptionPair struct{ Name, Value string }

// maxOptions matches the product service's three option axes.
const maxOptions = 3

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
	colLengthCM      = "length_cm"
	colWidthCM       = "width_cm"
	colHeightCM      = "height_cm"
	colCategorySlugs = "category_slugs"
	colOption1Name   = "option1_name"
	colOption1Value  = "option1_value"
	colOption2Name   = "option2_name"
	colOption2Value  = "option2_value"
	colOption3Name   = "option3_name"
	colOption3Value  = "option3_value"
)

var optionCols = [maxOptions][2]string{
	{colOption1Name, colOption1Value},
	{colOption2Name, colOption2Value},
	{colOption3Name, colOption3Value},
}

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
	return parseRow(headers, row, rowNum, tracker, false)
}

// ParseVariantRow parses a row that continues the product above it (same
// handle): another variant. Title may be blank, as in a Shopify export,
// and the handle is not a duplicate.
func ParseVariantRow(headers []string, row []string, rowNum int) (ProductDraft, error) {
	return parseRow(headers, row, rowNum, nil, true)
}

func parseRow(headers []string, row []string, rowNum int, tracker *HandleTracker, continuation bool) (ProductDraft, error) {
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
	if draft.Title == "" && !continuation {
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

	// weight — whole grams, the unit the variant stores
	if weightStr := get(colWeight); weightStr != "" {
		g, err := strconv.Atoi(weightStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("weight %q is not a whole number of grams", weightStr))
		} else if g < 0 {
			errs = append(errs, "weight must not be negative")
		} else {
			draft.WeightGrams = &g
		}
	}

	// length_cm / width_cm / height_cm — the parcel size, in centimetres
	for _, d := range []struct {
		col string
		dst **decimal.Decimal
	}{{colLengthCM, &draft.LengthCM}, {colWidthCM, &draft.WidthCM}, {colHeightCM, &draft.HeightCM}} {
		s := get(d.col)
		if s == "" {
			continue
		}
		v, err := decimal.NewFromString(s)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s %q is not a number of centimetres", d.col, s))
		} else if !v.IsPositive() {
			errs = append(errs, d.col+" must be greater than 0")
		} else {
			*d.dst = &v
		}
	}

	// option1..3 — a name without a value, or the reverse, is a typo
	// that would otherwise import as a different product shape.
	for i, c := range optionCols {
		name, value := get(c[0]), get(c[1])
		switch {
		case name == "" && value == "":
			continue
		case value == "":
			errs = append(errs, fmt.Sprintf("%s %q has no %s", c[0], name, c[1]))
		case name == "" && !continuation:
			errs = append(errs, fmt.Sprintf("%s %q has no %s", c[1], value, c[0]))
		default:
			draft.Options[i] = OptionPair{Name: name, Value: value}
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
