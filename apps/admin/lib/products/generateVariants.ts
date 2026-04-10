// apps/admin/lib/products/generateVariants.ts
//
// Pure helper that derives the full variant matrix from an option
// draft. Existing variants are preserved by matching sorted variant
// keys; orphaned variants (those whose key no longer matches any
// combination) are returned in `removedIds` so the caller can forward
// them to the backend as deletions.

import { buildVariantKey } from "./variantKey";

export const MAX_VARIANTS = 500;

export interface OptionDraft {
  name: string;
  values: string[];
}

export interface VariantDefaults {
  price: string;
  sku: string;
  stock: number;
  weight: number;
}

export interface VariantDraft extends VariantDefaults {
  key: string;
  id?: string;
  variantImageId?: string | null;
}

export interface GenerateVariantsResult {
  variants: VariantDraft[];
  removedIds: string[];
}

export function generateVariants(
  options: OptionDraft[],
  existing: VariantDraft[],
  defaults: VariantDefaults,
): GenerateVariantsResult {
  if (options.length === 0) {
    const removedIds = existing
      .filter((v): v is VariantDraft & { id: string } => typeof v.id === "string")
      .map((v) => v.id);
    return { variants: [], removedIds };
  }

  // Cartesian product of option values, in declared-option order.
  let combinations: Array<{ name: string; value: string }[]> = [[]];
  for (const option of options) {
    const next: Array<{ name: string; value: string }[]> = [];
    for (const combo of combinations) {
      for (const value of option.values) {
        next.push([...combo, { name: option.name, value }]);
      }
    }
    combinations = next;
  }

  if (combinations.length > MAX_VARIANTS) {
    throw new Error(
      `Too many variants: ${combinations.length}. Maximum is ${MAX_VARIANTS}.`,
    );
  }

  const existingByKey = new Map(existing.map((v) => [v.key, v]));
  const nextVariants: VariantDraft[] = combinations.map((pairs) => {
    const key = buildVariantKey(pairs);
    const prior = existingByKey.get(key);
    if (prior) {
      existingByKey.delete(key);
      return { ...prior, key };
    }
    return { key, ...defaults };
  });

  const removedIds = Array.from(existingByKey.values())
    .filter((v): v is VariantDraft & { id: string } => typeof v.id === "string")
    .map((v) => v.id);

  return { variants: nextVariants, removedIds };
}
