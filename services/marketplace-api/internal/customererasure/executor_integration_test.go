//go:build integration

// In the INTERNAL package, like coverage_integration_test.go: the failure
// path is asserted by calling markFailed directly, which is the only way to
// pin context.WithoutCancel deterministically.
package customererasure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// customer is one seeded subject and everything of theirs the erasure must
// reach. The second customer in every test is seeded with the SAME shape in
// the SAME store, so "untouched" is a claim about identical data, not about
// data the plan would never have matched anyway.
type customer struct {
	email     string
	name      string
	profileID uuid.UUID
	addressID uuid.UUID
	wishlist  uuid.UUID
	reviewID  uuid.UUID
	mediaID   uuid.UUID
	orderID   uuid.UUID
	emailSend uuid.UUID
	// The JSONB half (#435): each of these rows SURVIVES the erasure with
	// its non-personal fields intact, and loses only the named keys.
	shipmentID uuid.UUID
	auditID    uuid.UUID
	cartID     uuid.UUID
	outboxID   uuid.UUID
}

type fixture struct {
	db       *gorm.DB
	tenantID uuid.UUID
	storeID  uuid.UUID
	subject  customer
	// other is a DIFFERENT person in the SAME store. They catch a statement
	// that lost its email predicate.
	other customer
	// twin is the SAME address in a DIFFERENT store — a real case, since one
	// person shops at several merchants and files a request per store. They
	// catch a statement that lost its store predicate, which `other` cannot:
	// dropping `AND store_id = ?` while keeping the email still misses a
	// bystander whose address differs.
	twin        customer
	twinStoreID uuid.UUID
	request     uuid.UUID
}

const (
	subjectAddr = "erasure-subject@example.test"
	otherAddr   = "bystander@example.test"
	orderTotal  = "42.50"
	orderCcy    = "EUR"
	addrCountry = "IE"
	// cartRecoveryURL embeds a per-customer token, so it is personal data in
	// its own right: anyone holding it can reopen that person's cart.
	cartRecoveryURLPrefix = "https://shop.test/recover/"
	auditAction           = "customer.updated"
)

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testdb.NewTx(t)

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	vendorID := testdb.SeedVendor(t, db, tenantID)

	productID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id)
		 VALUES (?, ?, ?, ?, ?, 'draft', ?)`,
		productID, tenantID, storeID, "p-"+productID.String()[:8], "Test product", vendorID,
	).Error)

	f := fixture{db: db, tenantID: tenantID, storeID: storeID}
	f.subject = seedCustomer(t, db, tenantID, storeID, productID, subjectAddr, "Subject Person")
	f.other = seedCustomer(t, db, tenantID, storeID, productID, otherAddr, "Bystander Person")

	// A second merchant entirely, where the SAME address shops. The erasure
	// request names one store; this person's data at every other store is
	// none of its business.
	twinTenantID, twinStoreID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, twinTenantID, twinStoreID)
	twinVendorID := testdb.SeedVendor(t, db, twinTenantID)
	twinProductID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id)
		 VALUES (?, ?, ?, ?, ?, 'draft', ?)`,
		twinProductID, twinTenantID, twinStoreID, "p-"+twinProductID.String()[:8], "Other store product", twinVendorID,
	).Error)
	f.twinStoreID = twinStoreID
	f.twin = seedCustomer(t, db, twinTenantID, twinStoreID, twinProductID, subjectAddr, "Subject Person")

	f.request = uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO customer_erasure_requests (id, tenant_id, store_id, customer_email)
		 VALUES (?, ?, ?, ?)`,
		f.request, tenantID, storeID, subjectAddr,
	).Error)

	return f
}

func seedCustomer(t *testing.T, db *gorm.DB, tenantID, storeID, productID uuid.UUID, email, name string) customer {
	t.Helper()
	c := customer{
		email: email, name: name,
		profileID: uuid.New(), addressID: uuid.New(), wishlist: uuid.New(),
		reviewID: uuid.New(), mediaID: uuid.New(), orderID: uuid.New(), emailSend: uuid.New(),
		shipmentID: uuid.New(), auditID: uuid.New(), cartID: uuid.New(), outboxID: uuid.New(),
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		require.NoError(t, db.Exec(sql, args...).Error, "seeding %s", email)
	}

	exec(`INSERT INTO customer_profiles (id, tenant_id, store_id, email, first_name, last_name, phone)
	      VALUES (?, ?, ?, ?, ?, 'Person', '+353100000000')`,
		c.profileID, tenantID, storeID, email, name)

	exec(`INSERT INTO customer_addresses (id, tenant_id, customer_id, name, line1, city, country_code)
	      VALUES (?, ?, ?, ?, '1 Test Lane', 'Dublin', ?)`,
		c.addressID, tenantID, c.profileID, name, addrCountry)

	exec(`INSERT INTO wishlists (id, tenant_id, store_id, customer_id, product_id)
	      VALUES (?, ?, ?, ?, ?)`,
		c.wishlist, tenantID, storeID, c.profileID, productID)

	exec(`INSERT INTO reviews (id, tenant_id, store_id, product_id, customer_profile_id,
	                           customer_name, customer_email, rating, content, status)
	      VALUES (?, ?, ?, ?, ?, ?, ?, 5, 'Great product', 'published')`,
		c.reviewID, tenantID, storeID, productID, c.profileID, name, email)

	exec(`INSERT INTO review_media (id, review_id, url) VALUES (?, ?, 'https://cdn.test/photo.jpg')`,
		c.mediaID, c.reviewID)

	exec(`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_id,
	                          customer_email, customer_name, subtotal, grand_total, currency_code)
	      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.orderID, tenantID, storeID, "ORD-"+c.orderID.String()[:8], c.orderID.String(),
		c.profileID, email, name, orderTotal, orderTotal, orderCcy)

	exec(`INSERT INTO order_addresses (order_id, kind, name, line1, line2, city, region, postal_code, country_code, phone)
	      VALUES (?, 'shipping', ?, '1 Test Lane', 'Apt 2', 'Dublin', 'Leinster', 'D01 X1X1', ?, '+353100000000')`,
		c.orderID, name, addrCountry)

	exec(`INSERT INTO payment_transactions (tenant_id, store_id, order_id, provider, amount, currency_code, status)
	      VALUES (?, ?, ?, 'stripe', ?, ?, 'succeeded')`,
		tenantID, storeID, c.orderID, orderTotal, orderCcy)

	exec(`INSERT INTO email_sends (id, tenant_id, store_id, recipient, kind, status)
	      VALUES (?, ?, ?, ?, 'order_confirmation', 'sent')`,
		c.emailSend, tenantID, storeID, email)

	// ---- the JSONB blobs (#435) ------------------------------------------
	// A shipment whose ship_to is the customer's delivery address. ship_from
	// is seeded as the customer's address too, which is exactly what
	// shipmentcancel writes onto a reverse leg — the case that makes
	// stripping BOTH columns necessary rather than fussy.
	exec(`INSERT INTO shipments (id, tenant_id, store_id, order_id, carrier, tracking_number,
	                             status, ship_from, ship_to, currency_code)
	      VALUES (?, ?, ?, ?, 'delhivery', ?, 'pending', ?::jsonb, ?::jsonb, ?)`,
		c.shipmentID, tenantID, storeID, c.orderID, "TRK-"+c.shipmentID.String()[:8],
		shipmentAddressJSON(name), shipmentAddressJSON(name), orderCcy)

	// An audit row whose metadata names the customer. store_id is set, and
	// `action` is the structural field that must survive the strip.
	exec(`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, resource_id, metadata)
	      VALUES (?, ?, ?, 'user', ?, 'customer', ?, ?::jsonb)`,
		c.auditID, tenantID, storeID, auditAction, c.profileID.String(), auditMetadataJSON(email))

	exec(`INSERT INTO abandoned_carts (id, tenant_id, store_id, cart_session_id, customer_email,
	                                   customer_name, item_count, subtotal, currency_code,
	                                   items_snapshot, recovery_url, last_active_at)
	      VALUES (?, ?, ?, ?, ?, ?, 2, ?, ?, '[]'::jsonb, ?, now())`,
		c.cartID, tenantID, storeID, c.cartID.String(), email, name,
		orderTotal, orderCcy, cartRecoveryURLPrefix+c.cartID.String())

	exec(`INSERT INTO outbox_events (id, tenant_id, aggregate, aggregate_id, event_type, payload)
	      VALUES (?, ?, 'abandoned_cart', ?, 'abandoned_cart.recovery_email', ?::jsonb)`,
		c.outboxID, tenantID, c.cartID, outboxPayloadJSON(storeID, c.cartID, email))

	return c
}

// shipmentAddressJSON is the eight-key blob handlers/admin/shipments.go:694
// writes into ship_to / ship_from.
func shipmentAddressJSON(name string) string {
	b, _ := json.Marshal(map[string]any{
		"name": name, "line1": "1 Test Lane", "line2": "Apt 2", "city": "Dublin",
		"region": "Leinster", "postal_code": "D01 X1X1", "country_code": addrCountry,
		"phone": "+353100000000",
	})
	return string(b)
}

// auditMetadataJSON carries the customer under ALL SIX stripped key names at
// once, plus a structural key that must survive. One row exercising every
// name is stronger than six rows each exercising one: a step that strips only
// the first key still fails here.
func auditMetadataJSON(email string) string {
	b, _ := json.Marshal(map[string]any{
		"customer_email":  email,
		"email":           email,
		"recipient_email": email,
		"submitter_email": email,
		"author_email":    email,
		"actor_email":     email,
		"source":          "storefront",
	})
	return string(b)
}

// outboxPayloadJSON is what order/abandoned_cart_service.go:128 enqueues.
// store_id and item_count are the routing/reporting fields that must survive
// — outbox/publisher.go:112 fails a batch whose payload has no store_id.
func outboxPayloadJSON(storeID, cartID uuid.UUID, email string) string {
	b, _ := json.Marshal(map[string]any{
		"store_id":          storeID.String(),
		"abandoned_cart_id": cartID.String(),
		"customer_email":    email,
		"recovery_url":      cartRecoveryURLPrefix + cartID.String(),
		"item_count":        2,
		"currency":          orderCcy,
	})
	return string(b)
}

func newExecutor(t *testing.T, db *gorm.DB) *Executor {
	t.Helper()
	e, err := NewExecutor(db, nil)
	require.NoError(t, err)
	return e
}

func count(t *testing.T, db *gorm.DB, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(sql, args...).Scan(&n).Error)
	return n
}

// ---------------------------------------------------------------------------

func TestNewExecutor_RefusesNilDB(t *testing.T) {
	_, err := NewExecutor(nil, nil)
	require.Error(t, err, "a nil db must be refused at construction, not panicked on at erasure time (#318)")
}

// TestProcess_DeletesWhatServesOnlyTheCustomer covers the DELETE half.
func TestProcess_DeletesWhatServesOnlyTheCustomer(t *testing.T) {
	f := newFixture(t)
	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)

	require.Zero(t, count(t, f.db, `SELECT count(*) FROM customer_profiles WHERE id = ?`, f.subject.profileID))
	require.Zero(t, count(t, f.db, `SELECT count(*) FROM customer_addresses WHERE id = ?`, f.subject.addressID))
	require.Zero(t, count(t, f.db, `SELECT count(*) FROM wishlists WHERE id = ?`, f.subject.wishlist))
	require.Zero(t, count(t, f.db, `SELECT count(*) FROM review_media WHERE id = ?`, f.subject.mediaID))
	require.Zero(t, count(t, f.db, `SELECT count(*) FROM email_sends WHERE id = ?`, f.subject.emailSend))
}

// TestProcess_KeepsTheFinancialRecordAndAnonymisesIt is the ANONYMISE half:
// the money must still be reconcilable after the person is gone.
func TestProcess_KeepsTheFinancialRecordAndAnonymisesIt(t *testing.T) {
	f := newFixture(t)
	token := Token(f.request)

	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)

	var order struct {
		CustomerEmail string
		CustomerName  *string
		CustomerID    *uuid.UUID
		GrandTotal    string
		CurrencyCode  string
	}
	require.NoError(t, f.db.Raw(
		`SELECT customer_email, customer_name, customer_id, grand_total::text AS grand_total, currency_code
		   FROM orders WHERE id = ?`, f.subject.orderID).Scan(&order).Error)

	require.Equal(t, token, order.CustomerEmail, "the order must survive with the token in place of the address")
	require.NotNil(t, order.CustomerName)
	require.Equal(t, RedactedName, *order.CustomerName)
	require.Nil(t, order.CustomerID, "the link back to the deleted profile must be severed")
	require.Equal(t, orderTotal, order.GrandTotal, "the financial figures must be untouched")
	require.Equal(t, orderCcy, order.CurrencyCode)

	// The payment row is retained whole: it holds no personal column at all.
	require.EqualValues(t, 1,
		count(t, f.db, `SELECT count(*) FROM payment_transactions WHERE order_id = ?`, f.subject.orderID),
		"payment_transactions is a financial record and carries no step; it must survive")

	var addr struct {
		Name, Line1, City, CountryCode string
		Line2, Region, PostalCode      *string
		Phone                          *string
	}
	require.NoError(t, f.db.Raw(
		`SELECT name, line1, city, country_code, line2, region, postal_code, phone
		   FROM order_addresses WHERE order_id = ?`, f.subject.orderID).Scan(&addr).Error)
	require.Equal(t, RedactedName, addr.Name)
	require.Equal(t, RedactedLine, addr.Line1)
	require.Equal(t, RedactedLine, addr.City)
	require.Nil(t, addr.Line2)
	require.Nil(t, addr.Region)
	require.Nil(t, addr.PostalCode)
	require.Nil(t, addr.Phone)
	require.Equal(t, addrCountry, strings.TrimSpace(addr.CountryCode),
		"country_code is kept deliberately: tax reporting needs it and it identifies nobody alone")
}

// TestProcess_KeepsTheReviewAndItsRating — deleting the review would
// retroactively change the merchant's historical star rating.
func TestProcess_KeepsTheReviewAndItsRating(t *testing.T) {
	f := newFixture(t)
	token := Token(f.request)

	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)

	var review struct {
		CustomerEmail     string
		CustomerName      string
		Rating            int
		CustomerProfileID *uuid.UUID
	}
	require.NoError(t, f.db.Raw(
		`SELECT customer_email, customer_name, rating, customer_profile_id
		   FROM reviews WHERE id = ?`, f.subject.reviewID).Scan(&review).Error)

	require.Equal(t, token, review.CustomerEmail)
	require.Equal(t, RedactedName, review.CustomerName)
	require.Equal(t, 5, review.Rating, "the rating carries the merchant's reputation and must not move")
	require.Nil(t, review.CustomerProfileID)
}

// TestProcess_LeavesEveryOtherCustomerUntouched is THE test.
//
// An unscoped WHERE — a missing `AND store_id = ?`, a subquery that forgot
// the email — would erase a different person who never asked for it. That is
// the worst thing this feature can do, and it is silent: every other
// assertion in this file would still pass.
//
// It takes BOTH bystanders to pin both predicates. A different person in the
// same store catches a dropped email; the same person at a different store
// catches a dropped store_id. Either one alone leaves half the mutation
// space green.
func TestProcess_LeavesEveryOtherCustomerUntouched(t *testing.T) {
	f := newFixture(t)
	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)

	assertCustomerIntact(t, f.db, f.other)
	assertCustomerIntact(t, f.db, f.twin)
}

// assertCustomerIntact asserts EVERY row seeded for c is exactly as seeded —
// not merely present, but still carrying the person's own name and address.
// An anonymised-but-present row is as much a wrongful erasure as a deleted
// one, and a bare count(*) would not notice.
func assertCustomerIntact(t *testing.T, db *gorm.DB, c customer) {
	t.Helper()
	require.EqualValues(t, 1, count(t, db, `SELECT count(*) FROM customer_profiles WHERE id = ? AND email = ?`, c.profileID, c.email))
	require.EqualValues(t, 1, count(t, db, `SELECT count(*) FROM customer_addresses WHERE id = ? AND name = ?`, c.addressID, c.name))
	require.EqualValues(t, 1, count(t, db, `SELECT count(*) FROM wishlists WHERE id = ?`, c.wishlist))
	require.EqualValues(t, 1, count(t, db, `SELECT count(*) FROM review_media WHERE id = ?`, c.mediaID))
	require.EqualValues(t, 1, count(t, db, `SELECT count(*) FROM email_sends WHERE id = ? AND recipient = ?`, c.emailSend, c.email))
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM reviews WHERE id = ? AND customer_email = ? AND customer_name = ? AND customer_profile_id = ?`,
		c.reviewID, c.email, c.name, c.profileID))
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM orders WHERE id = ? AND customer_email = ? AND customer_name = ? AND customer_id = ?`,
		c.orderID, c.email, c.name, c.profileID))
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM order_addresses WHERE order_id = ? AND name = ? AND line1 = '1 Test Lane' AND city = 'Dublin'`,
		c.orderID, c.name))

	// The JSONB blobs (#435). Asserted key-by-key, not by row count: a strip
	// that lost its predicate leaves the row present and empty, and count(*)
	// would call that intact.
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM shipments
		  WHERE id = ? AND ship_to ->> 'name' = ? AND ship_from ->> 'name' = ?
		    AND ship_to ->> 'phone' = '+353100000000'`,
		c.shipmentID, c.name, c.name))
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM audit_logs
		  WHERE id = ? AND metadata ->> 'customer_email' = ? AND metadata ->> 'actor_email' = ?`,
		c.auditID, c.email, c.email))
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM abandoned_carts WHERE id = ? AND customer_email = ?`,
		c.cartID, c.email))
	require.EqualValues(t, 1, count(t, db,
		`SELECT count(*) FROM outbox_events
		  WHERE id = ? AND payload ->> 'customer_email' = ? AND payload ->> 'recovery_url' <> ''`,
		c.outboxID, c.email))
}

// TestProcess_StripsPersonalKeysFromJSONBBlobsAndKeepsTheRest is the #435
// test.
//
// Three blobs outlive every other copy of the subject's address:
// shipments.ship_to/ship_from, audit_logs.metadata (retained FOREVER on a Pro
// plan — audit/prune_cron.go:42) and outbox_events.payload. Each row must
// survive with its operational fields byte-identical and lose only the keys
// that name the person. A step that emptied the whole blob would pass a
// "the PII is gone" assertion and destroy a governance record; that is why
// every assertion below has a matching "and this survived".
func TestProcess_StripsPersonalKeysFromJSONBBlobsAndKeepsTheRest(t *testing.T) {
	f := newFixture(t)
	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)

	// ---- shipments -------------------------------------------------------
	var ship struct {
		ShipTo, ShipFrom, Carrier, TrackingNumber, Status string
	}
	require.NoError(t, f.db.Raw(
		`SELECT ship_to::text AS ship_to, ship_from::text AS ship_from,
		        carrier, tracking_number, status
		   FROM shipments WHERE id = ?`, f.subject.shipmentID).Scan(&ship).Error)

	require.Equal(t, "{}", ship.ShipTo, "ship_to is the customer's delivery address and must be emptied")
	require.Equal(t, "{}", ship.ShipFrom,
		"ship_from carries the customer on a reverse leg (shipmentcancel/executor.go:254) and must be emptied too")
	require.Equal(t, "delhivery", ship.Carrier, "the shipment itself must survive: it is the carrier record")
	require.NotEmpty(t, ship.TrackingNumber, "the waybill is not personal data and must not be destroyed")
	require.Equal(t, "pending", ship.Status)

	// ---- audit_logs ------------------------------------------------------
	var audit struct {
		Metadata, Action, ResourceType string
	}
	require.NoError(t, f.db.Raw(
		`SELECT metadata::text AS metadata, action, resource_type
		   FROM audit_logs WHERE id = ?`, f.subject.auditID).Scan(&audit).Error)

	require.Equal(t, auditAction, audit.Action, "the governance record must survive the strip")
	require.Equal(t, "customer", audit.ResourceType)
	require.NotContains(t, audit.Metadata, subjectAddr,
		"no stripped key may leave the subject's address behind in the metadata")
	for _, key := range []string{
		"customer_email", "\"email\"", "recipient_email",
		"submitter_email", "author_email", "actor_email",
	} {
		require.NotContains(t, audit.Metadata, key, "metadata still carries the %s key", key)
	}
	require.Contains(t, audit.Metadata, "storefront",
		"the strip must remove named keys only — the rest of the object is the audit record")

	// ---- outbox_events ---------------------------------------------------
	var evt struct {
		Payload, Aggregate, EventType string
	}
	require.NoError(t, f.db.Raw(
		`SELECT payload::text AS payload, aggregate, event_type
		   FROM outbox_events WHERE id = ?`, f.subject.outboxID).Scan(&evt).Error)

	require.Equal(t, "abandoned_cart", evt.Aggregate, "the event row must survive; only its payload keys go")
	require.Equal(t, "abandoned_cart.recovery_email", evt.EventType)
	require.NotContains(t, evt.Payload, subjectAddr)
	require.NotContains(t, evt.Payload, "customer_email")
	require.NotContains(t, evt.Payload, "recovery_url",
		"the recovery URL is a bearer token for the person's cart")
	require.Contains(t, evt.Payload, f.storeID.String(),
		"store_id must survive: outbox/publisher.go:112 fails a batch whose payload has no store_id")
	require.Contains(t, evt.Payload, "item_count")

	// The cart itself is deleted outright, taking items_snapshot with it.
	require.Zero(t, count(t, f.db, `SELECT count(*) FROM abandoned_carts WHERE id = ?`, f.subject.cartID))
}

// TestProcess_WritesAReceiptCarryingNoPersonalData. The receipt is the
// evidence, and evidence that names the subject re-creates the record the
// erasure removed.
func TestProcess_WritesAReceiptCarryingNoPersonalData(t *testing.T) {
	f := newFixture(t)

	receipt, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.NoError(t, err)
	require.Equal(t, f.request, receipt.RequestID)
	require.EqualValues(t, 1, receipt.Deleted["customer_profiles"])
	require.EqualValues(t, 1, receipt.Anonymised["orders"])
	require.NotContains(t, receipt.Deleted, "orders",
		"orders is a financial record: the receipt must record it as retained-and-anonymised, never as destroyed")
	require.Contains(t, receipt.RetainedTables, "orders")
	require.Contains(t, receipt.RetainedTables, "payment_transactions",
		"a table retained WITHOUT a step must still be named, or a reader cannot tell it was considered")
	require.NotEmpty(t, receipt.RetentionBasis)

	var row struct {
		Status      string
		Notes       *string
		ProcessedAt *string
	}
	require.NoError(t, f.db.Raw(
		`SELECT status, notes, processed_at::text AS processed_at
		   FROM customer_erasure_requests WHERE id = ?`, f.request).Scan(&row).Error)

	require.Equal(t, StatusCompleted, row.Status)
	require.NotNil(t, row.ProcessedAt)
	require.NotNil(t, row.Notes)
	require.NotEmpty(t, *row.Notes)

	require.NotContains(t, *row.Notes, subjectAddr, "the receipt must never carry the subject's email")
	require.NotContains(t, *row.Notes, f.subject.name, "the receipt must never carry the subject's name")
	require.NotContains(t, *row.Notes, "+353100000000", "the receipt must never carry the subject's phone")
	require.NotContains(t, *row.Notes, "1 Test Lane", "the receipt must never carry the subject's address")

	var stored Receipt
	require.NoError(t, json.Unmarshal([]byte(*row.Notes), &stored),
		"notes must be the machine-readable receipt, not prose")
	require.Equal(t, receipt.Deleted, stored.Deleted)
}

// TestProcess_IsIdempotent — a retry after a timeout must not read as
// "nothing was there", and must not run a second destructive pass.
func TestProcess_IsIdempotent(t *testing.T) {
	f := newFixture(t)
	e := newExecutor(t, f.db)

	first, err := e.Process(context.Background(), f.request)
	require.NoError(t, err)

	beforeAttempts := count(t, f.db, `SELECT attempts FROM customer_erasure_requests WHERE id = ?`, f.request)

	second, err := e.Process(context.Background(), f.request)
	require.NoError(t, err, "re-processing a completed request must not error")
	require.Equal(t, first.Deleted, second.Deleted, "the replay must report the ORIGINAL counts, not a second pass's zeroes")
	require.Equal(t, first.Anonymised, second.Anonymised)

	require.EqualValues(t, beforeAttempts,
		count(t, f.db, `SELECT attempts FROM customer_erasure_requests WHERE id = ?`, f.request),
		"a replay must not consume an attempt")
	require.Equal(t, StatusCompleted, statusOf(t, f.db, f.request))
	require.Zero(t, count(t, f.db, `SELECT count(*) FROM customer_profiles WHERE id = ?`, f.subject.profileID))
}

// TestProcess_RefusesARequestAnotherWorkerHolds. The claim is the
// concurrency control; a second worker must bounce off it, not run in
// parallel and double-decrement.
func TestProcess_RefusesARequestAnotherWorkerHolds(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Exec(
		`UPDATE customer_erasure_requests SET status = ? WHERE id = ?`, StatusProcessing, f.request).Error)

	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.ErrorIs(t, err, ErrAlreadyClaimed)
	require.EqualValues(t, 1, count(t, f.db, `SELECT count(*) FROM customer_profiles WHERE id = ?`, f.subject.profileID),
		"a worker that lost the claim must not have erased anything")
}

func TestProcess_UnknownRequestIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := newExecutor(t, f.db).Process(context.Background(), uuid.New())
	require.ErrorIs(t, err, ErrRequestNotFound)
}

// TestProcess_RecordsFailureAfterTheTransactionRollsBack is the #397 guard.
//
// A BEFORE UPDATE trigger on orders — the LAST anonymise step — makes the
// erasure transaction fail once everything before it has run. Everything in
// that transaction rolls back. The 'failed' status must NOT: written inside
// it, it would roll back too, leaving the row stuck in 'processing' with no
// record of why, which is precisely the defect #397 fixed elsewhere.
func TestProcess_RecordsFailureAfterTheTransactionRollsBack(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Exec(`
		CREATE OR REPLACE FUNCTION pg_temp.erasure_forced_failure() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'forced failure for the erasure failure-path test'; END;
		$$ LANGUAGE plpgsql`).Error)
	require.NoError(t, f.db.Exec(`
		CREATE TRIGGER erasure_forced_failure BEFORE UPDATE ON orders
		FOR EACH ROW EXECUTE FUNCTION pg_temp.erasure_forced_failure()`).Error)

	_, err := newExecutor(t, f.db).Process(context.Background(), f.request)
	require.Error(t, err)

	var se *StepError
	require.ErrorAs(t, err, &se, "a failed statement must report which table and disposition failed")
	require.Equal(t, "orders", se.Table)
	require.Equal(t, DispositionAnonymise, se.Disposition)

	require.Equal(t, StatusFailed, statusOf(t, f.db, f.request),
		"the failed status must survive the rolled-back transaction — write it AFTER, on the pooled handle (#397)")

	// Everything the transaction did is gone: the erasure is all-or-nothing.
	require.EqualValues(t, 1, count(t, f.db, `SELECT count(*) FROM customer_profiles WHERE id = ?`, f.subject.profileID))
	require.EqualValues(t, 1, count(t, f.db, `SELECT count(*) FROM wishlists WHERE id = ?`, f.subject.wishlist))

	var notes *string
	require.NoError(t, f.db.Raw(`SELECT notes FROM customer_erasure_requests WHERE id = ?`, f.request).Scan(&notes).Error)
	require.NotNil(t, notes)
	require.Contains(t, *notes, "orders")
	require.NotContains(t, *notes, subjectAddr,
		"a Postgres error message embeds the offending value; the failure note must carry the SQLSTATE, never the driver text")

	// Claimable again: 'failed' is a retryable state, and the attempt was counted.
	require.EqualValues(t, 1, count(t, f.db, `SELECT attempts FROM customer_erasure_requests WHERE id = ?`, f.request))
}

// TestMarkFailed_WritesEvenWhenTheCallerContextIsCancelled pins
// context.WithoutCancel. The operator who triggered an erasure has very often
// closed the tab by the time it fails; a cancelled context must not be able
// to drop the only record that the attempt happened.
func TestMarkFailed_WritesEvenWhenTheCallerContextIsCancelled(t *testing.T) {
	f := newFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	newExecutor(t, f.db).markFailed(ctx, Request{ID: f.request, StoreID: f.storeID, Attempts: 1},
		errors.New("customererasure: synthetic failure"))

	require.Equal(t, StatusFailed, statusOf(t, f.db, f.request),
		"a cancelled caller context must not lose the failure record")
}

// TestReject_IsTerminalAndDestroysNothing.
func TestReject_IsTerminalAndDestroysNothing(t *testing.T) {
	f := newFixture(t)
	e := newExecutor(t, f.db)

	req, err := e.Reject(context.Background(), f.request, "identity could not be verified")
	require.NoError(t, err)
	require.Equal(t, StatusRejected, req.Status)
	require.Equal(t, StatusRejected, statusOf(t, f.db, f.request))

	require.EqualValues(t, 1, count(t, f.db, `SELECT count(*) FROM customer_profiles WHERE id = ?`, f.subject.profileID),
		"a rejection must not erase anything")

	_, err = e.Reject(context.Background(), f.request, "second opinion")
	require.ErrorIs(t, err, ErrAlreadyClaimed, "a second decision must not overwrite the first")

	_, err = e.Process(context.Background(), f.request)
	require.ErrorIs(t, err, ErrAlreadyClaimed, "a rejected request must not be processable")
}

func TestPendingIDs_ListsClaimableRequests(t *testing.T) {
	f := newFixture(t)
	ids, err := newExecutor(t, f.db).PendingIDs(context.Background(), 50)
	require.NoError(t, err)
	require.Contains(t, ids, f.request)
}

func statusOf(t *testing.T, db *gorm.DB, requestID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, db.Raw(`SELECT status FROM customer_erasure_requests WHERE id = ?`, requestID).Scan(&status).Error)
	return status
}
