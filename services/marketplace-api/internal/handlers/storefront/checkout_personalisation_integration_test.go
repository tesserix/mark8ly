//go:build integration

// Tamper tests for #967.
//
// In the INTERNAL package because the thing worth asserting is
// applyPersonalisation and the resolved shape it produces, both of which
// are unexported by design — exporting them for a test would put the
// checkout's internals on the package's public surface.
//
// The point of this file is NOT that the happy path works. It is that
// each forgery a shopper could attempt fails the checkout rather than
// producing a cheap or a stolen order.
package storefront

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedPersonalisedProduct creates a product with a priced select, a paid
// checkbox, a required text field and an image field.
func seedPersonalisedProduct(t *testing.T, db *gorm.DB, tenantID, storeID string) (productID string, ids map[string]string) {
	t.Helper()
	ids = map[string]string{}
	productID = uuid.NewString()
	// published_at is NOT optional on an active product:
	// products_published_requires_active (migration 000001) rejects
	// status='active' with a null published_at. A sellable product is
	// what these tests are about, so set both rather than dropping to
	// draft.
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id, published_at)
		 VALUES (?, ?, ?, ?, 'Shirt', 'active', ?, now())`,
		productID, tenantID, storeID, "shirt-"+productID[:8], uuid.NewString()).Error)

	mk := func(key, kind string, required bool, extra map[string]any) string {
		id := uuid.NewString()
		cols := "id, tenant_id, store_id, product_id, key, label, kind, required"
		vals := "?, ?, ?, ?, ?, ?, ?, ?"
		args := []any{id, tenantID, storeID, productID, key, key, kind, required}
		for c, v := range extra {
			cols += ", " + c
			vals += ", ?"
			args = append(args, v)
		}
		require.NoError(t, db.Exec(
			"INSERT INTO product_personalisation_fields ("+cols+") VALUES ("+vals+")", args...).Error)
		ids[key] = id
		return id
	}

	finish := mk("finish", "select", false, nil)
	for label, delta := range map[string]string{"matte": "0", "gloss": "2.50"} {
		oid := uuid.NewString()
		require.NoError(t, db.Exec(
			`INSERT INTO product_personalisation_options (id, field_id, value, label, price_delta)
			 VALUES (?, ?, ?, ?, ?)`, oid, finish, label, label, delta).Error)
		ids["option_"+label] = oid
	}
	mk("gift_wrap", "checkbox", false, map[string]any{"price_delta": "4.00"})
	mk("name", "text", true, map[string]any{"max_length": 20})
	mk("photo", "image", false, map[string]any{"max_images": 1})
	return productID, ids
}

func seedVariant(t *testing.T, db *gorm.DB, productID, storeID string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 25.00, 'INR')`,
		id, productID, storeID, "SKU-"+id[:8]).Error)
	return id
}

func seedUpload(t *testing.T, db *gorm.DB, tenantID, storeID, productID, fieldID, cartToken, state string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO personalisation_uploads
		 (id, tenant_id, store_id, product_id, field_id, cart_token,
		  storage_key_original, content_hash, content_type, size_bytes,
		  original_filename, state, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'abc', 'image/jpeg', 100, 'p.jpg', ?, now() + interval '72 hours')`,
		id, tenantID, storeID, productID, fieldID, cartToken,
		"buyer-uploads/"+tenantID+"/"+cartToken+"/"+id+".jpg", state).Error)
	return id
}

type personalisationFixture struct {
	db        *gorm.DB
	tenantID  string
	storeID   string
	productID string
	variantID string
	cartToken string
	ids       map[string]string
}

func newPersonalisationFixture(t *testing.T) personalisationFixture {
	t.Helper()
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	testdb.SeedStore(t, db, uuid.MustParse(tenantID), uuid.MustParse(storeID))
	productID, ids := seedPersonalisedProduct(t, db, tenantID, storeID)
	return personalisationFixture{
		db: db, tenantID: tenantID, storeID: storeID,
		productID: productID,
		variantID: seedVariant(t, db, productID, storeID),
		cartToken: uuid.NewString(),
		ids:       ids,
	}
}

// ─── the money ───────────────────────────────────────────────────────────

func TestIntegration_Personalisation_PricesDeltasFromTheCatalog(t *testing.T) {
	f := newPersonalisationFixture(t)
	items := []CheckoutItemRequest{{
		VariantID: &f.variantID,
		Quantity:  2,
		UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["finish"], OptionID: pStr(f.ids["option_gloss"])},
			{FieldID: f.ids["gift_wrap"], Checked: pBool(true)},
		},
	}}

	resolved, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.NoError(t, err)

	// 25.00 + 2.50 gloss + 4.00 wrap
	require.Equal(t, "31.5", items[0].UnitPrice.String())
	require.Equal(t, "63", items[0].LineTotal.String())
	require.Len(t, resolved[0], 3)
}

// A forged delta in the request must change nothing: the number comes
// from the catalog row, and the request carries no number at all.
func TestIntegration_Personalisation_IgnoresAnyClientSuppliedPrice(t *testing.T) {
	f := newPersonalisationFixture(t)
	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1,
		UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["gift_wrap"], Checked: pBool(true)},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.NoError(t, err)
	require.Equal(t, "29", items[0].UnitPrice.String(), "the catalog's 4.00, not anything sent")
}

// ─── the forgeries ───────────────────────────────────────────────────────

func TestIntegration_Personalisation_RefusesAnotherCartsUpload(t *testing.T) {
	// The one that matters most: an upload id is a bare uuid and there is
	// no authenticated customer, so without the cart-token scope one
	// shopper could attach another's photograph to their own order.
	f := newPersonalisationFixture(t)
	theirs := seedUpload(t, f.db, f.tenantID, f.storeID, f.productID,
		f.ids["photo"], uuid.NewString(), "verified")

	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["photo"], UploadID: &theirs},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no longer available",
		"the message must not distinguish wrong-cart from does-not-exist")
}

func TestIntegration_Personalisation_RefusesAnUnverifiedUpload(t *testing.T) {
	// `pending` means the object may never have arrived. Selling a line
	// against it would hand the merchant an order they cannot fulfil.
	f := newPersonalisationFixture(t)
	pending := seedUpload(t, f.db, f.tenantID, f.storeID, f.productID,
		f.ids["photo"], f.cartToken, "pending")

	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["photo"], UploadID: &pending},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
}

func TestIntegration_Personalisation_RefusesAFieldFromAnotherProduct(t *testing.T) {
	f := newPersonalisationFixture(t)
	otherProduct, otherIDs := seedPersonalisedProduct(t, f.db, f.tenantID, f.storeID)
	_ = otherProduct

	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: otherIDs["name"], Text: pStr("Asha")},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not belong to this product")
}

func TestIntegration_Personalisation_RefusesAnOptionFromAnotherField(t *testing.T) {
	// A stale form or a forged one. Never fall back to a default: that
	// would charge for a choice the buyer did not make.
	f := newPersonalisationFixture(t)
	_, otherIDs := seedPersonalisedProduct(t, f.db, f.tenantID, f.storeID)

	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["finish"], OptionID: pStr(otherIDs["option_gloss"])},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not available")
}

func TestIntegration_Personalisation_RequiredFieldCannotBeOmitted(t *testing.T) {
	// Sending no array at all must not be a way past a required field.
	f := newPersonalisationFixture(t)
	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "required")
}

func TestIntegration_Personalisation_EnforcesTheCharacterLimit(t *testing.T) {
	f := newPersonalisationFixture(t)
	long := "this name is very considerably longer than twenty characters"
	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: &long},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "20 characters")
}

func TestIntegration_Personalisation_EnforcesMaxImages(t *testing.T) {
	f := newPersonalisationFixture(t)
	a := seedUpload(t, f.db, f.tenantID, f.storeID, f.productID, f.ids["photo"], f.cartToken, "verified")
	b := seedUpload(t, f.db, f.tenantID, f.storeID, f.productID, f.ids["photo"], f.cartToken, "verified")

	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["photo"], UploadID: &a},
			{FieldID: f.ids["photo"], UploadID: &b},
		},
	}}
	_, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 1")
}

// The select snapshot records the LABEL the buyer read, not the machine
// value, because that is what the merchant has to produce.
func TestIntegration_Personalisation_SnapshotsTheLabelNotTheValue(t *testing.T) {
	f := newPersonalisationFixture(t)
	items := []CheckoutItemRequest{{
		VariantID: &f.variantID, Quantity: 1, UnitPrice: decimal.RequireFromString("25.00"),
		Personalisation: []CheckoutPersonalisationRequest{
			{FieldID: f.ids["name"], Text: pStr("Asha")},
			{FieldID: f.ids["finish"], OptionID: pStr(f.ids["option_gloss"])},
		},
	}}
	resolved, err := applyPersonalisation(
		context.Background(), personalisationResolver{db: f.db}, f.storeID, f.cartToken, items)
	require.NoError(t, err)

	var finish *resolvedPersonalisation
	for i := range resolved[0] {
		if resolved[0][i].Kind == "select" {
			finish = &resolved[0][i]
		}
	}
	require.NotNil(t, finish)
	require.NotNil(t, finish.TextValue)
	require.Equal(t, "gloss", *finish.TextValue)
	require.Equal(t, "2.5", finish.PriceDelta.String())
}

func pStr(s string) *string { return &s }
func pBool(b bool) *bool    { return &b }
