// Package harddelete implements the irreversible merchant data deletion pipeline
// (§15.2 — 150-day hard delete). Every public function is idempotent: running
// it twice on the same store_id produces the same end state.
package harddelete

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// sweepTable deletes all rows in tableName where the given column = id.
// It logs the delete count via the audit emitter for compliance visibility.
// Errors are returned so the caller can abort the transaction.
func sweepTable(ctx context.Context, tx *gorm.DB, emitter *audit.Emitter, logger *slog.Logger,
	tableName, column string, storeID, tenantID uuid.UUID) error {

	res := tx.WithContext(ctx).Exec(
		fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableName, column), //nolint:gosec // table/column names are internal constants
		storeID,
	)
	if res.Error != nil {
		return fmt.Errorf("harddelete: sweep %s: %w", tableName, res.Error)
	}
	logger.Info("harddelete: swept table",
		"table", tableName, "column", column,
		"store_id", storeID, "rows_deleted", res.RowsAffected)

	// Emit per-table audit event for compliance trail.
	emitter.Emit(nil, audit.Event{
		Action:         "subscription.hard_delete_sweep",
		ResourceType:   tableName,
		ResourceID:     storeID.String(),
		Severity:       audit.SeverityCritical,
		TenantID:       tenantID,
		StoreID:        storeID,
		ForceActorType: audit.ActorSystem,
		Metadata: map[string]any{
			"table":        tableName,
			"rows_deleted": res.RowsAffected,
		},
	})
	return nil
}

// Sweep deletes all tenant-scoped data for storeID across all relevant tables.
// The deletion order respects FK constraints:
//
//  1. Child tables first (order_items, product_variants, etc. cascade via FK).
//  2. Parent tables last (orders, products, customers, stores).
//
// All DELETEs run inside a single transaction passed by the caller (Runner).
// Tables that use tenant_id scoping are filtered by tenant_id; tables that
// use only store_id (or rely on FK cascade from stores) are filtered by store_id.
func Sweep(ctx context.Context, tx *gorm.DB, emitter *audit.Emitter, logger *slog.Logger,
	storeID, tenantID uuid.UUID) error {

	// Ordered sweeps: child rows before parent rows.
	// order_items, abandoned_carts, and order_events cascade from orders.
	// product_variants, product_options, product_media cascade from products via FK.
	// We delete them explicitly before the parent for clarity and safety.

	type tableSpec struct {
		table  string
		column string
	}

	sweeps := []tableSpec{
		// Audit logs for this store (compliance: deleted with the store per §15.2).
		{"audit_logs", "store_id"},
		// Subscription lifecycle audit.
		{"subscription_plan_change_audit", "store_id"},
		{"migration_fast_path_reviews", "store_id"},
		// Review-related child rows.
		{"review_reactions", "store_id"},
		{"review_replies", "store_id"},
		{"review_media", "store_id"},
		{"reviews", "store_id"},
		// Loyalty.
		{"loyalty_transactions", "store_id"},
		{"customer_loyalties", "store_id"},
		{"loyalty_programs", "store_id"},
		// Campaigns.
		{"campaign_recipients", "store_id"},
		{"campaigns", "store_id"},
		// Gift cards.
		{"gift_card_transactions", "store_id"},
		{"gift_cards", "store_id"},
		// Coupons.
		{"coupon_usage", "store_id"},
		{"coupons", "store_id"},
		// Orders (cascade deletes order_items, order_addresses, order_events).
		{"abandoned_carts", "store_id"},
		{"returns", "store_id"},
		{"orders", "store_id"},
		// Shipping, payment, tax configs.
		{"shipments", "store_id"},
		{"payment_transactions", "store_id"},
		{"payment_gateway_configs", "store_id"},
		{"tax_provider_configs", "store_id"},
		// Notifications.
		{"notification_preferences", "store_id"},
		{"notifications", "store_id"},
		// Tickets.
		{"ticket_replies", "store_id"},
		{"tickets", "store_id"},
		// Pages.
		{"pages", "store_id"},
		// Media / product child rows (FK cascades from products).
		{"product_categories", "store_id"},
		{"products", "store_id"},
		{"categories", "store_id"},
		// Customer profiles.
		{"customer_profiles", "store_id"},
		// Wishlists.
		{"wishlists", "store_id"},
		// Custom domains.
		{"custom_domains", "store_id"},
		// Branding.
		{"store_branding", "store_id"},
		// Push tokens.
		{"admin_push_tokens", "store_id"},
		// Subscription row.
		{"store_subscriptions", "store_id"},
		// Outbox events (best-effort — may already be consumed).
		{"outbox_events", "tenant_id"},
		// Idempotency keys (best-effort).
		{"idempotency_keys", "tenant_id"},
		// Stores table — must be last (other tables FK to it).
		{"stores", "id"},
	}

	for _, s := range sweeps {
		id := storeID
		if s.column == "tenant_id" {
			id = tenantID
		}
		if err := sweepTable(ctx, tx, emitter, logger, s.table, s.column, id, tenantID); err != nil {
			return err
		}
	}
	return nil
}
