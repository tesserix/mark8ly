"use client";

import { useFormContext, useWatch } from "react-hook-form";
import { OptionsEditor, type OptionDraft } from "@/components/products/options/OptionsEditor";
import type { ProductFormValues } from "@/lib/validation/product-form";

export function OptionsTab() {
  const { control, setValue, formState } = useFormContext<ProductFormValues>();
  const options = (useWatch({ control, name: "options" }) as OptionDraft[] | undefined) ?? [];
  const variantError = formState.errors.variants?.message as string | undefined;
  const optionErrors = options.map((_, i) => {
    const e = formState.errors.options?.[i];
    return e?.values?.message ?? e?.name?.message;
  });

  return (
    <div className="flex flex-col gap-6">
      {variantError && (
        <div
          role="alert"
          className="rounded-md border border-[color:var(--signal)]/40 bg-[color:var(--paper-200)] px-4 py-3 text-sm text-[color:var(--signal)]"
        >
          {variantError}
        </div>
      )}
      <OptionsEditor
        value={options}
        errors={optionErrors}
        onChange={(next) =>
          // Once a save has failed, re-check as they fix it so the message clears.
          setValue("options", next, { shouldDirty: true, shouldValidate: formState.isSubmitted })
        }
      />
    </div>
  );
}
