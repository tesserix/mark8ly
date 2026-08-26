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

// viaParent describes how to reach a store-scoped table that has no
// store_id column of its own: its rows are deleted by joining through a
// parent table (already present earlier in the sweep list) that does carry
// store_id.
type viaParent struct {
	fkColumn    string // column on the child referencing the parent's id, e.g. "review_id"
	parentTable string // parent table, e.g. "reviews"
}

// sweepStep describes one table to sweep. Exactly one of column or via is
// set: column for tables with their own store-scoped column, via for
// tables reached only through a parent FK.
type sweepStep struct {
	table  string
	column string // store-scoped column, when the table has one
	via    *viaParent
}

// sweepTable deletes all rows in s.table that belong to storeID, either
// directly via s.column or, for tables with no store-scoped column of
// their own, via s.via's FK to a parent table that does carry store_id.
// It logs the delete count via the audit emitter for compliance visibility.
// Errors are returned so the caller can abort the transaction.
func sweepTable(ctx context.Context, tx *gorm.DB, emitter *audit.Emitter, logger *slog.Logger,
	s sweepStep, storeID, tenantID uuid.UUID) error {

	tableName := s.table

	var query string
	if s.via != nil {
		query = fmt.Sprintf("DELETE FROM %s WHERE %s IN (SELECT id FROM %s WHERE store_id = ?)",
			tableName, s.via.fkColumn, s.via.parentTable)
	} else {
		query = fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableName, s.column)
	}

	res := tx.WithContext(ctx).Exec(query, //nolint:gosec // table/column names are internal constants
		storeID,
	)
	if res.Error != nil {
		return fmt.Errorf("harddelete: sweep %s: %w", tableName, res.Error)
	}
	logger.Info("harddelete: swept table",
		"table", tableName, "column", s.column,
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
	//
	// Nine tables below (review_reactions, review_replies, review_media,
	// loyalty_transactions, campaign_recipients, gift_card_transactions,
	// coupon_usage, ticket_replies, product_categories) have no store_id
	// column of their own — they carry only an FK to a parent that does
	// (reviews, customer_loyalties, campaigns, gift_cards, coupons,
	// tickets, products, respectively). Those are given a `via` instead of
	// a `column`, which reaches them with
	// `DELETE ... WHERE <fk> IN (SELECT id FROM <parent> WHERE store_id = ?)`.
	// They stay in the sweep explicitly — rather than being dropped in
	// favor of the parent's CASCADE — because sweepTable emits the
	// per-table audit event that is this pipeline's compliance trail;
	// dropping them would leave a GDPR hard-delete with no deletion record
	// for those tables. Each keeps its original position in the list:
	// coupon_usage must still precede orders (NO ACTION FK — surviving
	// rows would block the orders delete), and product_categories must
	// still precede categories (RESTRICT FK).

	sweeps := []sweepStep{
		// Audit logs for this store (compliance: deleted with the store per §15.2).
		{table: "audit_logs", column: "store_id"},
		// Subscription lifecycle audit.
		{table: "subscription_plan_change_audit", column: "store_id"},
		{table: "migration_fast_path_reviews", column: "store_id"},
		// Review-related child rows.
		{table: "review_reactions", via: &viaParent{fkColumn: "review_id", parentTable: "reviews"}},
		{table: "review_replies", via: &viaParent{fkColumn: "review_id", parentTable: "reviews"}},
		{table: "review_media", via: &viaParent{fkColumn: "review_id", parentTable: "reviews"}},
		{table: "reviews", column: "store_id"},
		// Loyalty.
		{table: "loyalty_transactions", via: &viaParent{fkColumn: "loyalty_id", parentTable: "customer_loyalties"}},
		{table: "customer_loyalties", column: "store_id"},
		{table: "loyalty_programs", column: "store_id"},
		// Campaigns.
		{table: "campaign_recipients", via: &viaParent{fkColumn: "campaign_id", parentTable: "campaigns"}},
		{table: "campaigns", column: "store_id"},
		// Gift cards.
		{table: "gift_card_transactions", via: &viaParent{fkColumn: "gift_card_id", parentTable: "gift_cards"}},
		{table: "gift_cards", column: "store_id"},
		// Coupons.
		{table: "coupon_usage", via: &viaParent{fkColumn: "coupon_id", parentTable: "coupons"}},
		{table: "coupons", column: "store_id"},
		// Orders (cascade deletes order_items, order_addresses, order_events).
		{table: "abandoned_carts", column: "store_id"},
		{table: "returns", column: "store_id"},
		{table: "orders", column: "store_id"},
		// Shipping, payment, tax configs.
		{table: "shipments", column: "store_id"},
		{table: "payment_transactions", column: "store_id"},
		{table: "payment_gateway_configs", column: "store_id"},
		{table: "tax_provider_configs", column: "store_id"},
		// Notifications.
		{table: "notification_preferences", column: "store_id"},
		{table: "notifications", column: "store_id"},
		// Tickets.
		{table: "ticket_replies", via: &viaParent{fkColumn: "ticket_id", parentTable: "tickets"}},
		{table: "tickets", column: "store_id"},
		// Pages.
		{table: "pages", column: "store_id"},
		// Media / product child rows (FK cascades from products).
		{table: "product_categories", via: &viaParent{fkColumn: "product_id", parentTable: "products"}},
		{table: "products", column: "store_id"},
		{table: "categories", column: "store_id"},
		// Customer profiles.
		{table: "customer_profiles", column: "store_id"},
		// Wishlists.
		{table: "wishlists", column: "store_id"},
		// Custom domains.
		{table: "custom_domains", column: "store_id"},
		// Branding.
		{table: "store_branding", column: "store_id"},
		// Push tokens.
		{table: "admin_push_tokens", column: "store_id"},
		// Subscription row.
		{table: "store_subscriptions", column: "store_id"},
		// Outbox events (best-effort — may already be consumed).
		{table: "outbox_events", column: "tenant_id"},
		// Idempotency keys (best-effort).
		{table: "idempotency_keys", column: "tenant_id"},
		// Stores table — must be last (other tables FK to it).
		{table: "stores", column: "id"},
	}

	for _, s := range sweeps {
		id := storeID
		if s.column == "tenant_id" {
			id = tenantID
		}
		if err := sweepTable(ctx, tx, emitter, logger, s, id, tenantID); err != nil {
			return err
		}
	}
	return nil
}
