"use client";

// apps/admin/components/products/form/TaxTab.tsx
//
// Tax classification tab on the product form. The concrete UI adapts
// to the store's country tax strategy, which mirrors the backend
// supported_countries seed:
//
//   india_gst : HSN code + GST rate select + Exempt switch
//   taxjar    : TaxJar TIC input (rate resolved server-side by TaxJar)
//   flat_rate : Tax category select + optional rate override
//
// The Zod schema (product-form.ts) keeps all three field names generic
// (taxCode, taxRateOverride, taxCategory). The backend's
// product.Product struct uses the matching fields. UI copy is purely
// presentational — storage and calculator semantics stay neutral.

import * as React from "react";
import { useFormContext } from "react-hook-form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { ProductFormValues } from "@/lib/validation/product-form";

// Radix Select cannot accept empty string as a value — use a sentinel
// for the "unset / use country default" option and translate at the
// form boundary so the Zod schema still sees "" when nothing is picked.
const UNSET = "__default__";

export interface TaxTabProps {
  storeCountryCode: string;
}

type Strategy = "india_gst" | "taxjar" | "flat_rate";

function strategyFor(countryCode: string): Strategy {
  const cc = countryCode.toUpperCase();
  if (cc === "IN") return "india_gst";
  if (cc === "US") return "taxjar";
  return "flat_rate";
}

// Keep country copy close to the seed in
// `000008_payments_shipping_tax.up.sql`. Used for contextual helper text.
const FLAT_RATE_COUNTRY_LABELS: Record<string, { rate: number; label: string }> = {
  GB: { rate: 20, label: "VAT" },
  DE: { rate: 19, label: "VAT" },
  FR: { rate: 20, label: "VAT" },
  IT: { rate: 22, label: "VAT" },
  ES: { rate: 21, label: "VAT" },
  NL: { rate: 21, label: "VAT" },
  CA: { rate: 5, label: "GST" },
  AU: { rate: 10, label: "GST" },
  SG: { rate: 9, label: "GST" },
  MY: { rate: 8, label: "SST" },
  TH: { rate: 7, label: "VAT" },
  PH: { rate: 12, label: "VAT" },
  ID: { rate: 11, label: "VAT" },
};

const INDIA_GST_RATES = [0, 3, 5, 12, 18, 28] as const;

const FIELD_CLASS =
  "h-10 w-full rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary transition-colors focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]";

const LABEL_CLASS =
  "text-xs uppercase tracking-widest text-foreground-tertiary";

const HELP_CLASS = "text-xs text-foreground-tertiary";

export function TaxTab({ storeCountryCode }: TaxTabProps): React.ReactElement {
  const strategy = strategyFor(storeCountryCode);
  const { register, watch, setValue, formState } =
    useFormContext<ProductFormValues>();

  const taxCategory = watch("taxCategory");
  const taxRateOverride = watch("taxRateOverride");

  const taxCodeErr = formState.errors.taxCode?.message as string | undefined;
  const taxRateErr = formState.errors.taxRateOverride?.message as
    | string
    | undefined;

  return (
    <div className="flex max-w-2xl flex-col gap-8">
      <header className="flex flex-col gap-2">
        <p className="eyebrow">Tax classification</p>
        <p className="text-sm text-foreground-secondary">
          {strategyBlurb(strategy, storeCountryCode)}
        </p>
      </header>

      {strategy === "india_gst" && (
        <IndiaGSTForm
          taxCategory={taxCategory}
          register={register}
          setValue={setValue}
          taxCodeErr={taxCodeErr}
          taxRateOverride={taxRateOverride}
        />
      )}

      {strategy === "taxjar" && (
        <TaxJarForm register={register} taxCodeErr={taxCodeErr} />
      )}

      {strategy === "flat_rate" && (
        <FlatRateForm
          countryCode={storeCountryCode}
          taxCategory={taxCategory}
          register={register}
          setValue={setValue}
          taxRateErr={taxRateErr}
        />
      )}
    </div>
  );
}

function strategyBlurb(strategy: Strategy, countryCode: string): string {
  if (strategy === "india_gst") {
    return "Your store is in India. GST is charged per product: supply the HSN code and the applicable GST rate so checkout can split into CGST/SGST or IGST.";
  }
  if (strategy === "taxjar") {
    return "Your store is in the US. TaxJar calculates US sales tax at checkout. Optionally classify this product with a TaxJar TIC (Tax Code) for category-specific rates; otherwise the default product class is used.";
  }
  const meta = FLAT_RATE_COUNTRY_LABELS[countryCode.toUpperCase()];
  if (meta) {
    return `Your store is in ${countryCode.toUpperCase()} (${meta.label} ${meta.rate}% default). Choose a tax category for this product — use “Reduced”, “Zero-rated” or “Exempt” to deviate from the default rate, and optionally set a rate override.`;
  }
  return "Choose a tax category for this product and optionally set a rate override. The store's country default rate is applied when no override is set.";
}

// ── India GST form ──────────────────────────────────────────────────────

interface IndiaGSTFormProps {
  taxCategory: ProductFormValues["taxCategory"];
  register: ReturnType<
    typeof useFormContext<ProductFormValues>
  >["register"];
  setValue: ReturnType<typeof useFormContext<ProductFormValues>>["setValue"];
  taxCodeErr?: string;
  taxRateOverride: string | undefined;
}

function IndiaGSTForm({
  taxCategory,
  register,
  setValue,
  taxCodeErr,
  taxRateOverride,
}: IndiaGSTFormProps): React.ReactElement {
  const exempt = taxCategory === "exempt";
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <label htmlFor="tax-code" className={LABEL_CLASS}>
          HSN code
        </label>
        <input
          id="tax-code"
          type="text"
          placeholder="e.g. 6109 or 8517.12.00"
          maxLength={32}
          className={FIELD_CLASS}
          {...register("taxCode")}
          disabled={exempt}
        />
        {taxCodeErr && (
          <p className="text-xs text-[color:var(--signal)]">{taxCodeErr}</p>
        )}
        <p className={HELP_CLASS}>
          Harmonised System of Nomenclature code. 4–8 digits is typical.
          Surfaces on GST invoices.
        </p>
      </div>

      <div className="flex flex-col gap-1.5">
        <span id="tax-rate-label" className={LABEL_CLASS}>
          GST rate
        </span>
        <Select
          value={
            taxRateOverride && taxRateOverride.length > 0
              ? taxRateOverride
              : UNSET
          }
          disabled={exempt}
          onValueChange={(next) =>
            setValue(
              "taxRateOverride",
              next === UNSET ? "" : next,
              { shouldDirty: true },
            )
          }
        >
          <SelectTrigger
            aria-labelledby="tax-rate-label"
            className="w-full"
          >
            <SelectValue placeholder="Use country default" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={UNSET}>Use country default</SelectItem>
            {INDIA_GST_RATES.map((r) => (
              <SelectItem key={r} value={String(r)}>
                {r}%
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className={HELP_CLASS}>
          Applied per product. Same-state sales split into CGST + SGST;
          inter-state sales produce a single IGST line.
        </p>
      </div>

      <ExemptToggle
        checked={exempt}
        onChange={(next) => {
          setValue("taxCategory", next ? "exempt" : undefined, {
            shouldDirty: true,
          });
          if (next) {
            setValue("taxRateOverride", "", { shouldDirty: true });
          }
        }}
      />
    </div>
  );
}

// ── TaxJar form ─────────────────────────────────────────────────────────

interface TaxJarFormProps {
  register: ReturnType<
    typeof useFormContext<ProductFormValues>
  >["register"];
  taxCodeErr?: string;
}

function TaxJarForm({
  register,
  taxCodeErr,
}: TaxJarFormProps): React.ReactElement {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor="tax-code" className={LABEL_CLASS}>
        TaxJar product tax code
      </label>
      <input
        id="tax-code"
        type="text"
        placeholder="e.g. 20010 (clothing) or leave blank for default"
        maxLength={32}
        className={FIELD_CLASS}
        {...register("taxCode")}
      />
      {taxCodeErr && (
        <p className="text-xs text-[color:var(--signal)]">{taxCodeErr}</p>
      )}
      <p className={HELP_CLASS}>
        Optional. Leave blank to use TaxJar's default product class for
        this store. See{" "}
        <a
          href="https://developers.taxjar.com/api/reference/#get-list-tax-categories"
          target="_blank"
          rel="noopener noreferrer"
          className="text-[color:var(--moss-700)] underline-offset-4 hover:underline"
        >
          TaxJar tax categories
        </a>
        .
      </p>
    </div>
  );
}

// ── Flat-rate form ──────────────────────────────────────────────────────

interface FlatRateFormProps {
  countryCode: string;
  taxCategory: ProductFormValues["taxCategory"];
  register: ReturnType<
    typeof useFormContext<ProductFormValues>
  >["register"];
  setValue: ReturnType<typeof useFormContext<ProductFormValues>>["setValue"];
  taxRateErr?: string;
}

function FlatRateForm({
  countryCode,
  taxCategory,
  register,
  setValue,
  taxRateErr,
}: FlatRateFormProps): React.ReactElement {
  const meta = FLAT_RATE_COUNTRY_LABELS[countryCode.toUpperCase()];
  const defaultRate = meta?.rate;
  const exempt = taxCategory === "exempt" || taxCategory === "zero_rated";

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <span id="tax-category-label" className={LABEL_CLASS}>
          Tax category
        </span>
        <Select
          value={taxCategory ?? "standard"}
          onValueChange={(next) => {
            const nextCat = next as NonNullable<
              ProductFormValues["taxCategory"]
            >;
            setValue("taxCategory", nextCat, { shouldDirty: true });
            if (nextCat === "exempt" || nextCat === "zero_rated") {
              setValue("taxRateOverride", "", { shouldDirty: true });
            }
          }}
        >
          <SelectTrigger
            aria-labelledby="tax-category-label"
            className="w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="standard">
              Standard (country default)
            </SelectItem>
            <SelectItem value="reduced">Reduced</SelectItem>
            <SelectItem value="zero_rated">Zero-rated</SelectItem>
            <SelectItem value="exempt">Exempt</SelectItem>
          </SelectContent>
        </Select>
        <p className={HELP_CLASS}>
          Picks the tier applied at checkout. Standard uses the country
          default; Reduced honours the rate override below.
        </p>
      </div>

      <div className="flex flex-col gap-1.5">
        <label htmlFor="tax-rate" className={LABEL_CLASS}>
          Rate override (%)
        </label>
        <input
          id="tax-rate"
          type="text"
          inputMode="decimal"
          placeholder={
            defaultRate != null
              ? `Leave blank to use ${defaultRate}%`
              : "Leave blank to use country default"
          }
          className={FIELD_CLASS}
          disabled={exempt}
          {...register("taxRateOverride")}
        />
        {taxRateErr && (
          <p className="text-xs text-[color:var(--signal)]">{taxRateErr}</p>
        )}
        <p className={HELP_CLASS}>
          {exempt
            ? "Not applicable — this product is zero-rated or exempt."
            : "Whole number or one decimal place, e.g. 10 or 9.5."}
        </p>
      </div>
    </div>
  );
}

// ── Shared: exempt toggle ───────────────────────────────────────────────

interface ExemptToggleProps {
  checked: boolean;
  onChange: (next: boolean) => void;
}

function ExemptToggle({
  checked,
  onChange,
}: ExemptToggleProps): React.ReactElement {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <input
        type="checkbox"
        className="mt-0.5 h-4 w-4 cursor-pointer accent-[color:var(--moss-700)]"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="flex flex-col gap-0.5">
        <span className="text-sm text-foreground">Tax exempt</span>
        <span className={HELP_CLASS}>
          Excludes this product from GST calculation at checkout.
        </span>
      </span>
    </label>
  );
}
