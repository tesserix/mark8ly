// apps/admin/lib/validation/product-form.ts
//
// Shared Zod schema for the product detail form. Used by both the
// client form (react-hook-form via zodResolver) AND the server action
// (defense-in-depth second pass). Single source of truth for field
// constraints and error messages.

import { z } from "zod";

export const productFormSchema = z.object({
  title: z
    .string()
    .trim()
    .min(1, "Title is required")
    .max(300, "Title is too long"),
  handle: z
    .string()
    .trim()
    .max(200, "Handle is too long")
    .regex(
      /^[a-z0-9-]*$/,
      "Handle may only contain lowercase letters, numbers, and dashes",
    )
    .optional()
    .or(z.literal("")),
  description: z
    .string()
    .max(5000, "Description is too long")
    .optional()
    .or(z.literal("")),
  status: z.enum(["draft", "active", "archived"]),
  price: z
    .string()
    .regex(/^\d+(\.\d{1,2})?$/, "Enter a valid price, e.g. 19.99"),
  inventoryQuantity: z
    .string()
    .regex(/^\d+$/, "Enter a whole number"),
  sku: z
    .string()
    .trim()
    .max(100, "SKU is too long")
    .optional()
    .or(z.literal("")),
  categoryIds: z.array(z.string().uuid()),
});

export type ProductFormValues = z.infer<typeof productFormSchema>;
