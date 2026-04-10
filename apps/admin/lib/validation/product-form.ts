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
  // M7c: full aggregate fields — all optional with `.default([])` so M7b
  // callers that omit them parse successfully.
  options: z
    .array(
      z.object({
        id: z.string().optional(),
        name: z.string().min(1),
        values: z.array(
          z.object({
            id: z.string().optional(),
            value: z.string().min(1),
          }),
        ),
      }),
    )
    .optional(),
  variants: z
    .array(
      z.object({
        id: z.string().optional(),
        key: z.string(),
        price: z.string(),
        sku: z.string(),
        stock: z.number().int().min(0),
        weight: z.number().min(0),
        variantImageId: z.string().nullable().optional(),
        optionValues: z.array(
          z.object({
            optionName: z.string(),
            value: z.string(),
          }),
        ),
      }),
    )
    .optional(),
  media: z
    .array(
      z.object({
        id: z.string(),
        url: z.string(),
        alt: z.string(),
        position: z.number().int(),
        variant_id: z.string().nullable(),
        storage_key: z.string(),
        gcs_path_original: z.string(),
      }),
    )
    .optional(),
  removed_variant_ids: z.array(z.string()).optional(),
});

export type ProductFormValues = z.infer<typeof productFormSchema>;
