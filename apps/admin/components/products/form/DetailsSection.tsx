"use client";

// DetailsSection — what the product is called and how it reads.
//
// Title, handle and description: the fields a merchant fills first and
// revisits least. Status and categories moved OUT of here and into the
// sticky rail, where they stay reachable at any scroll depth — a merchant
// editing a long description should not have to scroll back to the top to
// publish.

import { useFormContext } from "react-hook-form";

import type { ProductFormValues } from "@/lib/validation/product-form";
import { Field } from "./Field";

export interface DetailsSectionProps {
  mode: "create" | "edit";
}

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

export function DetailsSection({ mode }: DetailsSectionProps) {
  const { register, formState } = useFormContext<ProductFormValues>();

  return (
    <div className="flex flex-col gap-6">
      <Field label="Title" error={formState.errors.title?.message}>
        <input
          type="text"
          {...register("title")}
          className={`${inputClass} text-base`}
        />
      </Field>

      <Field
        label="Handle"
        helper={
          mode === "create"
            ? "Leave empty to auto-generate from the title. Lives at your-store.mark8ly.com/products/<handle>."
            : "Changing the handle breaks existing links. Lives at your-store.mark8ly.com/products/<handle>."
        }
        error={formState.errors.handle?.message}
      >
        <input
          type="text"
          {...register("handle")}
          placeholder="linen-shirt"
          className={inputClass}
        />
      </Field>

      <Field
        label="Description"
        helper="Shown on the product page below the title."
        error={formState.errors.description?.message}
      >
        <textarea
          {...register("description")}
          rows={6}
          className={`${inputClass} resize-vertical`}
        />
      </Field>
    </div>
  );
}
