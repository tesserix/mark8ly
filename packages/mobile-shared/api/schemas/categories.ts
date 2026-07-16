import { z } from "zod";
import { dataOnly } from "../schema-helpers";

/**
 * Wire truth for the admin category endpoints.
 *
 * TWO shapes share the name "category" — do not conflate them:
 *  - `categoryRefSchema` (AdminCategoryRef, dto.go:165) is what a PRODUCT
 *    embeds under `categories[]`: id/name/slug only.
 *  - `categorySchema` (AdminCategoryResponse, dto.go:14) is what
 *    `GET /categories` returns: the full record, including `parent_id`.
 *
 * Categories are a TREE. `parent_id` is a Go *string with omitempty, so a root
 * category OMITS the key — absent, never null.
 */
export const categoryRefSchema = z.object({
  id: z.string(),
  name: z.string(),
  slug: z.string(),
});
export type CategoryRef = z.infer<typeof categoryRefSchema>;

export const categorySchema = z.object({
  id: z.string(),
  store_id: z.string(),
  parent_id: z.string().optional(),
  name: z.string(),
  slug: z.string(),
  description: z.string().optional(),
  image_url: z.string().optional(),
  position: z.number(),
  is_active: z.boolean(),
  featured: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Category = z.infer<typeof categorySchema>;

/**
 * `GET /categories` returns `{data}` with NO meta (categories.go:44) — the same
 * envelope as /stores, NOT the `{data, meta}` of /products. Using `paginated`
 * here would invent a meta block the endpoint never sends.
 */
export const categoryListSchema = dataOnly(categorySchema);
export type CategoryListResponse = z.infer<typeof categoryListSchema>;
