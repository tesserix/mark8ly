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
//     personal column at all. payment_transactions.metadata (JSONB) is a
//     DEAD column — payment.PaymentTransaction declares no Metadata field
//     and no production statement writes it, so it is always '{}' (#435).
//   - order_items, order_tax_lines, return_items, loyalty_transactions,
//     gift_card_transactions and referrals hold no personal column; they
//     survive attached to rows that have been anonymised.
//
// JSONB COLUMNS. #435 audited the nine blobs that can embed a customer email
// or address and resolved each one:
//   - shipments.ship_to / ship_from, audit_logs.metadata and
//     outbox_events.payload carry the subject and are key-stripped by steps
//     below. Keys are stripped by NAME — never a wholesale rewrite — so a
//     blob's non-personal structure survives intact.
//   - abandoned_carts.items_snapshot goes with the row, which is deleted.
//   - returns.pickup_details is text, not JSONB, and is already nulled.
//   - payment_transactions.metadata is the dead column described above.
//   - stripe_webhook_events.payload is merchant subscription billing: its
//     "customer" is the tenant, not a shopper, and a tenant purge already
//     destroys it.
//   - idempotency_keys.response is platform-operator only and self-expires
//     on a 24h TTL.
//   - webhook_events.payload is genuinely unscopable — the table has no
//     tenant, store or order column, so a subject's rows cannot be found. It
//     needs an age-based prune, tracked separately as #440.
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

// aggregateAbandonedCart is the outbox_events.aggregate discriminator for the
// one event whose payload names a shopper. It is the value of
// outbox.AggregateAbandonedCart, duplicated as a local const rather than
// imported so the plan stays a pure, dependency-free description of
// statements. There is NO CHECK constraint on outbox_events.aggregate, so
// nothing but this pairing keeps the two in step — the integration test seeds
// a row with the literal the producer writes and asserts the strip reaches it.
const aggregateAbandonedCart = "abandoned_cart"

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
			// outbox_events has NO store_id column — only tenant_id — so it
			// is scoped through abandoned_carts, the one aggregate whose
			// payload names a shopper: order/abandoned_cart_service.go:128
			// writes customer_email and recovery_url into it.
			//
			// MUST RUN BEFORE the abandoned_carts DELETE immediately below,
			// which destroys the very rows this subquery reads. Same
			// ordering hazard as review_media/reviews, and pinned by a test.
			//
			// Only those two keys go; store_id, item_count, subtotal and
			// currency stay, because outbox/publisher.go:112 reads store_id
			// to route the event and an unroutable row poisons its batch. On
			// an unpublished row this degrades to a recovery email that is
			// never sent, which is the correct outcome for an erased person.
			Table:       "outbox_events", // 000001
			Disposition: DispositionAnonymise,
			SQL: `UPDATE outbox_events SET payload = payload - 'customer_email' - 'recovery_url'
				WHERE aggregate = ? AND aggregate_id IN (
					SELECT id FROM abandoned_carts WHERE store_id = ? AND customer_email = ?)`,
			Args: []any{aggregateAbandonedCart, storeID, email},
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
			// ship_to is the customer's delivery address — eight keys
			// written at handlers/admin/shipments.go:694. ship_from is
			// normally the MERCHANT's warehouse, but
			// shipmentcancel/executor.go:254 persists reverse-leg rows with
			// ShipFrom = the forward row's ShipTo, i.e. the customer's
			// address. Which column holds whom is not decidable per row
			// without re-deriving the leg, so BOTH are stripped: a merchant
			// warehouse address is recoverable from warehouses, a person's
			// is not recoverable at all once erasure is due.
			//
			// Both are NOT NULL, so they are emptied, not nulled. The one
			// reader, shipmentcancel.parseShipmentAddress, treats an
			// address with no line1 as unusable and the reverse pickup
			// fails cleanly with "arrange the return manually".
			//
			// Scoped through orders, so it MUST run before orders is
			// anonymised.
			Table:       "shipments", // 000008
			Disposition: DispositionAnonymise,
			SQL: `UPDATE shipments SET ship_to = '{}'::jsonb, ship_from = '{}'::jsonb
				WHERE store_id = ? AND order_id IN (` + subjectOrders + `)`,
			Args: []any{storeID, storeID, email},
		},
		{
			// audit_logs.metadata is the highest-value blob in this plan,
			// because it is the longest-lived: retention is plan-tiered
			// (audit/prune_cron.go:42) at 90d trial/starter, 365d studio,
			// and NEVER for Pro. A Pro merchant's audit metadata outlives
			// every other copy of the address.
			//
			// Six keys, each traced to a writer: customer_email
			// (storefront/checkout.go:242, checkout_ext.go:681), email
			// (customer/service.go:103), recipient_email
			// (admin/gift_cards.go:127), submitter_email / author_email /
			// actor_email (storefront/tickets.go:189,501,628).
			//
			// The audit_logs.actor_EMAIL COLUMN is deliberately untouched:
			// it names the operator or staff member who performed the
			// action, which is the governance record itself and is not the
			// subject's personal data. Only the metadata key of that name is
			// stripped.
			//
			// The row is matched by the subject's address appearing in any
			// of those six keys AND by store. `->> = ?` rather than
			// jsonb_exists: existence alone would strip a bystander's email
			// out of the subject's audit rows, and there is no key by which
			// a row "belongs to" one shopper other than the address itself.
			// Absent keys yield NULL, which is not equal to anything, so a
			// row with none of them is never rewritten. Deliberately NOT the
			// infix `metadata ? 'k'` operator — GORM consumes `?` as a bind
			// placeholder and would misbind the statement (#369).
			//
			// store_id is NULLABLE here (operator rows carry NULL by
			// design); `store_id = ?` excludes them, which is correct: an
			// operator row is not a customer record.
			Table:       "audit_logs", // 000035
			Disposition: DispositionAnonymise,
			SQL: `UPDATE audit_logs
				SET metadata = metadata - 'customer_email' - 'email' - 'recipient_email'
					- 'submitter_email' - 'author_email' - 'actor_email'
				WHERE store_id = ?
				  AND (metadata ->> 'customer_email' = ?
					OR metadata ->> 'email' = ?
					OR metadata ->> 'recipient_email' = ?
					OR metadata ->> 'submitter_email' = ?
					OR metadata ->> 'author_email' = ?
					OR metadata ->> 'actor_email' = ?)`,
			Args: []any{storeID, email, email, email, email, email, email},
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
