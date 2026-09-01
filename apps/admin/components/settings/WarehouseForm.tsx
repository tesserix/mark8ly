"use client";

// WarehouseForm — create/edit one warehouse (#177 PR 5c).
//
// Reuses AddressFieldset, the same component the carrier form used to
// embed. The difference is where the address now LIVES: on the warehouse,
// keyed by id, so a rename edits the row instead of forking a second,
// stockless one that quietly takes allocations.

import { useState, useTransition } from "react";

import {
  AddressFieldset,
  EMPTY_ADDRESS,
  type AddressValue,
} from "@/components/forms/AddressFieldset";
import type { Warehouse, WarehouseWriteInput } from "@/lib/api/warehouses-api";

interface WarehouseFormProps {
  /** Absent when creating. */
  existing?: Warehouse;
  storeCountry: string;
  onSubmit: (input: WarehouseWriteInput) => Promise<{ ok: boolean; message?: string }>;
  onCancel: () => void;
}

function toAddressValue(w: Warehouse | undefined): AddressValue {
  if (!w) return EMPTY_ADDRESS;
  return {
    name: w.name,
    line1: w.line1,
    line2: w.line2 ?? "",
    city: w.city,
    region: w.region,
    postal: w.postal_code,
    country: w.country_code,
    phone: w.phone,
  };
}

export function WarehouseForm({
  existing,
  storeCountry,
  onSubmit,
  onCancel,
}: WarehouseFormProps) {
  const [address, setAddress] = useState<AddressValue>(toAddressValue(existing));
  const [contactPerson, setContactPerson] = useState(existing?.contact_person ?? "");
  const [email, setEmail] = useState(existing?.email ?? "");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  // Mirrors validateWarehouseWrite on the server. Checked here too so the
  // merchant sees the rule at the field rather than as a round-tripped
  // error — but the server remains the authority, and its message wins
  // when the two ever disagree.
  function localError(): string | null {
    if (!address.name.trim()) return "Give this warehouse a name.";
    if (!address.line1.trim()) return "Street address is required.";
    if (!address.city.trim()) return "City is required.";
    if (!address.postal.trim()) return "Postcode is required.";
    if (address.country.trim().length !== 2) return "Select a country.";
    // Not bureaucracy: a warehouse saved without a phone made every rate
    // request fail with a bare "valid from zip code" and no cause (#508).
    if (!address.phone.trim()) {
      return "Add a phone number — carriers reject rate requests without one, and your storefront would show no delivery options.";
    }
    return null;
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const invalid = localError();
    if (invalid) {
      setError(invalid);
      return;
    }
    setError(null);
    startTransition(async () => {
      const result = await onSubmit({
        name: address.name.trim(),
        line1: address.line1.trim(),
        line2: address.line2.trim() || undefined,
        city: address.city.trim(),
        region: address.region.trim() || undefined,
        postal_code: address.postal.trim(),
        country_code: address.country.trim().toUpperCase(),
        phone: address.phone.trim(),
        email: email.trim() || undefined,
        contact_person: contactPerson.trim() || undefined,
      });
      if (!result.ok) {
        setError(result.message ?? "Could not save this warehouse.");
      }
    });
  }

  const inputClass =
    "w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";
  const idPrefix = existing ? `wh-${existing.id}` : "wh-new";

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <AddressFieldset
        value={address}
        onChange={(next) => {
          setAddress(next);
          setError(null);
        }}
        defaultCountryCode={storeCountry}
        lockCountry={Boolean(storeCountry)}
        disabled={pending}
        idPrefix={idPrefix}
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label
            htmlFor={`${idPrefix}-contact`}
            className="mb-1.5 block text-sm text-[color:var(--ink-900)]/70"
          >
            Contact person (optional)
          </label>
          <input
            id={`${idPrefix}-contact`}
            type="text"
            value={contactPerson}
            onChange={(e) => setContactPerson(e.target.value)}
            disabled={pending}
            className={inputClass}
          />
          <p className="mt-1 text-xs text-[color:var(--ink-900)]/40">
            Who the courier asks for at pickup. Falls back to the warehouse
            name.
          </p>
        </div>
        <div>
          <label
            htmlFor={`${idPrefix}-email`}
            className="mb-1.5 block text-sm text-[color:var(--ink-900)]/70"
          >
            Contact email (optional)
          </label>
          <input
            id={`${idPrefix}-email`}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            disabled={pending}
            className={inputClass}
          />
        </div>
      </div>

      <div aria-live="polite">
        {error && (
          <div
            role="alert"
            className="rounded-md border border-[color:var(--danger)]/25 bg-[color:var(--danger)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--danger)]"
          >
            {error}
          </div>
        )}
      </div>

      <div className="flex items-center gap-3">
        <button
          type="submit"
          disabled={pending}
          className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {pending ? "Saving..." : existing ? "Save changes" : "Add warehouse"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          disabled={pending}
          className="rounded-md border border-[color:var(--ink-900)]/10 px-4 py-2 text-sm font-medium text-[color:var(--ink-900)]/70 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
