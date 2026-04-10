// apps/admin/lib/products/variantKey.ts
//
// Pure helpers for building and parsing the canonical "variant key"
// used to identify a variant by its option-value combination. The key
// is stable under option reordering because pairs are sorted by option
// name before joining. Format: `Name1=Value1|Name2=Value2`.

export interface OptionValuePair {
  name: string;
  value: string;
}

export function buildVariantKey(pairs: OptionValuePair[]): string {
  if (pairs.length === 0) return "";
  const sorted = [...pairs].sort((a, b) => a.name.localeCompare(b.name));
  return sorted.map((p) => `${p.name}=${p.value}`).join("|");
}

export function parseVariantKey(key: string): OptionValuePair[] {
  if (key === "") return [];
  return key.split("|").map((segment) => {
    const eq = segment.indexOf("=");
    if (eq === -1) return { name: segment, value: "" };
    return { name: segment.slice(0, eq), value: segment.slice(eq + 1) };
  });
}
