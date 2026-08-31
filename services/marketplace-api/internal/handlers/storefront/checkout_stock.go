package storefront

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
)

// inventoryPolicyContinue means the merchant sells past zero on purpose.
const inventoryPolicyContinue = "continue"

// outOfStockError names the variant a shopper has to remove.
//
// It carries the id because a bare "out of stock" is unactionable on a cart
// of six items — the storefront needs to know which line to mark.
type outOfStockError struct {
	VariantID string
}

func (e outOfStockError) Error() string {
	return fmt.Sprintf("storefront: variant %s is out of stock", e.VariantID)
}

// stockLine is one checkout item that consumes inventory.
type stockLine struct {
	VariantID string
	Quantity  int
}

// commitStock reserves and consumes inventory for a checkout, INSIDE the
// order's transaction (#230).
//
// # Why it runs in the order transaction
//
// Decrementing after the order commits would leave a paid order whose stock
// was never taken when the decrement failed; decrementing before would strand
// stock when the order failed. order.CreateInput.WithinTx exists for exactly
// this shape — returning an error here rolls the order back entirely, which
// is the only correct outcome: the buyer can retry and nothing has moved.
//
// # Hold-then-commit, even for a cart that already holds
//
// stockhold.Hold is idempotent per (cart_token, variant, location) and its
// availability sum excludes the calling cart, so re-holding what this cart
// already reserved is free and does not double-count. That makes one code
// path serve both cases: a storefront that placed holds at cart-add, and any
// caller that did not. Checkout enforces availability either way, which is
// what stops #230's oversell independently of the UI.
//
// A hold placed at cart-add that has since EXPIRED simply fails here if the
// units are gone — which is correct, and is what the shopper is told.
func commitStock(
	ctx context.Context,
	tx *gorm.DB,
	holds *stockhold.Repository,
	cartToken string,
	orderID string,
	storeID string,
	lines []stockLine,
) error {
	if len(lines) == 0 {
		return nil
	}

	policies, err := inventoryPolicies(tx, lines)
	if err != nil {
		return err
	}

	// A store with NO warehouses keeps the pre-#177 behaviour exactly:
	// hold and decrement at the sentinel location, write no allocations.
	// This is not a tidy fallback — production has zero warehouse rows, so
	// it is the only path that currently runs, and allocation.Plan would
	// return ErrNoWarehouse for every checkout if it ran unconditionally.
	warehouses, err := storeWarehousesInFillOrder(ctx, tx, storeID)
	if err != nil {
		return err
	}
	if len(warehouses) == 0 {
		return commitStockAtSentinel(ctx, tx, holds, cartToken, lines, policies)
	}

	avail, storage, err := loadAvailability(ctx, tx, cartToken, warehouses, variantIDsOf(lines))
	if err != nil {
		return err
	}

	allocLines := make([]allocation.Line, 0, len(lines))
	for _, l := range lines {
		allocLines = append(allocLines, allocation.Line{
			VariantID:     l.VariantID,
			Quantity:      l.Quantity,
			SellsPastZero: policies[l.VariantID] == inventoryPolicyContinue,
		})
	}

	assignments, err := allocation.Plan(warehouses, avail, allocLines)
	var cannot allocation.CannotFillError
	if errors.As(err, &cannot) {
		// Same shape the shopper already gets for a sold-out cart.
		return outOfStockError{VariantID: cannot.VariantID}
	}
	if err != nil {
		return err
	}

	// stockhold.Hold's ON CONFLICT DO UPDATE SET qty = EXCLUDED.qty REPLACES
	// a quantity rather than adding to it, and its unique key is
	// (cart_token, variant_id, location_id). Two assignments for the same
	// variant and warehouse — which happens whenever two order lines carry
	// the same variant, because stockLinesFromItems does not merge them —
	// would leave only the SECOND quantity reserved. Aggregate first.
	type holdKey struct{ variantID, locationID string }
	totals := map[holdKey]int{}
	// drawn tracks, per (variant, location), how many units EARLIER
	// assignments in this loop already claimed from that location's
	// breakdown. storage[...] is an immutable snapshot taken once before the
	// loop — without this, every assignment would draw from the FULL
	// breakdown again, and two lines of the same variant filled at the same
	// warehouse would double-count what the location actually holds (e.g. a
	// breakdown of [sentinel:3, A:4] with two assignments of 2 and 3 must
	// draw 3 from the sentinel and 2 from A in total, not 5 from the
	// sentinel).
	drawn := map[holdKey]int{}
	for _, a := range assignments {
		if policies[a.VariantID] == inventoryPolicyContinue {
			// Sell past zero on purpose. No hold, no gate — decrement at
			// this assignment's warehouse, clamped for the same reason the
			// sentinel path clamps (#231's non-negative CHECK).
			at := storage[a.VariantID][a.WarehouseID]
			if len(at) == 0 {
				// No variant_stock row exists for this warehouse — the
				// common case for a sell-past-zero variant, since it never
				// needed a stock row to sell. The legacy sentinel path's
				// UPDATE simply matches zero rows and succeeds; erroring
				// here would hard-fail a checkout that used to work the
				// moment a merchant has a warehouse at all (PR 5).
				continue // sold past zero on purpose; nothing to decrement
			}
			// Walk every location the way the hold path does, rather than
			// only at[0]: a warehouse whose units span the sentinel row and
			// a real row must have BOTH decremented, or the second silently
			// keeps stale stock. The last location absorbs whatever is left
			// after the others — continue-policy is unconstrained by
			// availability, so there is no "short" to error on here the way
			// the hold path does; GREATEST clamps it at zero.
			want := a.Quantity
			for i, loc := range at {
				take := want
				if i < len(at)-1 {
					take = min(want, loc.Units)
				}
				if take <= 0 {
					continue
				}
				if err := tx.WithContext(ctx).Exec(
					`UPDATE variant_stock
					    SET quantity = GREATEST(quantity - ?, 0), updated_at = now()
					  WHERE variant_id = ? AND location_id = ?`,
					take, a.VariantID, loc.LocationID).Error; err != nil {
					return fmt.Errorf("storefront: decrement continue-policy variant: %w", err)
				}
				want -= take
				if want == 0 {
					break
				}
			}
			continue // sold past zero on purpose; decremented, not held
		}
		// A warehouse's units can sit in more than one physical location
		// while the sentinel and real rows coexist, so draw the assigned
		// quantity from that warehouse's locations in order until it is
		// covered. The breakdown is sorted by LocationID, so the same
		// inputs always produce the same holds.
		want := a.Quantity
		for _, at := range storage[a.VariantID][a.WarehouseID] {
			if want == 0 {
				break
			}
			key := holdKey{a.VariantID, at.LocationID}
			remaining := at.Units - drawn[key]
			take := min(want, remaining)
			if take <= 0 {
				continue
			}
			totals[key] += take
			drawn[key] += take
			want -= take
		}
		if want > 0 {
			// The snapshot said this warehouse had the units and the
			// breakdown does not account for them. That is a bug in the
			// snapshot, not a stock shortage — fail loudly rather than
			// under-hold and oversell.
			return fmt.Errorf(
				"storefront: allocation for variant %s at warehouse %s is %d units short of its storage breakdown",
				a.VariantID, a.WarehouseID, want)
		}
	}
	// Release any cart-time hold whose (variant, location) is not part of
	// this plan BEFORE placing the plan's holds and BEFORE Commit runs.
	// cart_holds.go places holds at product.DefaultLocationID; a placement
	// plan is free to target a different warehouse, and holds.Commit
	// decrements EVERY live hold this cart owns regardless of whether it
	// matches. Left in place, a stale cart-add hold gets decremented a
	// second time alongside the plan's own hold for the same units.
	keep := make([]stockhold.VariantLocation, 0, len(totals))
	for k := range totals {
		keep = append(keep, stockhold.VariantLocation{VariantID: k.variantID, LocationID: k.locationID})
	}
	if err := holds.ReleaseExcept(ctx, tx, cartToken, keep); err != nil {
		return err
	}

	// Sorted rather than ranged directly: map iteration order is randomised,
	// and each Hold takes a SELECT ... FOR UPDATE row lock. Two concurrent
	// checkouts of identical carts taking the same two locks in opposite
	// orders is a deadlock waiting to happen; a fixed order rules it out.
	keys := make([]holdKey, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].variantID != keys[j].variantID {
			return keys[i].variantID < keys[j].variantID
		}
		return keys[i].locationID < keys[j].locationID
	})
	for _, k := range keys {
		qty := totals[k]
		err := holds.Hold(ctx, tx, cartToken, k.variantID, k.locationID, qty, HoldTTL)
		if errors.Is(err, stockhold.ErrInsufficientStock) {
			return outOfStockError{VariantID: k.variantID}
		}
		if err != nil {
			return err
		}
	}

	// order_items rows already exist: order.CreateInput.WithinTx runs after
	// CreateInTx inserts them. Reading them back is how an allocation gets
	// its order_item_id without threading ids through the checkout request.
	if err := recordAllocations(ctx, tx, orderID, assignments, lines); err != nil {
		return err
	}

	// One Commit for the whole cart: it decrements every live hold this cart
	// holds and flips them to 'committed' in the same transaction.
	return holds.Commit(ctx, tx, cartToken)
}

// commitStockAtSentinel is the pre-#177 behaviour, moved here verbatim: hold
// and decrement everything at the single sentinel location, and write no
// order_allocations rows. This is the ONLY path production exercises today
// — there are zero warehouse rows — so its body must stay provably
// unchanged rather than be re-implemented alongside the allocation path.
func commitStockAtSentinel(
	ctx context.Context,
	tx *gorm.DB,
	holds *stockhold.Repository,
	cartToken string,
	lines []stockLine,
	policies map[string]string,
) error {
	for _, line := range lines {
		if policies[line.VariantID] == inventoryPolicyContinue {
			// Sell past zero on purpose. No hold, no gate — and the
			// decrement is clamped, because variant_stock carries a
			// non-negative CHECK (#231) that cannot be policy-aware: the
			// policy lives on product_variants, and a CHECK cannot read
			// another table.
			//
			// The consequence, stated rather than hidden: how far a
			// 'continue' variant is oversold is NOT tracked. The orders
			// are the record of what was sold; this column stops at zero.
			if err := tx.WithContext(ctx).Exec(
				`UPDATE variant_stock
				    SET quantity = GREATEST(quantity - ?, 0), updated_at = now()
				  WHERE variant_id = ? AND location_id = ?`,
				line.Quantity, line.VariantID, product.DefaultLocationID).Error; err != nil {
				return fmt.Errorf("storefront: decrement continue-policy variant: %w", err)
			}
			continue
		}

		err := holds.Hold(ctx, tx, cartToken, line.VariantID,
			product.DefaultLocationID, line.Quantity, HoldTTL)
		if errors.Is(err, stockhold.ErrInsufficientStock) {
			return outOfStockError{VariantID: line.VariantID}
		}
		if err != nil {
			return err
		}
	}

	// One Commit for the whole cart: it decrements every live hold this cart
	// holds and flips them to 'committed' in the same transaction.
	return holds.Commit(ctx, tx, cartToken)
}

// storeWarehousesInFillOrder reads storeID's warehouses and returns them in
// the order allocation.Plan should fill them. Plan takes an ordered slice
// and cannot detect an unordered one, so ordering here is mandatory.
func storeWarehousesInFillOrder(ctx context.Context, tx *gorm.DB, storeID string) ([]allocation.Warehouse, error) {
	var rows []allocation.Warehouse
	if err := tx.WithContext(ctx).Raw(
		`SELECT id, priority, is_default, created_at FROM warehouses WHERE store_id = ?`, storeID).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("storefront: load warehouses: %w", err)
	}
	return allocation.InPriorityOrder(rows), nil
}

// variantIDsOf returns the distinct variant ids across lines.
func variantIDsOf(lines []stockLine) []string {
	seen := make(map[string]struct{}, len(lines))
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		if _, ok := seen[l.VariantID]; ok {
			continue
		}
		seen[l.VariantID] = struct{}{}
		ids = append(ids, l.VariantID)
	}
	return ids
}

// recordAllocations writes one order_allocations row per assignment.
//
// # Why order_items cannot be paired to lines by position
//
// Production inserts every item of an order in ONE batch tx.Create(&items)
// (internal/order/repository.go), so all of an order's rows share a single
// created_at, and the only tie-break `ORDER BY created_at, id` has left is
// id — gen_random_uuid(). items[i] is then a RANDOM permutation of lines,
// not lines[i]'s item: pairing positionally silently attaches every
// allocation to the wrong line the moment an order has two or more items.
//
// # Items without a variant_id are not lines
//
// order.CreateInput.WithinTx inserts an order_items row for EVERY checkout
// item, including ones whose variant_id is nil or unparseable, but
// stockLinesFromItems drops those — they carry nothing to allocate. The
// query below only reads order_items whose variant_id IS NOT NULL, so items
// is naturally restricted to the ones that can correspond to a line; a
// variantless item is simply ignored here, matching the legacy sentinel path
// which silently skipped it too. What still fails loudly is a genuine
// attribution mismatch: a line for which no matching order_item can be
// found, in the per-line pairing loop below.
//
// Paired instead by the (variant_id, quantity) MULTISET: two lines
// identical in both are genuinely interchangeable — swapping which gets
// which order_item_id changes nothing observable — so matching on that key
// is exact, and does not depend on any ordering order_items happens to
// come back in.
//
// # Attributing an assignment back to a line
//
// allocation.Plan documents its output as "grouped by warehouse in fill
// order, and by LINE order within a warehouse" — not grouped by line. So two
// lines sharing a variant, split across warehouses with partial fills, can
// interleave: {whA: line0, whA: line2, whB: line0, whB: line2}. A running
// "current line" pointer over the flat assignment slice mis-attributes that
// case. Instead, every time the warehouse changes, the set of not-yet-filled
// lines for each variant is rebuilt in ascending index order; within one
// warehouse's contribution to a given variant, Plan visits at most one
// assignment per line, in that same ascending order, so consuming that
// queue front-to-back for each variant, only resetting on a warehouse
// change, is exact.
func recordAllocations(
	ctx context.Context,
	tx *gorm.DB,
	orderID string,
	assignments []allocation.Assignment,
	lines []stockLine,
) error {
	var items []struct {
		ID        string
		VariantID string
		Quantity  int
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT id, variant_id, quantity FROM order_items
		  WHERE order_id = ? AND variant_id IS NOT NULL ORDER BY created_at, id`,
		orderID).Scan(&items).Error; err != nil {
		return fmt.Errorf("storefront: load order items for allocation: %w", err)
	}

	var tenantID, storeID string
	if err := tx.WithContext(ctx).Raw(
		`SELECT tenant_id, store_id FROM orders WHERE id = ?`, orderID).
		Row().Scan(&tenantID, &storeID); err != nil {
		return fmt.Errorf("storefront: load order for allocation: %w", err)
	}

	type itemKey struct {
		variantID string
		quantity  int
	}
	byKey := map[itemKey][]string{}
	for _, it := range items {
		k := itemKey{it.VariantID, it.Quantity}
		byKey[k] = append(byKey[k], it.ID)
	}

	lineItemID := make([]string, len(lines))
	for i, l := range lines {
		k := itemKey{l.VariantID, l.Quantity}
		queue := byKey[k]
		if len(queue) == 0 {
			// Every line was supposed to have created exactly one
			// order_item with its own (variant, quantity). Running out
			// means the order_items this order actually has don't match
			// what commitStock was told to place — fail loudly rather
			// than attribute the allocation to an unrelated line.
			return fmt.Errorf(
				"storefront: no order_item left matching variant %s quantity %d for line %d of order %s",
				l.VariantID, l.Quantity, i, orderID)
		}
		lineItemID[i] = queue[0]
		byKey[k] = queue[1:]
	}

	remaining := make([]int, len(lines))
	for i, l := range lines {
		remaining[i] = l.Quantity
	}

	var prevWarehouse string
	haveWarehouse := false
	queues := map[string][]int{} // variantID -> ascending line indices still owed, as of the current warehouse
	pointer := map[string]int{}  // variantID -> next queue position to consume

	for _, a := range assignments {
		if !haveWarehouse || a.WarehouseID != prevWarehouse {
			queues = map[string][]int{}
			pointer = map[string]int{}
			for i, l := range lines {
				if remaining[i] > 0 {
					queues[l.VariantID] = append(queues[l.VariantID], i)
				}
			}
			prevWarehouse = a.WarehouseID
			haveWarehouse = true
		}

		idxList := queues[a.VariantID]
		p := pointer[a.VariantID]
		if p >= len(idxList) {
			// Plan produced an assignment for a variant with no line left
			// owing it in this warehouse's chunk — a bug in this pairing
			// logic or in Plan's ordering contract, not a data problem.
			// Fail loudly rather than attribute it to the wrong line.
			return fmt.Errorf(
				"storefront: cannot attribute allocation of %d units of variant %s at warehouse %s to any order line",
				a.Quantity, a.VariantID, a.WarehouseID)
		}
		lineIdx := idxList[p]
		pointer[a.VariantID] = p + 1
		remaining[lineIdx] -= a.Quantity

		id := uuid.NewString()
		if err := tx.WithContext(ctx).Exec(
			`INSERT INTO order_allocations (id, tenant_id, store_id, order_id, order_item_id, warehouse_id, quantity)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, tenantID, storeID, orderID, lineItemID[lineIdx], a.WarehouseID, a.Quantity).Error; err != nil {
			return fmt.Errorf("storefront: insert order_allocations: %w", err)
		}
	}
	return nil
}

// inventoryPolicies reads the policy for each line's variant in one query.
//
// A variant with no row is treated as 'deny' — the column's own default, and
// the safe reading: refusing to oversell something we cannot classify is
// recoverable, overselling it is not.
func inventoryPolicies(tx *gorm.DB, lines []stockLine) (map[string]string, error) {
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.VariantID)
	}

	var rows []struct {
		ID              string
		InventoryPolicy string
	}
	if err := tx.Raw(
		`SELECT id, COALESCE(inventory_policy, 'deny') AS inventory_policy
		   FROM product_variants WHERE id IN ?`, ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("storefront: read inventory policies: %w", err)
	}

	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.InventoryPolicy
	}
	return out, nil
}

// stockLinesFromItems selects the checkout items that consume inventory.
//
// An item with no variant_id is a custom or unstocked line — refusing those
// would break every unstocked sale, and there is nothing to decrement.
//
// Variant ids are lowercased here, at the boundary where lines are built.
// The legacy path passed the client's string straight into SQL, where
// Postgres casts to uuid case-insensitively; everything downstream of this
// function does Go map lookups against ids read back from the database in
// canonical lowercase, so an uppercase id from a client would otherwise miss
// every lookup and read as zero availability.
func stockLinesFromItems(items []CheckoutItemRequest) []stockLine {
	lines := make([]stockLine, 0, len(items))
	for _, it := range items {
		if it.VariantID == nil || *it.VariantID == "" || it.Quantity <= 0 {
			continue
		}
		lines = append(lines, stockLine{VariantID: strings.ToLower(*it.VariantID), Quantity: it.Quantity})
	}
	return lines
}

// cartTokenForCheckout returns the cart identity to commit holds against.
//
// A checkout arriving without one — an API client, or a storefront build
// predating #232 — gets a fresh token, so the hold-then-commit path still
// enforces availability for it. Enforcement must not depend on the caller
// having cooperated.
func cartTokenForCheckout(c *gin.Context) string {
	if ck, err := c.Cookie(CartTokenCookie); err == nil {
		if _, perr := uuid.Parse(ck); perr == nil {
			return ck
		}
	}
	return uuid.NewString()
}
