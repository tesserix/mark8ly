"use client";

// apps/storefront/components/VariantSelector.tsx
//
// Interactive option picker for the product detail page. Renders each
// product option as a row of pill buttons. When all options are selected,
// resolves the matching variant and calls onVariantChange so the parent
// can update price, stock, and add-to-cart props.

import { useCallback, useMemo, useState } from "react";
import type {
  StorefrontProductOption,
  StorefrontVariant,
} from "@/lib/api/marketplace-api";

export interface VariantSelectorProps {
  options: StorefrontProductOption[];
  variants: StorefrontVariant[];
  onVariantChange: (variant: StorefrontVariant | null) => void;
}

export function VariantSelector({
  options,
  variants,
  onVariantChange,
}: VariantSelectorProps) {
  // Map of option_name → selected value
  const [selected, setSelected] = useState<Record<string, string>>(() => {
    // Pre-select if there's only one value per option
    const initial: Record<string, string> = {};
    for (const opt of options) {
      if (opt.values.length === 1) {
        initial[opt.name] = opt.values[0]!.value;
      }
    }
    return initial;
  });

  const resolvedVariant = useMemo(() => {
    if (Object.keys(selected).length !== options.length) return null;
    return (
      variants.find((v) =>
        v.option_values.every(
          (ov) => selected[ov.option_name] === ov.value,
        ),
      ) ?? null
    );
  }, [selected, options.length, variants]);

  const handleSelect = useCallback(
    (optionName: string, value: string) => {
      const next = { ...selected, [optionName]: value };
      setSelected(next);

      // Resolve variant with the new selection
      if (Object.keys(next).length === options.length) {
        const match =
          variants.find((v) =>
            v.option_values.every(
              (ov) => next[ov.option_name] === ov.value,
            ),
          ) ?? null;
        onVariantChange(match);
      } else {
        onVariantChange(null);
      }
    },
    [selected, options.length, variants, onVariantChange],
  );

  // For each option value, check if selecting it could lead to an
  // in-stock variant (given current selections for other options).
  const isValueAvailable = useCallback(
    (optionName: string, value: string): boolean => {
      const hypothetical = { ...selected, [optionName]: value };
      const otherKeys = Object.keys(hypothetical).filter(
        (k) => k !== optionName && hypothetical[k] !== undefined,
      );
      return variants.some((v) => {
        const matchesOthers = otherKeys.every((k) =>
          v.option_values.some(
            (ov) => ov.option_name === k && ov.value === hypothetical[k],
          ),
        );
        const matchesThis = v.option_values.some(
          (ov) => ov.option_name === optionName && ov.value === value,
        );
        return matchesOthers && matchesThis && v.in_stock;
      });
    },
    [selected, variants],
  );

  if (options.length === 0) return null;

  return (
    <div className="flex flex-col gap-5">
      {options.map((opt) => (
        <fieldset key={opt.name} className="flex flex-col gap-2">
          <legend className="text-xs font-semibold uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            {opt.name}
            {selected[opt.name] && (
              <span className="ml-2 normal-case tracking-normal opacity-80">
                — {selected[opt.name]}
              </span>
            )}
          </legend>
          <div className="flex flex-wrap gap-2">
            {opt.values.map((v) => {
              const isSelected = selected[opt.name] === v.value;
              const available = isValueAvailable(opt.name, v.value);
              return (
                <button
                  key={v.value}
                  type="button"
                  onClick={() => handleSelect(opt.name, v.value)}
                  disabled={!available && !isSelected}
                  aria-pressed={isSelected}
                  className={[
                    "rounded-md border px-4 py-2 text-sm transition-all duration-150",
                    "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
                    isSelected
                      ? "border-[color:var(--storefront-text,var(--ink-900))] bg-[color:var(--storefront-accent,var(--ink-900))] text-[color:var(--storefront-on-accent,var(--paper-200))] scale-[1.02]"
                      : available
                        ? "border-[color:var(--storefront-text,var(--ink-900))]/20 text-[color:var(--storefront-text,var(--ink-900))] hover:border-[color:var(--storefront-text,var(--ink-900))]/50 active:scale-95"
                        : "border-[color:var(--storefront-text,var(--ink-900))]/10 text-[color:var(--storefront-text,var(--ink-900))] opacity-30 line-through cursor-not-allowed",
                  ].join(" ")}
                >
                  {v.value}
                </button>
              );
            })}
          </div>
        </fieldset>
      ))}
      {resolvedVariant && !resolvedVariant.in_stock && (
        <p className="text-sm text-[color:var(--signal,#C23B22)]">
          This combination is out of stock
        </p>
      )}
    </div>
  );
}
