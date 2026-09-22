package csvjob_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

func applied(t *testing.T, in string, mapping map[string]string) string {
	t.Helper()
	r, err := csvjob.ApplyColumnMapping(strings.NewReader(in), mapping)
	require.NoError(t, err)
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(b)
}

// The merchant's own export is the case the mapper exists for: a Shopify
// or Woo CSV whose headers match nothing the parser looks for.
func TestApplyColumnMapping_RenamesMerchantHeaders(t *testing.T) {
	out := applied(t,
		"Product Name,URL Key,Variant Price\nShirt,shirt-1,10.00\n",
		map[string]string{"Product Name": "title", "URL Key": "handle", "Variant Price": "base_price"})

	require.Equal(t, "title,handle,base_price\nShirt,shirt-1,10.00\n", out)
}

// Every data row must survive byte for byte — the header reader buffering
// ahead and eating the first data row is the obvious way to get this wrong.
func TestApplyColumnMapping_PreservesAllDataRows(t *testing.T) {
	out := applied(t,
		"Name,Slug\nA,a\nB,b\nC,c\n",
		map[string]string{"Name": "title", "Slug": "handle"})

	require.Equal(t, "title,handle\nA,a\nB,b\nC,c\n", out)
}

// A skipped column must not be imported even when its own name happens to
// be one the parser would otherwise recognise.
func TestApplyColumnMapping_BlanksSkippedColumn(t *testing.T) {
	out := applied(t,
		"title,handle,status\nShirt,shirt-1,active\n",
		map[string]string{"title": "title", "handle": "handle", "status": ""})

	require.Equal(t, "title,handle,\nShirt,shirt-1,active\n", out)
}

// Excel writes a BOM by default; left in place it becomes part of the
// first header's name and that column silently never maps.
func TestApplyColumnMapping_StripsBOM(t *testing.T) {
	out := applied(t,
		"\xef\xbb\xbfName,Slug\nA,a\n",
		map[string]string{"Name": "title", "Slug": "handle"})

	require.Equal(t, "title,handle\nA,a\n", out)
}

func TestApplyColumnMapping_QuotedFieldsSurvive(t *testing.T) {
	out := applied(t,
		"Name,Notes\n\"Shirt, blue\",\"line one\nline two\"\n",
		map[string]string{"Name": "title", "Notes": "description"})

	require.Equal(t, "title,description\n\"Shirt, blue\",\"line one\nline two\"\n", out)
}

func TestApplyColumnMapping_NoMappingIsAPassThrough(t *testing.T) {
	in := "title,handle\nShirt,shirt-1\n"
	require.Equal(t, in, applied(t, in, nil))
	require.Equal(t, in, applied(t, in, map[string]string{}))
}

// Resolving a collision by last-write-wins would import the wrong column
// into price without telling anyone.
func TestValidateColumnMapping_RejectsTwoHeadersOnOneField(t *testing.T) {
	err := csvjob.ValidateColumnMapping(map[string]string{
		"Price": "base_price", "Sale Price": "base_price",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "base_price")
}

func TestValidateColumnMapping_RejectsUnknownField(t *testing.T) {
	require.Error(t, csvjob.ValidateColumnMapping(map[string]string{"Foo": "not_a_column"}))
}

func TestValidateColumnMapping_AllowsEveryCanonicalColumn(t *testing.T) {
	for _, c := range csvjob.CanonicalColumns {
		require.NoError(t, csvjob.ValidateColumnMapping(map[string]string{"H": c}), c)
	}
}

// The mapper offering a field the parser does not read would be the same
// class of defect as the mapper not being sent at all.
func TestCanonicalColumns_MatchWhatTheParserReads(t *testing.T) {
	headers := csvjob.CanonicalColumns
	row := []string{"Shirt", "shirt-1", "desc", "active", "10.00", "S1", "5", "1kg", "tops"}
	require.Len(t, row, len(headers), "update this row when a column is added")

	draft, err := csvjob.ParseRow(headers, row, 1, csvjob.NewHandleTracker())
	require.NoError(t, err)
	require.Equal(t, "Shirt", draft.Title)
	require.Equal(t, "shirt-1", draft.Handle)
	require.Equal(t, "10.00", draft.BasePrice.StringFixed(2))
	require.Equal(t, "S1", draft.SKU)
	require.Equal(t, 5, draft.Stock)
}
