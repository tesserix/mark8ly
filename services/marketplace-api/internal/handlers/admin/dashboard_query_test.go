package admin

import (
	"regexp"
	"strings"
	"testing"
)

// These are white-box (package admin) regression tests for the two raw SQL
// queries fixed in dashboard.go: lowStockQuery (C1 — pv.title does not
// exist, so the query 42703'd and the discarded error made low_stock
// silently serialize as []) and topProductsQuery (I6 — the image subselect
// was missing the pm.id tie-break its two siblings already have).
//
// There is no DB test harness for this handler (see dashboard.go's doc
// comment), so these tests cannot prove the SQL executes against Postgres.
// What they CAN do — and do — is pin the exact column references in the
// query text, so a column rename/drop in a future migration fails a fast
// unit test instead of shipping a query that 500s or silently drops rows
// in production. Read them as a schema-drift tripwire, not a substitute
// for integration coverage.

// knownColumns mirrors the CREATE TABLE / ALTER TABLE statements in
// migrations/000001_products_initial.up.sql and
// migrations/000081_variant_dimensions.up.sql for every table the two
// queries touch. Keep in sync with those migrations.
var knownColumns = map[string]map[string]bool{
	// product_variants: migrations/000001 (base) + 000081 (+length/width/height_cm)
	"pv": set(
		"id", "product_id", "store_id", "sku", "barcode", "price",
		"compare_at_price", "cost_price", "currency_code", "weight_grams",
		"length_cm", "width_cm", "height_cm", "inventory_quantity",
		"inventory_policy", "low_stock_threshold", "position",
		"created_at", "updated_at", "deleted_at",
	),
	// products
	"p": set(
		"id", "tenant_id", "store_id", "handle", "title", "description",
		"status", "vendor_id", "tags", "seo_title", "seo_description",
		"primary_category_id", "copy_source_product_id", "published_at",
		"created_by", "updated_by", "created_at", "updated_at", "deleted_at",
	),
	// product_media
	"pm": set(
		"id", "product_id", "variant_id", "url", "storage_key", "alt",
		"position", "media_type", "width", "height", "bytes", "created_at",
	),
	// variant_option_values
	"vov": set("variant_id", "option_value_id"),
	// product_option_values (aliased pov in lowStockQuery)
	"pov": set("id", "option_id", "value", "position", "created_at"),
	// product_options (aliased po in lowStockQuery)
	"po": set("id", "product_id", "name", "position", "created_at"),
	// order_items / orders — referenced by topProductsQuery; listed for
	// completeness even though this bug was scoped to product_variants.
	"oi": set(
		"id", "order_id", "product_id", "variant_id", "image_url",
		"line_total", "quantity", "created_at",
	),
	"o": set("id", "store_id", "tenant_id", "status"),
}

func set(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

// aliasColumnRefRe matches `alias.column` tokens like `pv.sku` or `p.title`.
var aliasColumnRefRe = regexp.MustCompile(`\b([a-z]+)\.([a-z_]+)\b`)

// assertOnlyKnownColumns fails the test if the query references any
// alias.column pair not present in knownColumns. Aliases the query
// introduces that aren't schema references (none currently) would need
// adding to knownColumns to pass; that's intentional friction — it forces
// whoever adds a new column reference to confirm it against a migration.
func assertOnlyKnownColumns(t *testing.T, name, query string) {
	t.Helper()
	for _, m := range aliasColumnRefRe.FindAllStringSubmatch(query, -1) {
		alias, col := m[1], m[2]
		cols, known := knownColumns[alias]
		if !known {
			// Unrecognised alias — not necessarily wrong (could be a SQL
			// keyword combo the regex over-matched), but flag it so a
			// human confirms rather than silently skipping it.
			t.Errorf("%s: alias %q (from %q) not in knownColumns — add it or confirm this isn't a real column ref", name, alias, m[0])
			continue
		}
		if !cols[col] {
			t.Errorf("%s: %s.%s does not exist on the corresponding table per migrations/000001 + 000081 — this is exactly the pv.title bug (C1)", name, alias, col)
		}
	}
}

func TestLowStockQuery_ReferencesOnlyRealColumns(t *testing.T) {
	assertOnlyKnownColumns(t, "lowStockQuery", lowStockQuery)
}

func TestTopProductsQuery_ReferencesOnlyRealColumns(t *testing.T) {
	assertOnlyKnownColumns(t, "topProductsQuery", topProductsQuery)
}

// TestLowStockQuery_DoesNotReferenceTitleColumn is a direct regression test
// for C1: pv.title was selected and product_variants has no title column,
// so Postgres raised 42703 on every call and the discarded .Scan error let
// low_stock silently serialize as [] with a 200 response.
func TestLowStockQuery_DoesNotReferenceTitleColumn(t *testing.T) {
	if strings.Contains(lowStockQuery, "pv.title") {
		t.Fatal("lowStockQuery selects pv.title, which does not exist on product_variants (see migrations/000001_products_initial.up.sql:148-175)")
	}
}

// TestLowStockQuery_FiltersSoftDeletedRows guards against deleted variants
// or products reappearing in low_stock once the query actually returns rows.
func TestLowStockQuery_FiltersSoftDeletedRows(t *testing.T) {
	if !strings.Contains(lowStockQuery, "pv.deleted_at IS NULL") {
		t.Error("lowStockQuery does not filter pv.deleted_at IS NULL")
	}
	if !strings.Contains(lowStockQuery, "p.deleted_at IS NULL") {
		t.Error("lowStockQuery does not filter p.deleted_at IS NULL")
	}
}

// TestTopProductsQuery_ImageSubselectTieBreaksOnMediaID is the regression
// test for I6: the image subselect ORDER BY pm.position alone is
// non-deterministic because product_media_product_idx (product_id,
// position) is a plain, non-unique index — duplicate positions are legal.
func TestTopProductsQuery_ImageSubselectTieBreaksOnMediaID(t *testing.T) {
	if !strings.Contains(topProductsQuery, "ORDER BY pm.position, pm.id") {
		t.Fatal("topProductsQuery image subselect is missing the pm.id tie-break (see the recent-orders and low-stock siblings for the pattern)")
	}
}
