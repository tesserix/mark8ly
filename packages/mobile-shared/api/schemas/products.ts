import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for the admin product endpoints. Verified 2026-07-16 against
 * ALL 161 products in the Bondi store (not a single sample).
 *
 * The app's previous Product type was entirely fictional: `name`, `price`,
 * `compare_at_price`, `sku`, `stock`, `thumbnail_url` — not one of those keys
 * exists on the wire. The real shape is `title` plus `variants[]` and
 * `media[]`, exactly like the web admin.
 *
 * Variant `price` is a shopspring/decimal.Decimal and arrives QUOTED
 * ("199", "19.99") — live proof of why `money` is a number|string union.
 */
export const variantOptionValueSchema = z.object({
  option_name: z.string(),
  option_value_id: z.string(),
  value: z.string(),
});

export const productVariantSchema = z.object({
  id: z.string(),
  sku: z.string(),
  price: money,
  compare_at_price: money.optional(),
  currency_code: z.string(),
  inventory_quantity: z.number(),
  inventory_policy: z.string(),
  option_values: z.array(variantOptionValueSchema),
  /**
   * The wire does NOT sort variants by position — a real product came back
   * as 2,3,4,0,1. Anything picking a "primary" variant must sort by this
   * field; variants[0] is not it. See lib/product-display.ts.
   */
  position: z.number(),
});
export type ProductVariant = z.infer<typeof productVariantSchema>;

export const productMediaSchema = z.object({
  id: z.string(),
  url: z.string(),
  storage_key: z.string(),
  alt: z.string().optional(),
  position: z.number(),
  media_type: z.string().optional(),
});
export type ProductMedia = z.infer<typeof productMediaSchema>;

/**
 * Response shape for a product option (`AdminProductOption`, dto.go:114-125).
 * `values` is an array of option-value objects `{id, value, position}`
 * (`AdminProductOptionValue`), NOT `string[]`.
 *
 * Do not confuse this with the REQUEST shape used when creating/updating a
 * product option (`CreateProductOptionInput`, validation.go:251-254), where
 * `values` really is `string[]`. That request/response asymmetry is a known
 * repeated trap on this project — mixing them up here is exactly the bug
 * that would blank the product list the moment any product has options.
 */
export const productOptionValueSchema = z.object({
  id: z.string(),
  value: z.string(),
  position: z.number(),
});

export const productOptionSchema = z.object({
  id: z.string(),
  name: z.string(),
  position: z.number(),
  values: z.array(productOptionValueSchema),
});
export type ProductOption = z.infer<typeof productOptionSchema>;

export const productSchema = z.object({
  id: z.string(),
  store_id: z.string(),
  handle: z.string(),
  title: z.string(),
  description: z.string().optional(),
  // Backend enum: draft | active | archived. NOT "inactive" — sending that
  // to ?status= is a 400 (verified live).
  status: z.string(),
  tags: z.array(z.string()),
  seo_title: z.string().optional(),
  seo_description: z.string().optional(),
  primary_category_id: z.string().optional(),
  categories: z.array(z.unknown()),
  options: z.array(productOptionSchema),
  variants: z.array(productVariantSchema),
  media: z.array(productMediaSchema),
  published_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Product = z.infer<typeof productSchema>;

/** The detail endpoint returns the same product object, unwrapped. */
export const productDetailSchema = productSchema;
export type ProductDetail = z.infer<typeof productDetailSchema>;

export const productListSchema = paginated(productSchema);
export type ProductListResponse = z.infer<typeof productListSchema>;
