"use client";

// ProductRail — the product's metadata: status and categories.
//
// It used to carry Save / Discard / Delete as well, and to be sticky with a
// hairline down its left edge. Both are gone. Save now lives in a docked
// action bar under the page header (see ProductForm), which is what actually
// answers "have I saved this" at any scroll position. A border around a
// floating control only frames the float; it does not anchor it.
//
// The rule went with it. `self-start` sized this box to its own content, so
// the border was ~450px tall beside a 2000px column — a stub, not a boundary.
// It was also invisible in practice: --border-subtle is --paper-300 on a
// --paper-200 page, roughly 2% luminance at 1px, so it rendered as nothing at
// all in production. The column already reads as metadata through width
// asymmetry and the grid gap, which is the editorial convention regardless —
// magazines divide with whitespace, not rules. A vertical rule is the visual
// grammar of a sidebar panel, which this system lists as an anti-reference.

import { useFormContext } from "react-hook-form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { AdminCategory } from "@/lib/api/marketplace-api";
import type { ProductFormValues } from "@/lib/validation/product-form";
import { ProductCategoriesPicker } from "../ProductCategoriesPicker";
import { Field } from "./Field";

export interface ProductRailProps {
  categories: AdminCategory[];
  storeId?: string;
}

export function ProductRail({ categories, storeId }: ProductRailProps) {
  const { formState, watch, setValue } = useFormContext<ProductFormValues>();

  return (
    <aside className="flex flex-col gap-6">
      <Field label="Status" error={formState.errors.status?.message}>
        <Select
          value={watch("status")}
          onValueChange={(value) =>
            setValue("status", value as ProductFormValues["status"], {
              shouldDirty: true,
            })
          }
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="draft">Draft</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="archived">Archived</SelectItem>
          </SelectContent>
        </Select>
      </Field>

      <Field label="Categories" error={formState.errors.categoryIds?.message}>
        <ProductCategoriesPicker
          allCategories={categories}
          selectedIds={watch("categoryIds")}
          onChange={(ids) => setValue("categoryIds", ids, { shouldDirty: true })}
          storeId={storeId}
        />
      </Field>
    </aside>
  );
}
