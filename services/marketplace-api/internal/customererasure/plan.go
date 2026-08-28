// Package customererasure executes GDPR art.17 erasure for one customer in
// one store (#259).
//
// Scope is (store_id, email), matching customer_erasure_requests' own
// UNIQUE (store_id, customer_email). A person with accounts in three stores
// files three requests; grouping them is a console presentation concern.
//
// KEYED ON EMAIL, NOT customer_id. orders.customer_id is set only when a
// logged-in profile is in context (handlers/storefront/checkout.go), so guest
// orders carry NULL, while customer_email is NOT NULL. customer_id is nulled
// where present, never used as the primary match.
//
// TWO DISPOSITIONS. A row that exists only to serve the customer is DELETED.
// A row that must survive for financial-record reasons is ANONYMISED: its
// personal fields are overwritten and the row is retained 7 years under
// legal-obligation basis, matching billing_archive (§23.2, migration 000046)
// — the same number #365 chose so the estate has one retention story rather
// than two. Migration 000113 writes that basis onto the tables themselves.
//
// EVERY STATEMENT IS SCOPED TO THE STORE, directly by store_id or through a
// store-scoped subquery, and the subject's email is ALWAYS a bound
// parameter, never interpolated. An unscoped WHERE here would erase a
// different merchant's customers.
//
// ORDER MATTERS in two places, and both are load-bearing:
//   - review_media is deleted while reviews still carry the subject's email;
//     the reviews anonymisation runs later.
//   - orders is the LAST anonymise step, because order_addresses,
//     order_events and returns all locate their rows through
//     `orders WHERE customer_email = <subject>`. Anonymising orders first
//     would orphan them.
//   - customer_profiles is the LAST step overall; groups 1 and 2 reference
//     it by subquery.
//
// TABLES DELIBERATELY CARRYING NO STEP, verified column-by-column against
// the live schema rather than assumed:
//   - payment_transactions, refund_transactions, platform_fee_ledger hold no
//     personal column at all. Their only PII risk is
//     payment_transactions.metadata (JSONB), which is out of scope below.
//   - shipments holds personal data only in ship_from/ship_to (JSONB).
//   - order_items, order_tax_lines, return_items, loyalty_transactions,
//     gift_card_transactions and referrals hold no personal column; they
//     survive attached to rows that have been anonymised.
//
// OUT OF SCOPE, deliberately: nine JSONB columns can embed a customer email
// or address (stripe_webhook_events.payload, payment_transactions.metadata,
// shipments.ship_from/ship_to, abandoned_carts.items_snapshot,
// audit_logs.metadata, outbox_events.payload and others). None are inspected
// here — safely key-stripping nine differently-shaped payloads is its own
// effort and guessing at their shapes risks corrupting payment metadata.
// abandoned_carts is deleted outright so its snapshot goes with it; the rest
// are known residual PII, tracked separately.
package customererasure

import (
	"github.com/google/uuid"
)

// Disposition is what the erasure does to a table: destroy the rows, or keep
// them and overwrite the personal fields.
type Disposition string

const (
	// DispositionDelete — the row exists only to serve the customer.
	DispositionDelete Disposition = "delete"
	// DispositionAnonymise — the row must survive for financial or
	// integrity reasons, but its personal fields must not.
	DispositionAnonymise Disposition = "anonymise"
)

// authorTypeCustomer is the discriminator on the mixed-subject reply tables.
// A merchant's or platform operator's reply on the same thread is NOT the
// subject's personal data and must not be touched. Value verified against
// the live CHECK constraints on review_replies and support_ticket_replies.
const authorTypeCustomer = "customer"

// Step is one parameterised statement in the erasure plan.
type Step struct {
	// Table is the table the statement writes to. Three gift_cards steps
	// share a name because one row can hold the subject in three roles.
	Table string
	// Disposition declares what the statement does. Every step has one.
	Disposition Disposition
	// SQL is a GORM-style statement with `?` placeholders. The subject's
	// email NEVER appears in this string.
	SQL string
	// Args are the positional args for SQL's `?` placeholders, in order.
	Args []any
}

// erasurePlan returns the ordered statements that erase one subject from one
// store. It is PURE — no DB handle, no I/O — so it is unit-testable and the
// schema-coverage guard can assert against it directly.
//
// token replaces the subject's email wherever a row survives; see Token.
func erasurePlan(storeID uuid.UUID, email string, token string) []Step {
	// profileIDs locates the subject's profile row(s) for the tables that
	// key on customer_profiles.id rather than on an email column. It is
	// itself store-scoped, so a table without its own store_id (e.g.
	// customer_addresses) is still confined to this merchant.
	const profileIDs = `SELECT id FROM customer_profiles WHERE store_id = ? AND email = ?`
	// subjectOrders locates the subject's orders. Only usable BEFORE the
	// orders anonymisation step, which is why orders runs last.
	const subjectOrders = `SELECT id FROM orders WHERE store_id = ? AND customer_email = ?`

	steps := []Step{
		// ---- Group 1: DELETE. Rows that exist only to serve the customer.
		// Children before parents.
		{
			Table:       "review_reactions", // 000017
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM review_reactions WHERE customer_profile_id IN (` + profileIDs + `)`,
			Args:        []any{storeID, email},
		},
		{
			// Customer-uploaded photographs. Deleted, not anonymised: a
			// photograph is not aggregate-rating-bearing, and it can show
			// the person.
			Table:       "review_media", // 000017
			Disposition: DispositionDelete,
			SQL: `DELETE FROM review_media WHERE review_id IN (
				SELECT id FROM reviews WHERE store_id = ? AND customer_email = ?)`,
			Args: []any{storeID, email},
		},
		{
			Table:       "wishlists", // 000018
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM wishlists WHERE store_id = ? AND customer_id IN (` + profileIDs + `)`,
			Args:        []any{storeID, storeID, email},
		},
		{
			Table:       "customer_addresses", // 000013
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM customer_addresses WHERE customer_id IN (` + profileIDs + `)`,
			Args:        []any{storeID, email},
		},
		{
			// Deleting the cart takes items_snapshot (JSONB) with it, which
			// is why abandoned_carts needs no blob handling.
			Table:       "abandoned_carts", // 000002
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM abandoned_carts WHERE store_id = ? AND customer_email = ?`,
			Args:        []any{storeID, email},
		},
		{
			// storefront_push_tokens carries store_slug, not store_id, and
			// has no FK to customer_profiles — the profile subquery is what
			// makes this store-scoped.
			Table:       "storefront_push_tokens", // 000022
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM storefront_push_tokens WHERE customer_id IN (` + profileIDs + `)`,
			Args:        []any{storeID, email},
		},
		{
			// Same shape as storefront_push_tokens: store_slug, no FK.
			Table:       "product_notify_subscriptions", // 000023
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM product_notify_subscriptions WHERE customer_id IN (` + profileIDs + `)`,
			Args:        []any{storeID, email},
		},
		{
			// campaign_recipients has NO store_id — only tenant_id and a
			// campaign_id FK. Scoped through campaigns.store_id.
			Table:       "campaign_recipients", // 000024
			Disposition: DispositionDelete,
			SQL: `DELETE FROM campaign_recipients WHERE customer_email = ?
				AND campaign_id IN (SELECT id FROM campaigns WHERE store_id = ?)`,
			Args: []any{email, storeID},
		},
		{
			// recipient_user_id is varchar with no FK, and holds
			// customer_profiles.id — established while fixing #350. Hence
			// the ::text cast.
			Table:       "notifications", // 000091
			Disposition: DispositionDelete,
			SQL: `DELETE FROM notifications WHERE store_id = ? AND recipient_user_id IN (
				SELECT id::text FROM customer_profiles WHERE store_id = ? AND email = ?)`,
			Args: []any{storeID, storeID, email},
		},
		{
			Table:       "email_sends", // 000108
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM email_sends WHERE store_id = ? AND recipient = ?`,
			Args:        []any{storeID, email},
		},

		// ---- Group 2: ANONYMISE. Rows that must survive.

		{
			// name, line1 and city are NOT NULL — they take placeholders,
			// not NULL. country_code is DELIBERATELY untouched: it is
			// required for tax reporting and is not identifying alone.
			Table:       "order_addresses", // 000001
			Disposition: DispositionAnonymise,
			SQL: `UPDATE order_addresses SET name = ?, line1 = ?, line2 = NULL, city = ?,
				region = NULL, postal_code = NULL, phone = NULL
				WHERE order_id IN (` + subjectOrders + `)`,
			Args: []any{RedactedName, RedactedLine, RedactedLine, storeID, email},
		},
		{
			// order_events is mixed-subject: staff actors appear here too.
			// Only rows whose actor IS the subject are rewritten.
			Table:       "order_events", // 000001
			Disposition: DispositionAnonymise,
			SQL: `UPDATE order_events SET actor_email = ?
				WHERE actor_email = ? AND order_id IN (` + subjectOrders + `)`,
			Args: []any{token, email, storeID, email},
		},
		{
			// pickup_details is free text holding the customer's pickup
			// address. It is nullable, so it is cleared outright.
			Table:       "returns", // 000006
			Disposition: DispositionAnonymise,
			SQL: `UPDATE returns SET pickup_details = NULL
				WHERE store_id = ? AND order_id IN (` + subjectOrders + `)`,
			Args: []any{storeID, storeID, email},
		},
		{
			// Anonymised, not deleted: deleting retroactively changes the
			// merchant's historical star rating. customer_name and
			// customer_email are both NOT NULL.
			Table:       "reviews", // 000017
			Disposition: DispositionAnonymise,
			SQL: `UPDATE reviews SET customer_email = ?, customer_name = ?, customer_profile_id = NULL
				WHERE store_id = ? AND customer_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			// Scoped by store through reviews, but NOT by the subject's
			// email on reviews — a customer can reply on someone else's
			// review. author_name is NOT NULL.
			Table:       "review_replies", // 000017
			Disposition: DispositionAnonymise,
			SQL: `UPDATE review_replies SET author_email = ?, author_name = ?
				WHERE author_type = ? AND author_email = ?
				AND review_id IN (SELECT id FROM reviews WHERE store_id = ?)`,
			Args: []any{token, RedactedName, authorTypeCustomer, email, storeID},
		},
		{
			// coupon_usage has no store_id and NO name column — only
			// customer_email. Scoped through orders.store_id.
			Table:       "coupon_usage", // 000025
			Disposition: DispositionAnonymise,
			SQL: `UPDATE coupon_usage SET customer_email = ?
				WHERE customer_email = ? AND order_id IN (SELECT id FROM orders WHERE store_id = ?)`,
			Args: []any{token, email, storeID},
		},
		{
			Table:       "customer_loyalties", // 000026
			Disposition: DispositionAnonymise,
			SQL: `UPDATE customer_loyalties SET customer_email = ?, customer_name = ?
				WHERE store_id = ? AND customer_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			// gift_cards holds the subject in THREE roles, each its own
			// pair of columns, and any one row can hold the subject in more
			// than one. One step per role. The free-text `message` is the
			// sender's, so it is cleared with the sender.
			Table:       "gift_cards", // 000027
			Disposition: DispositionAnonymise,
			SQL: `UPDATE gift_cards SET sender_email = ?, sender_name = ?, message = NULL
				WHERE store_id = ? AND sender_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			Table:       "gift_cards",
			Disposition: DispositionAnonymise,
			SQL: `UPDATE gift_cards SET recipient_email = ?, recipient_name = ?
				WHERE store_id = ? AND recipient_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			Table:       "gift_cards",
			Disposition: DispositionAnonymise,
			SQL: `UPDATE gift_cards SET purchased_by_email = ?, purchased_by_name = ?
				WHERE store_id = ? AND purchased_by_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			// support_tickets is the CUSTOMER support table (000089);
			// submitted_by_name and submitted_by_email are both NOT NULL.
			Table:       "support_tickets", // 000089
			Disposition: DispositionAnonymise,
			SQL: `UPDATE support_tickets SET submitted_by_email = ?, submitted_by_name = ?
				WHERE store_id = ? AND submitted_by_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			Table:       "support_ticket_replies", // 000089
			Disposition: DispositionAnonymise,
			SQL: `UPDATE support_ticket_replies SET author_email = ?, author_name = ?
				WHERE author_type = ? AND author_email = ?
				AND ticket_id IN (SELECT id FROM support_tickets WHERE store_id = ?)`,
			Args: []any{token, RedactedName, authorTypeCustomer, email, storeID},
		},
		{
			// `tickets` (000019) is the platform-engineering table, normally
			// merchant-submitted. It is covered anyway: the match is on the
			// subject's exact email within this store, so a row only changes
			// if it really does name the subject.
			Table:       "tickets", // 000019
			Disposition: DispositionAnonymise,
			SQL: `UPDATE tickets SET submitted_by_email = ?, submitted_by_name = ?
				WHERE store_id = ? AND submitted_by_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},
		{
			Table:       "ticket_replies", // 000019
			Disposition: DispositionAnonymise,
			SQL: `UPDATE ticket_replies SET author_email = ?, author_name = ?
				WHERE author_type = ? AND author_email = ?
				AND ticket_id IN (SELECT id FROM tickets WHERE store_id = ?)`,
			Args: []any{token, RedactedName, authorTypeCustomer, email, storeID},
		},
		{
			// LAST anonymise step: order_addresses, order_events and returns
			// all find their rows through orders.customer_email.
			// customer_email is NOT NULL; customer_name is nullable but is
			// given the placeholder rather than NULL so the row still reads
			// as deliberately erased rather than as a guest order.
			Table:       "orders", // 000001
			Disposition: DispositionAnonymise,
			SQL: `UPDATE orders SET customer_email = ?, customer_name = ?, customer_id = NULL
				WHERE store_id = ? AND customer_email = ?`,
			Args: []any{token, RedactedName, storeID, email},
		},

		// ---- Group 3: DELETE the identity itself, last.
		{
			// Everything above either deleted its rows or nulled its
			// customer_profile_id, so nothing references this row by the
			// time it goes.
			Table:       "customer_profiles", // 000013
			Disposition: DispositionDelete,
			SQL:         `DELETE FROM customer_profiles WHERE store_id = ? AND email = ?`,
			Args:        []any{storeID, email},
		},
	}

	return steps
}
