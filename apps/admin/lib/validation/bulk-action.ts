// apps/admin/lib/validation/bulk-action.ts
//
// Zod schema for bulk product actions. Enforces the 100-id cap at
// the server action layer (defense-in-depth — hook + backend also cap).

import { z } from "zod";

export const MAX_BULK_IDS = 100;

export const bulkActionSchema = z.object({
  product_ids: z
    .array(z.string().min(1))
    .min(1, "Select at least one product")
    .max(MAX_BULK_IDS, `Cannot bulk-edit more than ${MAX_BULK_IDS} products at once`),
});

export type BulkActionInput = z.infer<typeof bulkActionSchema>;
