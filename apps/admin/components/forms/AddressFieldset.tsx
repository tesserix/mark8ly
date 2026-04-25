"use client";

// AddressFieldset — shared warehouse / shipping address inputs backed by
// platform-api's /locations reference data. Country comes from the seeded
// countries list; State/Region is a dropdown when subdivisions are seeded
// for the chosen country, otherwise a free-form text input.
//
// Why this exists: ShippingConfigForm previously held 8 separate useStates
// for the address fields (one per input) and a free-form 2-letter country
// box that produced "Au" instead of "AU" in prod. Consolidating into a
// single value/onChange contract + driving country from /locations
// eliminates both class of bugs and gives every future address surface
// (storefront checkout, store-general settings, etc.) a single component
// to depend on.

import { useEffect, useState } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import {
  listCountries,
  listStatesByCountry,
  type Country,
  type State,
} from "@/lib/api/platform-api";

export interface AddressValue {
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal: string;
  country: string;
  phone: string;
}

export const EMPTY_ADDRESS: AddressValue = {
  name: "",
  line1: "",
  line2: "",
  city: "",
  region: "",
  postal: "",
  country: "",
  phone: "",
};

interface AddressFieldsetProps {
  value: AddressValue;
  onChange: (next: AddressValue) => void;
  /**
   * ISO 3166 alpha-2 country code used to pre-select the country dropdown
   * when `value.country` is empty. Typically `currentStore.country_code`.
   */
  defaultCountryCode?: string;
  /** Disables every input. Useful while a server action is pending. */
  disabled?: boolean;
  /** Prefix for input ids/labels so multiple fieldsets can coexist on the page. */
  idPrefix?: string;
  /** Suppresses the "Warehouse name" row when the consumer manages it elsewhere. */
  hideName?: boolean;
}

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";

export function AddressFieldset({
  value,
  onChange,
  defaultCountryCode,
  disabled = false,
  idPrefix = "addr",
  hideName = false,
}: AddressFieldsetProps) {
  const [countries, setCountries] = useState<Country[]>([]);
  const [states, setStates] = useState<State[]>([]);
  const [statesLoading, setStatesLoading] = useState(false);

  // One-shot fetch of the country reference list.
  useEffect(() => {
    let cancelled = false;
    listCountries().then((data) => {
      if (!cancelled) setCountries(data);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // Pre-populate country from defaultCountryCode the first time the
  // fieldset mounts with an empty value. Only fires once and only when
  // there's nothing to overwrite.
  useEffect(() => {
    if (!value.country && defaultCountryCode) {
      onChange({ ...value, country: defaultCountryCode });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultCountryCode]);

  // Re-fetch states whenever the chosen country changes. Empty list is a
  // valid "no subdivisions seeded yet" signal — the consumer falls back
  // to a text input below.
  useEffect(() => {
    let cancelled = false;
    if (!value.country) {
      setStates([]);
      return;
    }
    setStatesLoading(true);
    listStatesByCountry(value.country)
      .then((data) => {
        if (!cancelled) setStates(data);
      })
      .finally(() => {
        if (!cancelled) setStatesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [value.country]);

  const update = (field: keyof AddressValue, fieldValue: string) =>
    onChange({ ...value, [field]: fieldValue });

  const updateCountry = (countryCode: string) => {
    // Clear region: a subdivision code from the previous country is
    // never valid for the new one and stale region values silently
    // break carrier rate calls (e.g. ShipEngine rejects "NSW" with
    // country=US).
    onChange({ ...value, country: countryCode, region: "" });
  };

  const stateInUse = states.length > 0;

  return (
    <div className="space-y-4">
      {!hideName && (
        <Field label="Warehouse name" htmlFor={`${idPrefix}-name`}>
          <input
            id={`${idPrefix}-name`}
            type="text"
            autoComplete="organization"
            value={value.name}
            disabled={disabled}
            onChange={(e) => update("name", e.target.value)}
            className={inputClass}
            placeholder="Primary"
          />
        </Field>
      )}

      <Field label="Address line 1" htmlFor={`${idPrefix}-line1`}>
        <input
          id={`${idPrefix}-line1`}
          type="text"
          autoComplete="address-line1"
          value={value.line1}
          disabled={disabled}
          onChange={(e) => update("line1", e.target.value)}
          className={inputClass}
        />
      </Field>

      <Field label="Address line 2" htmlFor={`${idPrefix}-line2`}>
        <input
          id={`${idPrefix}-line2`}
          type="text"
          autoComplete="address-line2"
          value={value.line2}
          disabled={disabled}
          onChange={(e) => update("line2", e.target.value)}
          className={inputClass}
          placeholder="Apt, suite, unit (optional)"
        />
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Country" htmlFor={`${idPrefix}-country`}>
          <Select
            value={value.country || undefined}
            onValueChange={updateCountry}
            disabled={disabled || countries.length === 0}
          >
            <SelectTrigger id={`${idPrefix}-country`} className="w-full">
              <SelectValue
                placeholder={
                  countries.length === 0 ? "Loading…" : "Select country"
                }
              />
            </SelectTrigger>
            <SelectContent>
              {countries.map((c) => (
                <SelectItem key={c.code} value={c.code}>
                  <span aria-hidden className="mr-2">
                    {c.flag_emoji}
                  </span>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>

        <Field label="State / Region" htmlFor={`${idPrefix}-region`}>
          {stateInUse ? (
            <Select
              value={value.region || undefined}
              onValueChange={(v) => update("region", v)}
              disabled={disabled}
            >
              <SelectTrigger id={`${idPrefix}-region`} className="w-full">
                <SelectValue placeholder="Select state" />
              </SelectTrigger>
              <SelectContent>
                {states.map((s) => (
                  <SelectItem key={s.id} value={s.code}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <input
              id={`${idPrefix}-region`}
              type="text"
              autoComplete="address-level1"
              value={value.region}
              disabled={disabled || statesLoading}
              onChange={(e) => update("region", e.target.value)}
              className={inputClass}
              placeholder={
                !value.country
                  ? "Select a country first"
                  : statesLoading
                  ? "Loading…"
                  : "Enter state or region"
              }
            />
          )}
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="City / Suburb" htmlFor={`${idPrefix}-city`}>
          <input
            id={`${idPrefix}-city`}
            type="text"
            autoComplete="address-level2"
            value={value.city}
            disabled={disabled}
            onChange={(e) => update("city", e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Postal code" htmlFor={`${idPrefix}-postal`}>
          <input
            id={`${idPrefix}-postal`}
            type="text"
            autoComplete="postal-code"
            value={value.postal}
            disabled={disabled}
            onChange={(e) => update("postal", e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Phone" htmlFor={`${idPrefix}-phone`}>
          <input
            id={`${idPrefix}-phone`}
            type="tel"
            autoComplete="tel"
            value={value.phone}
            disabled={disabled}
            onChange={(e) => update("phone", e.target.value)}
            className={inputClass}
          />
        </Field>
      </div>
    </div>
  );
}

function Field({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label
        htmlFor={htmlFor}
        className="block text-sm font-medium text-[color:var(--ink-900)]"
      >
        {label}
      </label>
      {children}
    </div>
  );
}
