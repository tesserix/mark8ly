import { z } from "zod";
import { dataOnly } from "../schema-helpers";

/**
 * Wire-truthful, from admin/stores.go AdminStoreResponse. Verified against a
 * real prod response 2026-07-15.
 *
 * NOTE: unlike the list endpoints, GET /stores returns `{data}` with NO meta,
 * so this uses dataOnly rather than paginated.
 */
export const storeSchema = z.object({
  id: z.string(),
  name: z.string(),
  slug: z.string(),
  country_code: z.string(),
  currency_code: z.string(),
  status: z.string(),
});

export type Store = z.infer<typeof storeSchema>;

export const storesResponseSchema = dataOnly(storeSchema);
