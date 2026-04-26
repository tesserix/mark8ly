"use client";

// Storefront AddressFieldset — sister of apps/admin's component.
// Drives the customer's own address book + checkout shipping form.
// Backed by /api/locations/* (countries, states, photon autocomplete).
// Styled with storefront CSS variables, not the admin Paper · Ink · Moss
// tokens, so it visually adapts to the merchant's theme.

import { useEffect, useRef, useState } from "react";

import type { Country, State } from "@/lib/api/platform-api";

interface AddressSuggestion {
  description: string;
  line1: string;
  city: string;
  region: string;
  postal: string;
  country: string;
}

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
  /** Pre-select country when value.country is empty. */
  defaultCountryCode?: string;
  disabled?: boolean;
  idPrefix?: string;
  /** Section token for browser autofill. */
  autocompleteSection?: "shipping" | "billing";
  /** When true, render country as a read-only badge. */
  lockCountry?: boolean;
  /** Hide the Full-name row. AddressBook owns that field separately. */
  hideName?: boolean;
}

const inputClass =
  "mt-1 w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))] disabled:opacity-60 disabled:cursor-not-allowed";

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
  autocompleteSection = "shipping",
  lockCountry = false,
  hideName = false,
}: AddressFieldsetProps) {
  const [countries, setCountries] = useState<Country[]>([]);
  const [states, setStates] = useState<State[]>([]);
  const [statesLoading, setStatesLoading] = useState(false);

  const [suggestions, setSuggestions] = useState<AddressSuggestion[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const fetchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const blurTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchJSON<Country>("/api/locations/countries").then((data) => {
      if (!cancelled) setCountries(data);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!value.country && defaultCountryCode) {
      onChange({ ...value, country: defaultCountryCode });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultCountryCode]);

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

  const updateCountry = (code: string) => {
    onChange({ ...value, country: code, region: "" });
  };

  const stateInUse = states.length > 0;
  const selectedCountry = countries.find((c) => c.code === value.country);
  const ac = (token: string) => `${autocompleteSection} ${token}`;

  function onLine1Change(text: string) {
    update("line1", text);
    if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current);
    if (text.trim().length < 3) {
      setSuggestions([]);
      setShowSuggestions(false);
      return;
    }
    fetchTimerRef.current = setTimeout(async () => {
      const params = new URLSearchParams({ q: text });
      if (value.country) params.set("country", value.country);
      try {
        const res = await fetch(
          `/api/locations/autocomplete?${params.toString()}`,
          { cache: "no-store" },
        );
        if (!res.ok) return;
        const body = (await res.json()) as { data: AddressSuggestion[] };
        setSuggestions(body.data ?? []);
        setShowSuggestions(true);
      } catch {
        // Silent — autocomplete is best-effort.
      }
    }, 250);
  }

  function selectSuggestion(s: AddressSuggestion) {
    onChange({
      ...value,
      line1: s.line1 || value.line1,
      city: s.city || value.city,
      region: s.region || value.region,
      postal: s.postal || value.postal,
      country: lockCountry ? value.country : s.country || value.country,
    });
    setSuggestions([]);
    setShowSuggestions(false);
  }

  return (
    <div className="space-y-4">
      {!hideName && (
        <div>
          <label
            htmlFor={`${idPrefix}-name`}
            className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          >
            Full name
          </label>
          <input
            id={`${idPrefix}-name`}
            name="name"
            type="text"
            autoComplete={ac("name")}
            value={value.name}
            disabled={disabled}
            onChange={(e) => update("name", e.target.value)}
            className={inputClass}
          />
        </div>
      )}

      <div>
        <label
          htmlFor={`${idPrefix}-line1`}
          className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Address line 1
        </label>
        <div className="relative">
          <input
            id={`${idPrefix}-line1`}
            name="address-line1"
            type="text"
            autoComplete={ac("address-line1")}
            value={value.line1}
            disabled={disabled}
            onChange={(e) => onLine1Change(e.target.value)}
            onFocus={() => {
              if (suggestions.length > 0) setShowSuggestions(true);
            }}
            onBlur={() => {
              if (blurTimerRef.current) clearTimeout(blurTimerRef.current);
              blurTimerRef.current = setTimeout(
                () => setShowSuggestions(false),
                150,
              );
            }}
            className={inputClass}
          />
          {showSuggestions && suggestions.length > 0 && (
            <ul
              role="listbox"
              className="absolute z-20 mt-1 w-full overflow-hidden rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] shadow-lg"
            >
              {suggestions.map((s, i) => (
                <li
                  key={`${s.description}-${i}`}
                  role="option"
                  aria-selected="false"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    selectSuggestion(s);
                  }}
                  className="cursor-pointer px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] hover:bg-[color:var(--storefront-accent,var(--moss-700))]/[0.08]"
                >
                  {s.description}
                </li>
              ))}
              <li
                aria-hidden
                className="border-t border-[color:var(--storefront-text,var(--ink-900))]/5 px-3 py-1.5 text-[10px] uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))]/50"
              >
                Suggestions by Photon · OpenStreetMap contributors
              </li>
            </ul>
          )}
        </div>
      </div>

      <div>
        <label
          htmlFor={`${idPrefix}-line2`}
          className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Address line 2
        </label>
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
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label
            htmlFor={`${idPrefix}-country`}
            className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          >
            Country
          </label>
          {lockCountry ? (
            <div
              id={`${idPrefix}-country`}
              aria-readonly
              className={`${inputClass} flex cursor-not-allowed items-center gap-2 bg-[color:var(--storefront-text,var(--ink-900))]/[0.04]`}
            >
              {selectedCountry ? (
                <>
                  <span aria-hidden>{selectedCountry.flag_emoji}</span>
                  <span>{selectedCountry.name}</span>
                </>
              ) : value.country ? (
                <span>{value.country}</span>
              ) : (
                <span className="opacity-40">—</span>
              )}
              <span className="ml-auto text-[10px] uppercase tracking-[0.12em] opacity-50">
                Set by store
              </span>
            </div>
          ) : (
            <select
              id={`${idPrefix}-country`}
              name="country"
              autoComplete={ac("country")}
              value={value.country}
              disabled={disabled || countries.length === 0}
              onChange={(e) => updateCountry(e.target.value)}
              className={inputClass}
            >
              <option value="">
                {countries.length === 0 ? "Loading…" : "Select country"}
              </option>
              {countries.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.flag_emoji} {c.name}
                </option>
              ))}
            </select>
          )}
        </div>

        <div>
          <label
            htmlFor={`${idPrefix}-region`}
            className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          >
            State / Region
          </label>
          {stateInUse ? (
            <select
              id={`${idPrefix}-region`}
              name="address-level1"
              autoComplete={ac("address-level1")}
              value={value.region}
              disabled={disabled}
              onChange={(e) => update("region", e.target.value)}
              className={inputClass}
            >
              <option value="">Select state</option>
              {states.map((s) => (
                <option key={s.id} value={s.code}>
                  {s.name}
                </option>
              ))}
            </select>
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
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <div>
          <label
            htmlFor={`${idPrefix}-city`}
            className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          >
            City / Suburb
          </label>
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
        </div>

        <div>
          <label
            htmlFor={`${idPrefix}-postal`}
            className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          >
            Postal code
          </label>
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
        </div>

        <div>
          <label
            htmlFor={`${idPrefix}-phone`}
            className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]"
          >
            Phone
          </label>
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
        </div>
      </div>
    </div>
  );
}
