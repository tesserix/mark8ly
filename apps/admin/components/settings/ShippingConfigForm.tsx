"use client";

// ShippingConfigForm — inline form for configuring a shipping carrier.
// Includes API credentials, warehouse address, and fee settings.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import { saveShippingConfig } from "@/app/(admin)/settings/shipping/actions";
import type { ShippingConfig } from "@/lib/api/settings-api";

interface ShippingConfigFormProps {
  provider: string;
  existing?: ShippingConfig;
}

export function ShippingConfigForm({
  provider,
  existing,
}: ShippingConfigFormProps) {
  const router = useRouter();
  const [apiKey, setApiKey] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [mode, setMode] = useState<"test" | "live">(
    (existing?.mode as "test" | "live") ?? "test",
  );
  const [isActive, setIsActive] = useState(existing?.enabled ?? false);
  const [handlingFee, setHandlingFee] = useState(existing?.handling_fee ?? "0");
  const [freeShippingMin, setFreeShippingMin] = useState(
    existing?.free_shipping_threshold ?? "0",
  );

  // Pickup automation. Defaults mirror the DB: TRUE + 14:00-18:00 so
  // existing merchants get the zero-friction path the moment they load
  // the edit form, without needing to flip the checkbox first.
  const [autoSchedulePickup, setAutoSchedulePickup] = useState(
    existing?.auto_schedule_pickup ?? true,
  );
  const [defaultPickupSlotStart, setDefaultPickupSlotStart] = useState(
    existing?.default_pickup_slot_start ?? "14:00:00",
  );

  // Warehouse address
  const [whName, setWhName] = useState(existing?.warehouse_name ?? "");
  const [whLine1, setWhLine1] = useState(existing?.warehouse_line1 ?? "");
  const [whLine2, setWhLine2] = useState(existing?.warehouse_line2 ?? "");
  const [whCity, setWhCity] = useState(existing?.warehouse_city ?? "");
  const [whRegion, setWhRegion] = useState(existing?.warehouse_region ?? "");
  const [whPostal, setWhPostal] = useState(existing?.warehouse_postal ?? "");
  const [whCountry, setWhCountry] = useState(existing?.warehouse_country ?? "");
  const [whPhone, setWhPhone] = useState(existing?.warehouse_phone ?? "");

  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    startTransition(async () => {
      const result = await saveShippingConfig(provider, {
        api_key: apiKey,
        secret_key: secretKey || undefined,
        mode,
        is_active: isActive,
        handling_fee: parseFloat(handlingFee) || 0,
        free_shipping_min: parseFloat(freeShippingMin) || 0,
        warehouse_name: whName || undefined,
        warehouse_line1: whLine1 || undefined,
        warehouse_line2: whLine2 || undefined,
        warehouse_city: whCity || undefined,
        warehouse_region: whRegion || undefined,
        warehouse_postal: whPostal || undefined,
        warehouse_country: whCountry || undefined,
        warehouse_phone: whPhone || undefined,
        auto_schedule_pickup: autoSchedulePickup,
        default_pickup_slot_start: defaultPickupSlotStart,
        // Slot end mirrors the start + 4h — kept implicit because the UI
        // only offers the three named windows below, each with a fixed
        // 4-hour span. We send it anyway so the DB always has a value.
        default_pickup_slot_end: slotEndFor(defaultPickupSlotStart),
      });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(true);
      setApiKey("");
      setSecretKey("");
      router.refresh();
    });
  }

  const inputClass =
    "w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Credentials */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50 mb-2">
          Credentials
        </legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="API key" htmlFor={`${provider}-api-key`}>
            <input
              id={`${provider}-api-key`}
              type="password"
              autoComplete="off"
              value={apiKey}
              onChange={(e) => { setApiKey(e.target.value); setSuccess(false); }}
              placeholder={existing ? "Enter new key to update" : "API key"}
              required={!existing}
              disabled={pending}
              className={inputClass}
            />
          </Field>
          <Field label="Secret key" htmlFor={`${provider}-secret-key`}>
            <input
              id={`${provider}-secret-key`}
              type="password"
              autoComplete="off"
              value={secretKey}
              onChange={(e) => { setSecretKey(e.target.value); setSuccess(false); }}
              placeholder="Optional"
              disabled={pending}
              className={inputClass}
            />
          </Field>
        </div>

        <div className="flex items-center gap-6">
          <Field label="Mode" htmlFor={`${provider}-mode`}>
            <Select
              value={mode}
              onValueChange={(value) => {
                setMode(value as "test" | "live");
                setSuccess(false);
              }}
              disabled={pending}
            >
              <SelectTrigger id={`${provider}-mode`} className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="test">Test mode</SelectItem>
                <SelectItem value="live">Live mode</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <label className="flex items-center gap-2 cursor-pointer select-none pt-5">
            <input
              type="checkbox"
              checked={isActive}
              onChange={(e) => { setIsActive(e.target.checked); setSuccess(false); }}
              disabled={pending}
              className="h-4 w-4 rounded border-[color:var(--ink-900)]/20 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
            />
            <span className="text-sm text-[color:var(--ink-900)]">Active</span>
          </label>
        </div>
      </fieldset>

      {/* Warehouse address */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50 mb-2">
          Warehouse address
        </legend>
        <Field label="Warehouse name" htmlFor={`${provider}-wh-name`}>
          <input id={`${provider}-wh-name`} type="text" autoComplete="organization" value={whName} onChange={(e) => setWhName(e.target.value)} disabled={pending} className={inputClass} />
        </Field>
        <Field label="Address line 1" htmlFor={`${provider}-wh-line1`}>
          <input id={`${provider}-wh-line1`} type="text" autoComplete="address-line1" value={whLine1} onChange={(e) => setWhLine1(e.target.value)} disabled={pending} className={inputClass} />
        </Field>
        <Field label="Address line 2" htmlFor={`${provider}-wh-line2`}>
          <input id={`${provider}-wh-line2`} type="text" autoComplete="address-line2" value={whLine2} onChange={(e) => setWhLine2(e.target.value)} disabled={pending} className={inputClass} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="City" htmlFor={`${provider}-wh-city`}>
            <input id={`${provider}-wh-city`} type="text" autoComplete="address-level2" value={whCity} onChange={(e) => setWhCity(e.target.value)} disabled={pending} className={inputClass} />
          </Field>
          <Field label="Region / State" htmlFor={`${provider}-wh-region`}>
            <input id={`${provider}-wh-region`} type="text" autoComplete="address-level1" value={whRegion} onChange={(e) => setWhRegion(e.target.value)} disabled={pending} className={inputClass} />
          </Field>
          <Field label="Postal code" htmlFor={`${provider}-wh-postal`}>
            <input id={`${provider}-wh-postal`} type="text" autoComplete="postal-code" value={whPostal} onChange={(e) => setWhPostal(e.target.value)} disabled={pending} className={inputClass} />
          </Field>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Country" htmlFor={`${provider}-wh-country`}>
            <input id={`${provider}-wh-country`} type="text" autoComplete="country" value={whCountry} onChange={(e) => setWhCountry(e.target.value)} disabled={pending} maxLength={2} placeholder="US" className={inputClass} />
          </Field>
          <Field label="Phone" htmlFor={`${provider}-wh-phone`}>
            <input id={`${provider}-wh-phone`} type="tel" autoComplete="tel" value={whPhone} onChange={(e) => setWhPhone(e.target.value)} disabled={pending} className={inputClass} />
          </Field>
        </div>
      </fieldset>

      {/* Pickup automation — Delhivery only, but the form is
          carrier-agnostic so the fields stay visible for all providers.
          A merchant with ShipEngine sees the checkbox but it no-ops
          because the backend only calls SchedulePickup when the carrier
          implements the interface. */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50 mb-2">
          Pickup automation
        </legend>
        <label className="flex items-center gap-2 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={autoSchedulePickup}
            onChange={(e) => { setAutoSchedulePickup(e.target.checked); setSuccess(false); }}
            disabled={pending}
            className="h-4 w-4 rounded border-[color:var(--ink-900)]/20 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
          />
          <span className="text-sm text-[color:var(--ink-900)]">
            Auto-schedule Delhivery pickup when a label is created
          </span>
        </label>
        <Field label="Default pickup window" htmlFor={`${provider}-pickup-slot`}>
          <Select
            value={defaultPickupSlotStart}
            onValueChange={(value) => {
              setDefaultPickupSlotStart(value);
              setSuccess(false);
            }}
            disabled={pending || !autoSchedulePickup}
          >
            <SelectTrigger id={`${provider}-pickup-slot`} className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="10:00:00">Morning (10:00 – 14:00)</SelectItem>
              <SelectItem value="14:00:00">Afternoon (14:00 – 18:00)</SelectItem>
              <SelectItem value="16:00:00">Evening (16:00 – 20:00)</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
            Delhivery dispatches a pickup agent during this window on the
            next business day after the label is created.
          </p>
        </Field>
      </fieldset>

      {/* Fees */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50 mb-2">
          Fees
        </legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Handling fee" htmlFor={`${provider}-handling-fee`}>
            <input
              id={`${provider}-handling-fee`}
              type="number"
              step="0.01"
              min="0"
              value={handlingFee}
              onChange={(e) => { setHandlingFee(e.target.value); setSuccess(false); }}
              disabled={pending}
              className={inputClass}
            />
            <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
              A flat fee added to every shipment to cover packaging and processing costs.
            </p>
          </Field>
          <Field label="Free shipping threshold" htmlFor={`${provider}-free-shipping`}>
            <input
              id={`${provider}-free-shipping`}
              type="number"
              step="0.01"
              min="0"
              value={freeShippingMin}
              onChange={(e) => { setFreeShippingMin(e.target.value); setSuccess(false); }}
              disabled={pending}
              className={inputClass}
            />
            <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
              Orders above this amount ship free. Set to 0 to disable.
            </p>
          </Field>
        </div>
      </fieldset>

      {error && (
        <div role="alert" className="rounded-[6px] border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-800">
          {error}
        </div>
      )}
      {success && (
        <div role="status" className="animate-in fade-in duration-300 rounded-[6px] border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]">
          Configuration saved.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={pending || (!apiKey.trim() && !existing)}
          className="rounded-[6px] bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          {pending ? "Saving..." : "Save configuration"}
        </button>
      </div>
    </form>
  );
}

// slotEndFor derives the slot-end string from the chosen start. We
// offer three fixed 4-hour windows so there's no free-form edit path
// — encoding the mapping here keeps the UI from surfacing a separate
// "Slot end" picker that would just confuse the merchant.
function slotEndFor(start: string): string {
  switch (start) {
    case "10:00:00":
      return "14:00:00";
    case "16:00:00":
      return "20:00:00";
    case "14:00:00":
    default:
      return "18:00:00";
  }
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
      <label htmlFor={htmlFor} className="block text-sm font-medium text-[color:var(--ink-900)]">
        {label}
      </label>
      {children}
    </div>
  );
}
