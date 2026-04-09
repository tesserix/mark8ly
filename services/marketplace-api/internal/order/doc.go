// Package order owns the orders, order_items, order_addresses, order_events,
// returns, return_items, and abandoned_carts tables in marketplace_db, plus
// per-store Postgres SEQUENCE objects created eagerly by the
// stores_after_insert_create_sequences trigger (migration 000004_orders_seq_eager).
// Every stores row insert fires mk_create_store_sequences() which creates both
// mk_seq_order_<id> and mk_seq_return_<id> in the same transaction, so the
// sequences are guaranteed to exist before any order write can reach the store.
//
// Note: orders does NOT own an outbox or pending_events table. Customer-facing
// order events are written to the shared outbox_events table (products-owned,
// see internal/outbox) using new aggregate and event_type constants added in
// Orders M2.
//
// Invariants (enforced at the DB layer; repeated here so readers don't need
// to reread the migration to understand them):
//
//   - orders.status is the operational lifecycle and NEVER includes 'refunded'.
//     Money state lives on orders.payment_status exclusively.
//   - orders.refunded_amount is the atomic running refund total. Refunds are
//     recorded via a single UPDATE ... WHERE refunded_amount + $new <= grand_total
//     statement (implemented in M2). M1 only proves the column exists.
//   - orders.idempotency_key is the INLINE idempotency column for checkout
//     creates (same cart session id -> same order row). It is separate from
//     the shared idempotency_keys table products ships, which is used in M4
//     for the refund endpoint's Idempotency-Key HTTP header.
//   - order_items is a price snapshot. product_id/variant_id have NO foreign
//     keys so products can be hard-deleted without corrupting order history.
//     DO NOT add those foreign keys later.
//   - order_addresses is immutable. No updated_at column, no trigger.
//   - returns and return_items hold an ON DELETE RESTRICT chain back to
//     order_items, which makes hard-deleting an order with returns IMPOSSIBLE.
//     Soft delete via orders.deleted_at is the only delete path in slice 1.
//   - Order and return numbers are issued by per-store Postgres sequences with
//     CACHE 50, NOT by a shared hot-row counter. Sequences are monotonic per
//     store forever; they do NOT reset daily. See NextDocumentNumber in number.go.
//
// See docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md §4.1.
package order
