"use client";

// Storefront AddressFieldset — sister of apps/admin's component.
// Drives the customer's own address book + checkout shipping form.
// Backed by /api/locations/* (countries, states, photon autocomplete).
// Styled with storefront CSS variables, not the admin Paper · Ink · Moss
// tokens, so it visually adapts to the merchant's theme.

import { useEffect, useRef, useState } from "react";
import { isIndia, isInPhoneValid } from "@/lib/checkout/phone";
import { SelectMenu } from "./SelectMenu";

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
  const [searching, setSearching] = useState(false);
  // Photon answers in 3–5 s, so a reply to an older query can land after
  // a newer one. Only the latest query may set suggestions.
  const querySeqRef = useRef(0);
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
    // value and onChange are real dependencies. The `!value.country`
    // guard means the extra runs do nothing once the default has been
    // applied, so listing them is honest and cannot loop (#1000).
  }, [defaultCountryCode, value, onChange]);

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

  // Only complain once something has been typed — an empty field is
  // incomplete, not wrong, and the placeholder already states the format.
  const showPhoneError =
    isIndia(value.country) &&
    value.phone.trim() !== "" &&
    !isInPhoneValid(value.phone);

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
      querySeqRef.current++;
      setSuggestions([]);
      setShowSuggestions(false);
      setSearching(false);
      return;
    }
    fetchTimerRef.current = setTimeout(async () => {
      const seq = ++querySeqRef.current;
      setSearching(true);
      const params = new URLSearchParams({ q: text });
      if (value.country) params.set("country", value.country);
      try {
        const res = await fetch(
          `/api/locations/autocomplete?${params.toString()}`,
          { cache: "no-store" },
        );
        if (!res.ok || seq !== querySeqRef.current) return;
        const body = (await res.json()) as { data: AddressSuggestion[] };
        if (seq !== querySeqRef.current) return;
        setSuggestions(body.data ?? []);
        setShowSuggestions(true);
      } catch {
        // Silent — autocomplete is best-effort.
      } finally {
        if (seq === querySeqRef.current) setSearching(false);
      }
    }, 250);
  }

  function selectSuggestion(s: AddressSuggestion) {
    // Photon names the state ("New South Wales"); the state menu is keyed
    // by code ("NSW"). Passing the name through left the menu on "Select
    // state" after every pick.
    const wanted = s.region.trim().toLowerCase();
    const state = states.find(
      (st) => st.name.toLowerCase() === wanted || st.code.toLowerCase() === wanted,
    );
    onChange({
      ...value,
      line1: s.line1 || value.line1,
      city: s.city || value.city,
      region: state ? state.code : stateInUse ? value.region : s.region || value.region,
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
          {searching && !(showSuggestions && suggestions.length > 0) && (
            <p
              aria-live="polite"
              className="mt-1 text-xs text-[color:var(--storefront-text,var(--ink-900))]/50"
            >
              Looking up addresses…
            </p>
          )}
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
            <SelectMenu
              id={`${idPrefix}-country`}
              value={value.country}
              options={countries.map((c) => ({
                value: c.code,
                label: c.name,
                icon: c.flag_emoji,
              }))}
              onChange={updateCountry}
              placeholder={countries.length === 0 ? "Loading…" : "Select country"}
              disabled={disabled || countries.length === 0}
              className={inputClass}
            />
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
            <SelectMenu
              id={`${idPrefix}-region`}
              value={value.region}
              options={states.map((s) => ({ value: s.code, label: s.name }))}
              onChange={(code) => update("region", code)}
              placeholder="Select state"
              disabled={disabled}
              className={inputClass}
            />
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
            {isIndia(value.country) && (
              <span className="ml-1 opacity-60">(required)</span>
            )}
          </label>
          <input
            id={`${idPrefix}-phone`}
            name="tel"
            type="tel"
            autoComplete={ac("tel")}
            value={value.phone}
            disabled={disabled}
            required={isIndia(value.country)}
            onChange={(e) => update("phone", e.target.value)}
            aria-invalid={showPhoneError || undefined}
            aria-describedby={showPhoneError ? `${idPrefix}-phone-error` : undefined}
            placeholder={isIndia(value.country) ? "10-digit mobile e.g. 9876543210" : ""}
            className={inputClass}
          />
          {/* Carriers reject an IN delivery whose phone isn't a real mobile,
              and this fieldset previously validated nothing at all — the
              address saved fine and failed later, at shipment creation. */}
          {showPhoneError && (
            <p
              id={`${idPrefix}-phone-error`}
              role="alert"
              className="mt-1.5 text-sm text-[color:var(--storefront-danger)]"
            >
              Enter a 10-digit Indian mobile starting with 6–9. A +91 prefix,
              spaces, and dashes are fine.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
