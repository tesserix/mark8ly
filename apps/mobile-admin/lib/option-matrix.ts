import type { ProductDetail, ProductVariant } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateProductOptionBody, VariantMatrixInput } from "@repo/mobile-shared/api/products";
import { primaryVariant } from "@/lib/product-display";
import { deriveVariantSku } from "@/lib/sku";

/**
 * Thrown instead of returning a matrix whenever the existing→desired mapping
 * is ambiguous. `variants` in the PATCH body is a FULL DESIRED MATRIX — the
 * backend soft-deletes any existing variant omitted from it — so guessing
 * wrong here means silently destroying a real variant's price/stock/sales
 * history. See the spec's Area 1.
 */
export class OptionMatrixError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OptionMatrixError";
  }
}

export interface OptionMatrixResult {
  options: UpdateProductOptionBody[];
  variants: VariantMatrixInput[];
}

type Tuple = { option_name: string; value: string }[];

/** Cartesian product of the option value lists, in declared axis order. */
function cartesian(options: UpdateProductOptionBody[]): Tuple[] {
  return options.reduce<Tuple[]>(
    (acc, opt) =>
      acc.flatMap((tuple) => opt.values.map((value) => [...tuple, { option_name: opt.name, value }])),
    [[]],
  );
}

/** Stable key for a tuple — order is fixed by the desired axis order. */
function tupleKey(tuple: Tuple): string {
  return tuple.map((t) => `${t.option_name}=${t.value}`).join("|");
}

/**
 * The tuple an existing variant occupies, projected onto the DESIRED axes.
 *
 * An axis the variant predates (it carries no value for that option name at
 * all) defaults to that axis's first declared value — the deterministic
 * "new axis" landing spot, and the only case where guessing is safe because
 * every variant lacking the axis defaults independently, keeping whatever
 * values it already had on its OTHER axes distinct from its siblings.
 *
 * An axis whose stored value no longer appears among the desired values
 * (the merchant deleted or renamed that value) can NOT be safely guessed —
 * a plain `values: string[]` request carries no id/position to disambiguate
 * "renamed" from "removed". Returning null here routes the variant into the
 * caller's unmapped bucket, which always throws rather than pick a slot.
 */
function existingTupleKey(variant: ProductVariant, desired: UpdateProductOptionBody[]): string | null {
  const byName = new Map(variant.option_values.map((ov) => [ov.option_name, ov.value]));
  const tuple: Tuple = [];
  for (const opt of desired) {
    const stored = byName.get(opt.name);
    if (stored === undefined) {
      tuple.push({ option_name: opt.name, value: opt.values[0]! });
      continue;
    }
    if (!opt.values.includes(stored)) return null;
    tuple.push({ option_name: opt.name, value: stored });
  }
  return tupleKey(tuple);
}

/**
 * Turns a desired option set into the full `variants` matrix to PATCH,
 * preserving every existing variant by id wherever its option tuple still
 * exists in the desired space. Never mutates `product` or its arrays.
 *
 * New combinations (no existing variant occupies that tuple) get a derived
 * SKU, the store's reference variant's price/currency, and zero stock.
 *
 * Throws `OptionMatrixError` — instead of emitting a possibly-wrong matrix —
 * when: an option has no values; two existing variants collide onto the
 * same desired tuple; or any existing variant's stored value can't be
 * resolved against the desired options (see `existingTupleKey`).
 */
export function buildOptionMatrix(
  product: ProductDetail,
  desiredOptions: UpdateProductOptionBody[],
): OptionMatrixResult {
  if (desiredOptions.length === 0) {
    throw new OptionMatrixError("At least one option is required.");
  }
  // Two options sharing a name would produce colliding tuple keys (the key is
  // built from option_name), silently corrupting the existing→desired mapping.
  const optionNames = new Set(desiredOptions.map((opt) => opt.name));
  if (optionNames.size !== desiredOptions.length) {
    throw new OptionMatrixError("Two options share a name.");
  }
  for (const opt of desiredOptions) {
    if (opt.values.length === 0) {
      throw new OptionMatrixError(`Option "${opt.name}" needs at least one value.`);
    }
    // A duplicated value yields two identical tuples, both resolving to the
    // same existing variant — the matrix would carry a real id twice and the
    // full-desired-matrix PATCH would be corrupt. Reject before expanding.
    if (new Set(opt.values).size !== opt.values.length) {
      throw new OptionMatrixError(`Option "${opt.name}" has duplicate values.`);
    }
  }

  const tuples = cartesian(desiredOptions);

  // Position-sorted, NOT array order — the wire does not return variants in
  // position order (verified live; see lib/product-display.ts), so
  // `variants[0]` is not reliably "the" reference variant.
  const reference = primaryVariant(product);
  const inheritedPrice = reference?.price ?? 0;
  const inheritedCurrency = reference?.currency_code ?? "AUD";

  const existingByTuple = new Map<string, ProductVariant>();
  const unmapped: ProductVariant[] = [];
  for (const variant of product.variants) {
    const key = existingTupleKey(variant, desiredOptions);
    if (key === null) {
      unmapped.push(variant);
      continue;
    }
    if (existingByTuple.has(key)) {
      throw new OptionMatrixError(
        "Two existing variants map to the same option combination — resolve the conflict before saving.",
      );
    }
    existingByTuple.set(key, variant);
  }

  if (unmapped.length > 0) {
    throw new OptionMatrixError(
      `${unmapped.length} existing variant(s) hold a value that no longer exists in the new options — resolve before saving.`,
    );
  }

  const variants: VariantMatrixInput[] = tuples.map((tuple) => {
    const existing = existingByTuple.get(tupleKey(tuple));
    if (existing) {
      return {
        id: existing.id,
        sku: existing.sku,
        price: existing.price,
        inventory_quantity: existing.inventory_quantity,
        currency_code: existing.currency_code,
        option_values: tuple,
      };
    }
    return {
      sku: deriveVariantSku(
        product.title,
        tuple.map((t) => t.value),
      ),
      price: inheritedPrice,
      inventory_quantity: 0,
      currency_code: inheritedCurrency,
      option_values: tuple,
    };
  });

  return { options: desiredOptions, variants };
}
