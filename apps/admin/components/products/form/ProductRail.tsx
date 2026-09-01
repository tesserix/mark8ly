"use client";

// ProductRail — status, categories and the actions, kept in view.
//
// These were spread across the top of the General tab and a footer at the
// bottom of the form. On a scrolling page that means a merchant editing a
// long description has to travel to publish or save. The rail is the one
// piece of chrome that earns being sticky: it is the answer to "is this
// live, and have I saved it".
//
// Asymmetric by design — a narrow rail beside a wider column, not a
// centred form.

import type { ReactNode } from "react";
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
  /** Save / delete / discard, composed by the form. */
  actions: ReactNode;
}

export function ProductRail({ categories, storeId, actions }: ProductRailProps) {
  const { formState, watch, setValue } = useFormContext<ProductFormValues>();

  return (
    // A hairline down the left edge is what makes this read as a COLUMN
    // rather than controls floating in whitespace. Without it the sticky
    // Save button appears to dangle in the middle of the page as the long
    // left column scrolls past it — the elements were right, the boundary
    // was missing. Hairline rather than a card, per the system.
    <aside className="lg:sticky lg:top-8 lg:self-start lg:border-l lg:border-border-subtle lg:pl-8">
      <div className="flex flex-col gap-6">
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

        <div className="border-t border-border-subtle pt-6">{actions}</div>
        <p className="text-xs text-[color:var(--ink-900)]/40">
          Changes apply when you save.
        </p>
      </div>
    </aside>
  );
}
