"use client";

import { useFormContext, useWatch } from "react-hook-form";
import { OptionsEditor, type OptionDraft } from "@/components/products/options/OptionsEditor";
import type { ProductFormValues } from "@/lib/validation/product-form";

export function OptionsTab() {
  const { control, setValue, formState } = useFormContext<ProductFormValues>();
  const options = (useWatch({ control, name: "options" }) as OptionDraft[] | undefined) ?? [];
  const variantError = formState.errors.variants?.message as string | undefined;

  return (
    <div className="flex flex-col gap-6">
      {variantError && (
        <div
          role="alert"
          className="rounded-md border border-[color:var(--signal)] border-opacity-40 bg-[color:var(--paper-200)] px-4 py-3 text-sm text-[color:var(--signal)]"
        >
          {variantError}
        </div>
      )}
      <OptionsEditor
        value={options}
        onChange={(next) =>
          setValue("options", next, { shouldDirty: true, shouldValidate: false })
        }
      />
    </div>
  );
}
