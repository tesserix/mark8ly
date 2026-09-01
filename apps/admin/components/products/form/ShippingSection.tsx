"use client";

// ShippingSection — how this product travels.
//
// Weight and dimensions used to sit on the General tab AND as three
// columns squeezed into one cell of the variant matrix. One home now.
//
// Known limit, stated rather than hidden: these fields write to the
// product's variant(s), because weight_grams / length_cm / width_cm /
// height_cm live on product_variants and there is no product-level
// column. For a single-variant product that is exact. Giving a product
// its own dimensions — so a T-shirt in S/M/L stops carrying three
// identical box sizes — needs a migration, and is deliberately not
// smuggled in with a layout change.

import { useFormContext } from "react-hook-form";

import type { ProductFormValues } from "@/lib/validation/product-form";
import { isUsableWeight } from "@/lib/products/usable-weight";
import { Field } from "./Field";

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

export function ShippingSection() {
  const { register, formState, watch } = useFormContext<ProductFormValues>();

  // Blank, whitespace, unparseable or non-positive all mean "no usable
  // weight" — each ends up as the store's fallback at checkout.
  const isWeightMissing = !isUsableWeight(watch("weightKg"));

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-4">
        <Field label="Weight (kg)" error={formState.errors.weightKg?.message}>
          <input
            type="text"
            inputMode="decimal"
            placeholder="0.5"
            {...register("weightKg")}
            className={inputClass}
          />
        </Field>
        <Field label="Length (cm)" error={formState.errors.lengthCm?.message}>
          <input type="text" inputMode="decimal" {...register("lengthCm")} className={inputClass} />
        </Field>
        <Field label="Width (cm)" error={formState.errors.widthCm?.message}>
          <input type="text" inputMode="decimal" {...register("widthCm")} className={inputClass} />
        </Field>
        <Field label="Height (cm)" error={formState.errors.heightCm?.message}>
          <input type="text" inputMode="decimal" {...register("heightCm")} className={inputClass} />
        </Field>
      </div>

      {isWeightMissing && (
        <p
          role="status"
          className="text-sm text-[color:var(--ink-900)]/60"
        >
          Without a weight, carriers quote against your store&rsquo;s fallback
          parcel weight. That is a guess, and a wrong one costs you the
          difference on every order.
        </p>
      )}
    </div>
  );
}
