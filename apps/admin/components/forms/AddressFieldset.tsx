"use client";

// AddressFieldset — shared warehouse / shipping address inputs backed by
// platform-api's /locations reference data via admin proxy routes.
// Country comes from the seeded countries list; State/Region is a
// dropdown when subdivisions are seeded for the chosen country,
// otherwise a free-form text input.
//
// Why proxy: PLATFORM_API_URL is a server-only env var (no NEXT_PUBLIC_
// prefix) so client components can't hit platform-api directly. The
// admin's /api/locations/* routes do the SSR-side fetch and forward.

import { useEffect, useState } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { Country, State } from "@/lib/api/platform-api";

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
   * ISO 3166 alpha-2 country code used to pre-select the country
   * dropdown when `value.country` is empty. Typically the store's
   * `country_code`.
   */
  defaultCountryCode?: string;
  /** Disables every input. Useful while a server action is pending. */
  disabled?: boolean;
  /** Prefix for input ids/labels so multiple fieldsets can coexist. */
  idPrefix?: string;
  /** Suppresses the "Warehouse name" row when the consumer manages it elsewhere. */
  hideName?: boolean;
  /**
   * Section token for browser autofill. "shipping" lets browsers /
   * password managers offer the user's shipping address; "billing"
   * targets billing-address autofill. Default "shipping".
   */
  autocompleteSection?: "shipping" | "billing";
}

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";

async function fetchJSON<T>(url: string): Promise<T[]> {
  try {
    const res = await fetch(url, { cache: "no-store" });
    if (!res.ok) return [];
    const body = (await res.json()) as { data: T[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

export function AddressFieldset({
  value,
  onChange,
  defaultCountryCode,
  disabled = false,
  idPrefix = "addr",
  hideName = false,
  autocompleteSection = "shipping",
}: AddressFieldsetProps) {
  const [countries, setCountries] = useState<Country[]>([]);
  const [states, setStates] = useState<State[]>([]);
  const [statesLoading, setStatesLoading] = useState(false);

  // One-shot fetch of the country reference list via admin proxy.
  useEffect(() => {
    let cancelled = false;
    fetchJSON<Country>("/api/locations/countries").then((data) => {
      if (!cancelled) setCountries(data);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // Pre-populate country from defaultCountryCode the first time the
  // fieldset mounts with an empty value.
  useEffect(() => {
    if (!value.country && defaultCountryCode) {
      onChange({ ...value, country: defaultCountryCode });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultCountryCode]);

  // Re-fetch states whenever the chosen country changes.
  useEffect(() => {
    let cancelled = false;
    if (!value.country) {
      setStates([]);
      return;
    }
    setStatesLoading(true);
    fetchJSON<State>(
      `/api/locations/countries/${encodeURIComponent(value.country)}/states`,
    )
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
    onChange({ ...value, country: countryCode, region: "" });
  };

  const stateInUse = states.length > 0;

  // Browser autofill works best when each input has a `name` attribute
  // matching the autocomplete value (some Chromium versions ignore
  // autocomplete without a name) AND a section token so 1Password /
  // iCloud / Chrome can distinguish shipping vs billing.
  const ac = (token: string) => `${autocompleteSection} ${token}`;

  return (
    <div className="space-y-4">
      {!hideName && (
        <Field label="Warehouse name" htmlFor={`${idPrefix}-name`}>
          <input
            id={`${idPrefix}-name`}
            name="organization"
            type="text"
            autoComplete={ac("organization")}
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
          name="address-line1"
          type="text"
          autoComplete={ac("address-line1")}
          value={value.line1}
          disabled={disabled}
          onChange={(e) => update("line1", e.target.value)}
          className={inputClass}
        />
      </Field>

      <Field label="Address line 2" htmlFor={`${idPrefix}-line2`}>
        <input
          id={`${idPrefix}-line2`}
          name="address-line2"
          type="text"
          autoComplete={ac("address-line2")}
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
              name="address-level1"
              type="text"
              autoComplete={ac("address-level1")}
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
            name="address-level2"
            type="text"
            autoComplete={ac("address-level2")}
            value={value.city}
            disabled={disabled}
            onChange={(e) => update("city", e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Postal code" htmlFor={`${idPrefix}-postal`}>
          <input
            id={`${idPrefix}-postal`}
            name="postal-code"
            type="text"
            autoComplete={ac("postal-code")}
            value={value.postal}
            disabled={disabled}
            onChange={(e) => update("postal", e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Phone" htmlFor={`${idPrefix}-phone`}>
          <input
            id={`${idPrefix}-phone`}
            name="tel"
            type="tel"
            autoComplete={ac("tel")}
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
