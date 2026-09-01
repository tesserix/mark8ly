import type { ProductFormValues } from "@/lib/validation/product-form";

/**
 * What a product's tax settings amount to, in one line.
 *
 * Tax was a whole tab — permanent navigation for something most
 * merchants set once or never, since the store already carries a
 * country default. It is a collapsed section now, and this is the line
 * that has to earn the collapse: a merchant must be able to tell,
 * without opening it, whether this product does anything unusual.
 *
 * The summary therefore names the OVERRIDE when there is one, and says
 * plainly that the store default applies when there is not. It never
 * guesses at the store's actual rate — the admin does not have it here,
 * and inventing "GST 18%" would be worse than saying nothing.
 */
export interface TaxSummary {
  /** One line for the collapsed header. */
  text: string;
  /** True when this product departs from the store default. */
  isOverridden: boolean;
}

type TaxFields = Pick<
  ProductFormValues,
  "taxCode" | "taxCategory" | "taxRateOverride"
>;

export function taxSummary(
  values: TaxFields,
  strategy: "india_gst" | "taxjar" | "flat_rate",
): TaxSummary {
  const code = values.taxCode?.trim() ?? "";
  const category = values.taxCategory ?? "";
  const rate = values.taxRateOverride?.trim() ?? "";

  const parts: string[] = [];

  if (category === "exempt") {
    // Exempt is the one worth naming first however it was reached: it
    // means this product is not taxed at all.
    parts.push("Tax exempt");
  } else if (category && category !== "standard") {
    parts.push(category === "zero_rated" ? "Zero-rated" : "Reduced rate");
  }

  if (code) {
    parts.push(strategy === "india_gst" ? `HSN ${code}` : `Code ${code}`);
  }
  if (rate) {
    parts.push(`${rate}%`);
  }

  if (parts.length === 0) {
    return { text: "Using the store default", isOverridden: false };
  }
  return { text: parts.join(" · "), isOverridden: true };
}
