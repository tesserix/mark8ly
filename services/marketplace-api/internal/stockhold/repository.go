// Package stockhold implements time-boxed stock reservations in Postgres
// (#231, under the #229 epic).
//
// # The problem it exists for
//
// The storefront had no inventory control at all: with one unit in stock,
// two concurrent customers both checked out and the quantity never moved
// (#230). A comment claimed a decrement-on-sale trigger existed; it never
// did.
//
// # Availability is computed, never stored
//
//	available = variant_stock.quantity
//	          - SUM(qty) FILTER (WHERE state = 'held' AND expires_at > now())
//
// A hold therefore expires BY THE CLOCK, not by a job running. If the
// sweeper stops, availability stays correct and only dead rows accumulate.
// A stored "reserved" counter would make the sweeper load-bearing for
// correctness, and a missed run would silently strand stock — the failure
// mode you discover at Christmas.
//
// # Why Postgres and not Redis
//
// The hold lives in the same database and the same transaction as the order.
// A hold in Redis with the order in Postgres cannot be made atomic, so a
// Redis failover would release stock a paying customer is mid-checkout on.
package stockhold

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrInsufficientStock signals the requested quantity exceeds what is
// available once live holds are subtracted.
//
// Deliberately distinct from a generic failure: the storefront turns this
// into a 409 naming the variant, and a caller must never confuse "someone
// else holds the last unit" with "the database is unreachable".
var ErrInsufficientStock = errors.New("stockhold: insufficient stock")

// sweepGrace is how long an expired hold is kept before the sweeper deletes
// it. An expired hold already reduces availability by nothing, so there is
// no rush — and keeping it briefly leaves evidence of what happened when
// someone asks why a cart lost its unit.
const sweepGrace = time.Hour

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Hold reserves qty of a variant for cartToken until now()+ttl.
//
// It runs in the CALLER'S transaction so the reservation and whatever
// prompted it commit or roll back together.
//
// Idempotent per (cart_token, variant_id, location_id): a repeat call
// refreshes the expiry rather than stacking a second reservation, so a
// customer revisiting checkout does not consume their own stock twice and
// lock themselves out.
func (r *Repository) Hold(ctx context.Context, tx *gorm.DB, cartToken, variantID, locationID string, qty int, ttl time.Duration) error {
	if qty <= 0 {
		return fmt.Errorf("stockhold: qty must be positive, got %d", qty)
	}

	// SELECT ... FOR UPDATE on the stock row serialises contenders. Without
	// it, two transactions read the same availability, both find it
	// sufficient, and both insert — the oversell in #230. The lock is taken
	// on variant_stock rather than on the holds, because the holds a
	// competitor is about to insert do not exist yet to be locked.
	var quantity int
	err := tx.WithContext(ctx).Raw(
		`SELECT quantity FROM variant_stock WHERE variant_id = ? AND location_id = ? FOR UPDATE`,
		variantID, locationID).Scan(&quantity).Error
	if err != nil {
		return fmt.Errorf("stockhold: lock stock row: %w", err)
	}

	// Sum live holds, EXCLUDING this cart's own: a refresh must not count
	// the reservation it is replacing, or a cart could never re-hold what it
	// already has.
	var held int
	err = tx.WithContext(ctx).Raw(
		`SELECT COALESCE(SUM(qty), 0) FROM stock_holds
		  WHERE variant_id = ? AND location_id = ?
		    AND state = 'held' AND expires_at > now()
		    AND cart_token <> ?`,
		variantID, locationID, cartToken).Scan(&held).Error
	if err != nil {
		return fmt.Errorf("stockhold: sum live holds: %w", err)
	}

	if quantity-held < qty {
		return fmt.Errorf("%w: want %d, available %d", ErrInsufficientStock, qty, quantity-held)
	}

	// ON CONFLICT makes the refresh atomic with the check above, inside the
	// row lock. state is reset to 'held' so a cart that released and came
	// back reuses its row rather than colliding with the unique constraint.
	return tx.WithContext(ctx).Exec(
		`INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
		 VALUES (?, ?, ?, ?, ?, 'held')
		 ON CONFLICT (cart_token, variant_id, location_id)
		 DO UPDATE SET qty = EXCLUDED.qty, expires_at = EXCLUDED.expires_at, state = 'held'`,
		variantID, locationID, cartToken, qty, time.Now().Add(ttl)).Error
}

// Available reports how many units a cart could still hold, live holds
// subtracted.
//
// It excludes the calling cart's own holds for the same reason Hold does: a
// cart asking what it can have must not be told its own reservation is
// competition.
//
// This is a READ for display — "only 2 left" beats a bare refusal. It takes
// no lock and must not be used to decide whether a hold may be placed: by
// the time a caller acted on it, the answer could be stale. Hold does its
// own check under FOR UPDATE, and that is the one that counts.
func (r *Repository) Available(ctx context.Context, tx *gorm.DB, variantID, locationID, exceptCart string) (int, error) {
	var available int
	err := tx.WithContext(ctx).Raw(
		`SELECT COALESCE(vs.quantity, 0) - COALESCE((
		            SELECT SUM(h.qty) FROM stock_holds h
		             WHERE h.variant_id = vs.variant_id AND h.location_id = vs.location_id
		               AND h.state = 'held' AND h.expires_at > now()
		               AND h.cart_token <> ?
		        ), 0)
		   FROM variant_stock vs
		  WHERE vs.variant_id = ? AND vs.location_id = ?`,
		exceptCart, variantID, locationID).Scan(&available).Error
	if err != nil {
		return 0, fmt.Errorf("stockhold: available: %w", err)
	}
	if available < 0 {
		// Defensive: a committed sale already decremented the stock row, so
		// arithmetic cannot normally go below zero. Reporting a negative
		// count to a storefront would render as nonsense.
		return 0, nil
	}
	return available, nil
}

// Commit decrements variant_stock for every live hold on the cart and flips
// those holds to 'committed', in the caller's transaction.
//
// The decrement and the state flip must be one statement pair inside one
// transaction: a committed hold no longer reduces availability (the stock
// row already reflects it), so a crash between them would either double-count
// the sale or release it back to the pool.
func (r *Repository) Commit(ctx context.Context, tx *gorm.DB, cartToken string) error {
	err := tx.WithContext(ctx).Exec(
		`UPDATE variant_stock vs
		    SET quantity = vs.quantity - h.qty, updated_at = now()
		   FROM stock_holds h
		  WHERE h.cart_token = ? AND h.state = 'held'
		    AND vs.variant_id = h.variant_id AND vs.location_id = h.location_id`,
		cartToken).Error
	if err != nil {
		// variant_stock_quantity_non_negative fires here if the arithmetic
		// would go below zero — the CHECK is the guarantee behind the
		// FOR UPDATE discipline, not a duplicate of it.
		return fmt.Errorf("stockhold: decrement stock: %w", err)
	}

	return tx.WithContext(ctx).Exec(
		`UPDATE stock_holds SET state = 'committed' WHERE cart_token = ? AND state = 'held'`,
		cartToken).Error
}

// Release returns a cart's live holds to the pool.
func (r *Repository) Release(ctx context.Context, tx *gorm.DB, cartToken string) error {
	return tx.WithContext(ctx).Exec(
		`UPDATE stock_holds SET state = 'released' WHERE cart_token = ? AND state = 'held'`,
		cartToken).Error
}

// Extend pushes a live cart's expiry out, for a customer still working
// through checkout.
//
// It deliberately does NOT revive an expired hold: by then the units may be
// someone else's, and silently taking them back would oversell exactly as if
// no hold existed.
func (r *Repository) Extend(ctx context.Context, tx *gorm.DB, cartToken string, ttl time.Duration) error {
	return tx.WithContext(ctx).Exec(
		`UPDATE stock_holds SET expires_at = ?
		  WHERE cart_token = ? AND state = 'held' AND expires_at > now()`,
		time.Now().Add(ttl), cartToken).Error
}

// Sweep deletes holds that expired more than sweepGrace ago, in bounded
// batches, and returns how many it removed.
//
// It is housekeeping, not correctness: an expired hold already reduces
// availability by nothing. FOR UPDATE SKIP LOCKED so concurrent sweepers on
// multiple replicas do not block each other or double-delete.
func (r *Repository) Sweep(ctx context.Context, db *gorm.DB, batch int) (int64, error) {
	if batch <= 0 {
		batch = 500
	}
	res := db.WithContext(ctx).Exec(
		`DELETE FROM stock_holds
		  WHERE id IN (
		      SELECT id FROM stock_holds
		       WHERE state = 'held' AND expires_at < now() - ?::interval
		       ORDER BY expires_at
		       LIMIT ?
		       FOR UPDATE SKIP LOCKED
		  )`,
		fmt.Sprintf("%d seconds", int(sweepGrace.Seconds())), batch)
	return res.RowsAffected, res.Error
}
