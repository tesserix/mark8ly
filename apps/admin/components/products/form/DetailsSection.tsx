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

// RHF's register() returns only { name, onChange, onBlur, ref } — no value
// and no defaultValue — so a server-rendered form emits EMPTY inputs and the
// values appear only once the client has hydrated and RHF has written them
// through its refs. On the product page that showed as a real flash: the
// heading and breadcrumb (plain JSX) were correct immediately while Title sat
// empty and Handle showed its placeholder, which reads exactly like a product
// with no data — or a failed load.
//
// Passing defaultValue puts the value in the server HTML. It affects nothing
// after hydration: RHF owns the input imperatively from then on, and a later
// reset() sets .value directly rather than consulting the attribute.
export interface DetailsSectionProps {
  mode: "create" | "edit";
  /**
   * Title only. The create form asks for the few things needed to make the
   * product exist and then redirects into edit, so handle and description
   * are asked for where there is room for them rather than up front —
   * handle auto-generates from the title anyway.
   */
  compact?: boolean;
}

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

export function DetailsSection({ mode, compact = false }: DetailsSectionProps) {
  const { register, formState, getValues } = useFormContext<ProductFormValues>();

  const titleField = (
    <Field label="Title" error={formState.errors.title?.message}>
      <input
        type="text"
        {...register("title")}
          defaultValue={getValues("title")}
        className={`${inputClass} text-base`}
      />
    </Field>
  );

  if (compact) {
    return titleField;
  }

  return (
    <div className="flex flex-col gap-6">
      {titleField}

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
          defaultValue={getValues("handle")}
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
          defaultValue={getValues("description")}
          rows={6}
          className={`${inputClass} resize-vertical`}
        />
      </Field>
    </div>
  );
}
