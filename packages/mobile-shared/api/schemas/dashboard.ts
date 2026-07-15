import { z } from "zod";
import { money } from "../schema-helpers";

/**
 * Wire-truthful, from admin/dashboard.go:22-86. Field names come from the Go
 * json tags, NOT from the TS types they replace — the old types invented
 * `name`/`total_sold`/`stock` and a 5-field checklist that never existed.
 *
 * `stats`, `setup_checklist` and the envelope were verified against a real
 * prod response 2026-07-15. `recent_orders`/`top_products`/`low_stock` came
 * back empty (the store has no orders), so those three item shapes are taken
 * from the Go DTOs.
 */
export const dashboardStatsSchema = z.object({
  revenue_today: money,
  revenue_week: money,
  revenue_month: money,
  revenue_change_pct: z.number(),
  revenue_trend: z.array(z.number()),
  orders_today: z.number(),
  orders_pending: z.number(),
  orders_fulfilled: z.number(),
  orders_cancelled: z.number(),
  customers_total: z.number(),
  customers_new_this_week: z.number(),
  pending_reviews: z.number(),
});

export const recentOrderSchema = z.object({
  id: z.string(),
  order_number: z.string(),
  customer_email: z.string(),
  grand_total: money,
  status: z.string(),
  created_at: z.string(),
});

export const topProductSchema = z.object({
  id: z.string(),
  title: z.string(),
  revenue: money,
  units_sold: z.number(),
  image_url: z.string().nullable(),
});

export const lowStockItemSchema = z.object({
  id: z.string(),
  title: z.string(),
  variant_title: z.string(),
  quantity: z.number(),
  low_stock_threshold: z.number(),
});

export const setupChecklistSchema = z.object({
  has_store: z.boolean(),
  has_brand_assets: z.boolean(),
  has_product: z.boolean(),
  has_storefront_theme: z.boolean(),
  has_payment_provider: z.boolean(),
  has_shipping_carrier: z.boolean(),
  has_return_policy: z.boolean(),
  has_custom_domain: z.boolean(),
});

export const dashboardResponseSchema = z.object({
  stats: dashboardStatsSchema,
  recent_orders: z.array(recentOrderSchema),
  top_products: z.array(topProductSchema),
  low_stock: z.array(lowStockItemSchema),
  setup_checklist: setupChecklistSchema,
});

export type DashboardStats = z.infer<typeof dashboardStatsSchema>;
export type RecentOrder = z.infer<typeof recentOrderSchema>;
export type TopProduct = z.infer<typeof topProductSchema>;
export type LowStockItem = z.infer<typeof lowStockItemSchema>;
export type SetupChecklist = z.infer<typeof setupChecklistSchema>;
export type DashboardResponse = z.infer<typeof dashboardResponseSchema>;
