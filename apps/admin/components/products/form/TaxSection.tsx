"use client";

// TaxSection — the product's tax settings, collapsed by default.
//
// Tax used to be one of five tabs. That spent permanent navigation on
// something most merchants set once or never: the store already carries a
// country default, and a product only departs from it for imported or
// bundled goods. A tab also hides whether the section holds anything until
// you click it, which is the failure the summary line below fixes — the
// merchant can see at a glance that this product is exempt, or carries an
// HSN code, without opening anything.
//
// Kept on the product rather than moved to store settings: a per-product
// override is rare, not obsolete.

import { useId, useState } from "react";
import { useFormContext } from "react-hook-form";

import type { ProductFormValues } from "@/lib/validation/product-form";
import { taxSummary } from "@/lib/products/tax-summary";
import { TaxTab } from "./TaxTab";

export interface TaxSectionProps {
  /**
   * Optional: this section renders inline on the product page, so it runs for
   * every product — including callers that never had a country to give.
   * An unknown country falls back to the generic strategy rather than
   * taking the whole form down with it.
   */
  storeCountryCode?: string;
}

function strategyFor(countryCode: string): "india_gst" | "taxjar" | "flat_rate" {
  if (countryCode === "IN") return "india_gst";
  if (countryCode === "US") return "taxjar";
  return "flat_rate";
}

export function TaxSection({ storeCountryCode }: TaxSectionProps) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const { watch } = useFormContext<ProductFormValues>();

  const summary = taxSummary(
    {
      taxCode: watch("taxCode"),
      taxCategory: watch("taxCategory"),
      taxRateOverride: watch("taxRateOverride"),
    },
    strategyFor((storeCountryCode ?? "").toUpperCase()),
  );

  return (
    <section className="border-t border-border-subtle pt-6">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-baseline justify-between gap-4 rounded-md py-1 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <span className="flex items-center gap-2">
          <svg
            width="10"
            height="10"
            viewBox="0 0 10 10"
            aria-hidden="true"
            className={`text-[color:var(--ink-900)]/40 transition-transform motion-reduce:transition-none ${open ? "rotate-180" : ""}`}
          >
            <path d="M2 4l3 3 3-3" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          <span className="font-serif text-lg text-[color:var(--ink-900)]">Tax</span>
        </span>
        {/* The summary is the whole point of the collapse. Muted when this
            product just follows the store, ink when it does not — one
            weight change, no badge, no second colour. */}
        <span
          className={
            summary.isOverridden
              ? "text-sm text-[color:var(--ink-900)]"
              : "text-sm text-[color:var(--ink-900)]/40"
          }
        >
          {summary.text}
        </span>
      </button>

      {open && (
        <div id={panelId} className="pt-6">
          <TaxTab storeCountryCode={storeCountryCode ?? ""} />
        </div>
      )}
    </section>
  );
}
