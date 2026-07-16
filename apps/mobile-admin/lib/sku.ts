/**
 * CreateProductVariantInput.SKU is `binding:"required,max=100"` — a product
 * cannot be created without one. When the merchant leaves SKU blank we derive
 * a stable one from the title rather than fail the request.
 */
export function deriveSku(title: string): string {
  return `${title.trim().toUpperCase().replace(/[^A-Z0-9]+/g, "-").slice(0, 40)}-1`;
}

/** SKU for a new variant combination: base slug + sanitised value suffixes. */
export function deriveVariantSku(title: string, values: string[]): string {
  const base = title.trim().toUpperCase().replace(/[^A-Z0-9]+/g, "-").slice(0, 40);
  const suffix = values
    .map((v) => v.trim().toUpperCase().replace(/[^A-Z0-9]+/g, "-"))
    .join("-");
  return suffix ? `${base}-${suffix}` : `${base}-1`;
}
