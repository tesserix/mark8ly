// Package tenantpurge deletes every row belonging to a single tenant across
// every marketplace-api domain table, in FK-safe order, inside one
// transaction. It is the destructive half of the tenant hard-delete flow —
// internal/billingarchive.Builder runs BEFORE this and persists the 7-year
// retention record that purge must never touch.
//
// # Design: plan is pure, execution is a thin wrapper
//
// purgePlan is a pure function — (tenantID, storeIDs) -> ordered
// []deleteStep — with no DB dependency, so the ORDER and SCOPE of every
// delete is unit-testable without a live Postgres connection (see
// purge_test.go). Purge itself just runs the plan inside db.Transaction.
//
// # Table list derivation
//
// The ordered table list in purgePlan was derived by reading every
// migration in services/marketplace-api/migrations/*.sql (000001 through
// 000096) — not by trusting a hand-copied list. That reading surfaced
// several corrections to a naively-copied table list:
//
//   - "webhook_events" (migration 000008) has NO tenant_id, store_id, or
//     order_id column at all — it cannot be scoped to a tenant. The
//     correctly-scoped Stripe webhook idempotency table is
//     "stripe_webhook_events" (migration 000043/000041 comment), which DOES
//     carry tenant_id/store_id and is what this package purges instead.
//   - "shipment_cancel_actions" and "refund_transactions_saga" are not
//     tables — they are columns ALTERed onto the existing "shipments" and
//     "refund_transactions" tables (migrations 000096, 000092-000094).
//   - Several tables have NO FK to stores(id) at all and so are NOT swept
//     by the final stores CASCADE: notifications, pages, customer_loyalties,
//     referrals, support_tickets, promo_redemptions, campaign_email_budget,
//     store_transactional_counter, warehouses, and more. These all need
//     explicit steps below; see the per-group comments.
//
// # Tables deliberately EXCLUDED from the plan
//
// Global reference data (never tenant-scoped, must never be touched):
//   - supported_countries, fx_rates, shipping_zones
//   - email_templates (shared transactional template catalog, no tenant_id)
//   - user_profiles (one row per GIP user, not owned by a single tenant)
//   - promo_codes (admin-created Stripe coupon catalog, no tenant_id)
//   - signup_anomaly_log (global daily cron marker, no tenant_id)
//
// Legally-protected / append-only tables — deleting these would either
// error (DB role has DELETE revoked) or defeat their entire purpose
// (records that must outlive the tenant for compliance):
//   - business_entity_attestations — migration 000045 REVOKEs DELETE from
//     both the marketplace_user role and PUBLIC; a DELETE against it
//     errors under the app's DB role.
//   - app_contract_attestations — migration 000075 mirrors the same
//     REVOKE DELETE pattern (Apple 4.2.6 attestation log).
//   - subscription_plan_change_audit — migration 000050 REVOKEs UPDATE,
//     DELETE FROM PUBLIC; append-only billing-change audit trail.
//   - billing_archive — populated by internal/billingarchive.Builder AFTER
//     a store hard-delete specifically so it SURVIVES the tenant's own
//     deletion (7-year GDPR/tax retention, §23.2). It is keyed by
//     original_tenant_id/original_store_id, not tenant_id/store_id — that
//     rename is itself a signal it has a different lifecycle. Purging it
//     here would delete the compliance record the purge is supposed to
//     leave behind.
//
// See task-1-report.md for the full per-table FK/scoping audit.
package tenantpurge

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// deleteStep is one DELETE statement in the purge plan.
type deleteStep struct {
	// table is the table this step deletes from — used for error context
	// and asserted on directly by the unit tests (order + scope).
	table string
	// sql is a raw parameterized DELETE statement using `?` placeholders.
	sql string
	// args are the positional args for sql's `?` placeholders, in order.
	args []any
}

// Purge deletes every row belonging to tenantID across every
// marketplace-api domain table, inside a single transaction, in FK-safe
// order. It never touches global reference tables or the legally-protected
// retention/append-only tables — see the package doc comment for the full
// list and rationale.
//
// Purge is idempotent: every delete is a `WHERE` clause that matches zero
// rows on a second run, so calling Purge twice for the same tenant is safe
// and returns nil both times.
func Purge(ctx context.Context, db *gorm.DB, tenantID string, storeIDs []string) error {
	if db == nil {
		return fmt.Errorf("tenantpurge: db must not be nil")
	}
	if tenantID == "" {
		return fmt.Errorf("tenantpurge: tenantID must not be empty")
	}

	steps := purgePlan(tenantID, storeIDs)

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, step := range steps {
			if err := tx.Exec(step.sql, step.args...).Error; err != nil {
				return fmt.Errorf("tenantpurge: delete from %s: %w", step.table, err)
			}
		}
		return nil
	})
}

// purgePlan builds the ordered list of deletes for tenantID/storeIDs. It has
// no DB dependency — every table name, FK edge, and scoping decision below
// is backed by a specific migration file, cited inline.
//
// globalDenyTables (never touched) and the append-only/legal-retention
// exclusions are documented in the package doc comment above and are
// enforced simply by never appearing in this function.
func purgePlan(tenantID string, storeIDs []string) []deleteStep {
	var steps []deleteStep

	// ------------------------------------------------------------------
	// Group 1 — Financial/audit leaves, FIRST. Order is load-bearing:
	// real-money tables reference each other with RESTRICT (the default
	// when a migration omits ON DELETE), so the referencing row must be
	// gone before the referenced row can be deleted.
	//
	//   refund_transactions.payment_transaction_id -> payment_transactions   RESTRICT (000008)
	//   platform_fee_ledger.order_id               -> orders                RESTRICT (000008, no ON DELETE)
	//   coupon_usage.order_id                      -> orders                RESTRICT (000009, no ON DELETE)
	//   payment_transactions.order_id              -> orders                RESTRICT (000008)
	//   shipments.order_id                         -> orders                RESTRICT (000008)
	//
	// order_tax_lines actually CASCADEs from orders (000008), but is listed
	// explicitly here for auditability — a redundant scoped delete is
	// harmless. webhook_events (000008) has no tenant/store/order column at
	// all and is intentionally excluded (see package doc); the correctly
	// scoped table is stripe_webhook_events (000043).
	// ------------------------------------------------------------------
	steps = append(steps,
		tenantScoped("refund_audit", tenantID),        // 000062: tenant_id, store_id
		tenantScoped("refund_transactions", tenantID), // 000008+000092-094: tenant_id
		tenantScoped("platform_fee_ledger", tenantID), // 000008: tenant_id, store_id
		subquery("order_tax_lines", "order_id", "orders", tenantID),
		tenantScoped("coupon_usage", tenantID),          // 000009: tenant_id
		tenantScoped("stripe_webhook_events", tenantID), // 000043: tenant_id (nullable)
		tenantScoped("payment_transactions", tenantID),  // 000008: tenant_id, store_id
		tenantScoped("shipments", tenantID),             // 000008: tenant_id, store_id
	)

	// ------------------------------------------------------------------
	// Group 2 — Order children, then orders itself.
	//
	//   return_items.order_item_id -> order_items  RESTRICT (000002)
	//   returns.order_id           -> orders        RESTRICT (000002)
	//   csv_import_jobs.store_id   -> stores         RESTRICT (000007, no ON DELETE)
	//
	// order_events/order_addresses/order_items all CASCADE from orders
	// (000002) and would be swept by the "orders" delete below; listed
	// explicitly anyway for auditability, same rationale as order_tax_lines.
	// abandoned_carts and csv_import_jobs have no FK to orders and are not
	// swept by anything else, so they need their own steps regardless.
	// ------------------------------------------------------------------
	steps = append(steps,
		subquery("return_items", "return_id", "returns", tenantID),
		tenantScoped("returns", tenantID), // 000002: tenant_id, store_id
		subquery("order_events", "order_id", "orders", tenantID),
		subquery("order_addresses", "order_id", "orders", tenantID),
		subquery("order_items", "order_id", "orders", tenantID),
		tenantScoped("orders", tenantID),          // 000002: tenant_id, store_id
		tenantScoped("abandoned_carts", tenantID), // 000002: tenant_id, store_id
		storeScoped("csv_import_jobs", storeIDs),  // 000007: store_id only, no tenant_id
	)

	// ------------------------------------------------------------------
	// Group 3 — Product/review subtree, then products, then categories.
	//
	//   product_categories.category_id -> categories  RESTRICT (000001)
	//   categories.store_id            -> stores        RESTRICT (000001)
	//   products.store_id              -> stores          RESTRICT (000001)
	//
	// product_options/product_option_values/product_variants/
	// variant_option_values/variant_stock/product_media all CASCADE from
	// products (directly or via a 2-level chain) and are swept by the
	// "products" delete below — no explicit steps needed for those.
	// product_categories, review_reactions/replies/media, reviews, and
	// wishlists are listed explicitly (children before their parent, per
	// design) even though most of these edges are also CASCADE, so a
	// second run / partial-failure retry stays correct either way.
	// ------------------------------------------------------------------
	steps = append(steps,
		subquery("product_categories", "product_id", "products", tenantID),
		subquery("review_reactions", "review_id", "reviews", tenantID),
		subquery("review_replies", "review_id", "reviews", tenantID),
		subquery("review_media", "review_id", "reviews", tenantID),
		tenantScoped("reviews", tenantID),                      // 000017: tenant_id, store_id
		tenantScoped("wishlists", tenantID),                    // 000018: tenant_id, store_id
		tenantScoped("product_notify_subscriptions", tenantID), // 000023: tenant_id
		tenantScoped("products", tenantID),                     // 000001: tenant_id, store_id
		tenantScoped("categories", tenantID),                   // 000001: tenant_id, store_id
	)

	// ------------------------------------------------------------------
	// Group 4 — vendors, AFTER products (products.vendor_id is NOT NULL
	// since 000028). No enforced DB FK exists from products.vendor_id to
	// vendors(id) — verified: no "REFERENCES vendors" anywhere in the
	// migrations — but the ordering is kept per the intended design.
	// vendors has tenant_id only (no store_id, no FK to stores at all;
	// 000027), so it is never swept by anything else.
	// ------------------------------------------------------------------
	steps = append(steps, tenantScoped("vendors", tenantID))

	// ------------------------------------------------------------------
	// Group 5 — Tenant-only tables with no FK to stores(id), so the final
	// stores CASCADE (group 6) does NOT reach them. Order among these is
	// mostly free except one real edge:
	//
	//   referrals.referrer_id/referee_id -> customer_loyalties  RESTRICT (000011, no ON DELETE)
	//
	// so referrals must precede customer_loyalties. loyalty_transactions
	// CASCADEs from customer_loyalties (000011) and needs no explicit step.
	// ------------------------------------------------------------------
	steps = append(steps,
		tenantScoped("tenant_sso_user_mappings", tenantID), // 000071: tenant_id (PK part)
		tenantScoped("tenant_sso_configs", tenantID),       // 000070: tenant_id (PK)
		tenantScoped("storefront_push_tokens", tenantID),   // 000022: tenant_id
		tenantScoped("admin_push_tokens", tenantID),        // 000021: tenant_id, store_id
		// break_glass_lockouts is intentionally NOT purged: in prod it is owned by
		// `postgres` (a manual-migration anomaly), so the marketplace_api role has
		// no DELETE privilege and including it aborts the whole single-tx purge
		// (SQLSTATE 42501). Its rows are ephemeral, HMAC'd-IP rate-limit lockouts
		// (self-expiring via locked_until) — safe to retain. See protectedTables
		// in purge_test.go. break_glass_accounts (below) IS owned by marketplace_api
		// and is still purged.
		tenantScoped("break_glass_accounts", tenantID), // 000072: tenant_id (PK)
		tenantScoped("enterprise_api_keys", tenantID),  // 000068: tenant_id, store_id
		tenantScoped("audit_logs", tenantID),           // 000035ish: tenant_id, store_id
		subquery("payment_action_reminders", "subscription_id", "store_subscriptions", tenantID),
		tenantScoped("trial_reminders", tenantID),             // 000088: tenant_id, store_id
		tenantScoped("warehouses", tenantID),                  // 000095: tenant_id, store_id
		tenantScoped("white_label_app_lifecycle", tenantID),   // 000048ish: tenant_id, store_id
		tenantScoped("white_label_app_state", tenantID),       // 000076: tenant_id, store_id
		tenantScoped("referrals", tenantID),                   // 000011: tenant_id, store_id — before customer_loyalties
		tenantScoped("customer_loyalties", tenantID),          // 000011: tenant_id, store_id
		tenantScoped("sea_manual_review_queue", tenantID),     // 000065: tenant_id, store_id
		tenantScoped("tax_validation_outage_log", tenantID),   // 000066: tenant_id (nullable)
		tenantScoped("migration_fast_path_reviews", tenantID), // 000051: tenant_id, store_id
		tenantScoped("customer_erasure_requests", tenantID),   // 000059: tenant_id, store_id
		storeScoped("promo_redemptions", storeIDs),            // 000061: store_id only, no tenant_id
		storeScoped("campaign_email_budget", storeIDs),        // 000047ish: store_id only, no tenant_id
		storeScoped("store_transactional_counter", storeIDs),  // 000064: store_id only, no tenant_id
		tenantScoped("notifications", tenantID),               // 000016: tenant_id, store_id — NOT cascade-linked to stores
		tenantScoped("pages", tenantID),                       // 000028ish: tenant_id, store_id — NOT cascade-linked to stores
		tenantScoped("support_tickets", tenantID),             // 000089: tenant_id, store_id (support_ticket_replies CASCADEs from it)
		tenantScoped("outbox_events", tenantID),               // 000001: tenant_id
		tenantScoped("idempotency_keys", tenantID),            // 000001: tenant_id
	)

	// ------------------------------------------------------------------
	// Group 6 — stores LAST. Its ON DELETE CASCADE sweeps every remaining
	// store-scoped config/child table in one statement: store_watermarks,
	// payment_gateway_configs, shipping_carrier_configs,
	// platform_fee_configs, tax_provider_configs, coupons,
	// customer_segments, campaigns (-> campaign_recipients),
	// gift_cards (-> gift_card_transactions), loyalty_programs,
	// customer_profiles (-> customer_addresses), custom_domains,
	// store_subscriptions (-> subscription_arbitrage_audit),
	// notification_preferences, tickets (-> ticket_replies), store_branding.
	// All of these CASCADE directly or transitively from stores(id) — see
	// task-1-report.md for the per-table citation — so no explicit steps
	// are listed for them; a redundant explicit delete would be harmless
	// but adds no safety since the FK graph already guarantees it.
	// ------------------------------------------------------------------
	steps = append(steps, tenantScoped("stores", tenantID)) // 000001: tenant_id (PK id)

	return steps
}

// tenantScoped builds a `DELETE FROM <table> WHERE tenant_id = ?` step.
func tenantScoped(table, tenantID string) deleteStep {
	return deleteStep{
		table: table,
		sql:   fmt.Sprintf("DELETE FROM %s WHERE tenant_id = ?", table),
		args:  []any{tenantID},
	}
}

// storeScoped builds a `DELETE FROM <table> WHERE store_id IN (...)` step
// for tables that carry store_id but no tenant_id column. When storeIDs is
// empty the clause becomes `IN (NULL)`, which matches zero rows — a safe,
// syntactically-valid no-op rather than an `IN ()` syntax error.
func storeScoped(table string, storeIDs []string) deleteStep {
	return deleteStep{
		table: table,
		sql:   fmt.Sprintf("DELETE FROM %s WHERE store_id IN (%s)", table, placeholders(len(storeIDs))),
		args:  toAnySlice(storeIDs),
	}
}

// subquery builds a `DELETE FROM <table> WHERE <fkCol> IN (SELECT id FROM
// <parentTable> WHERE tenant_id = ?)` step, for child tables that carry
// neither tenant_id nor store_id themselves (e.g. order_items only has
// order_id) and so must be scoped through their tenant-scoped parent.
func subquery(table, fkCol, parentTable, tenantID string) deleteStep {
	return deleteStep{
		table: table,
		sql: fmt.Sprintf(
			"DELETE FROM %s WHERE %s IN (SELECT id FROM %s WHERE tenant_id = ?)",
			table, fkCol, parentTable,
		),
		args: []any{tenantID},
	}
}

// placeholders returns n comma-joined `?` placeholders, or the literal
// "NULL" when n is zero (see storeScoped).
func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

// toAnySlice adapts a []string to []any for tx.Exec's variadic args.
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
