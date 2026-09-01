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
import {
  defaultAutoSchedulePickup,
  supportsPickupAutomation,
} from "@/lib/settings/pickup-automation";
import {
  AddressFieldset,
  type AddressValue,
} from "@/components/forms/AddressFieldset";

interface ShippingConfigFormProps {
  provider: string;
  existing?: ShippingConfig;
  /**
   * ISO 3166 alpha-2 country code of the store, used to pre-select the
   * warehouse country dropdown when the merchant first opens the form.
   * Plumbed from `supported.country_code` in ShippingSettingsClient.
   */
  defaultCountryCode?: string;
}

export function ShippingConfigForm({
  provider,
  existing,
  defaultCountryCode,
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

  // Pickup automation. A saved value always wins; the unset default is
  // ON only for carriers that actually implement SchedulePickup, so a
  // ShipEngine merchant is no longer shown a pre-ticked Delhivery
  // option. See lib/settings/pickup-automation.
  const showPickupAutomation = supportsPickupAutomation(provider);
  const [autoSchedulePickup, setAutoSchedulePickup] = useState(
    defaultAutoSchedulePickup(provider, existing?.auto_schedule_pickup),
  );
  const [defaultPickupSlotStart, setDefaultPickupSlotStart] = useState(
    existing?.default_pickup_slot_start ?? "14:00:00",
  );

  // Single object for the entire warehouse address. Replaces 8 separate
  // useStates that each forced a full-form rerender on every keystroke
  // and made the country box accept "Au" instead of "AU". For a brand
  // new carrier card (no existing config) the country starts pre-selected
  // to the store's country, so the merchant only types the street/city
  // bits — no scroll-and-pick through 240+ countries to find their own.
  const [address, setAddress] = useState<AddressValue>({
    name: existing?.warehouse_name ?? "",
    line1: existing?.warehouse_line1 ?? "",
    line2: existing?.warehouse_line2 ?? "",
    city: existing?.warehouse_city ?? "",
    region: existing?.warehouse_region ?? "",
    postal: existing?.warehouse_postal ?? "",
    country:
      existing?.warehouse_country ?? defaultCountryCode ?? "",
    phone: existing?.warehouse_phone ?? "",
  });

  const [warehouseContactPerson, setWarehouseContactPerson] = useState(
    existing?.warehouse_contact_person ?? "",
  );
  const [warehouseEmail, setWarehouseEmail] = useState(
    existing?.warehouse_email ?? "",
  );

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
        warehouse_name: address.name || undefined,
        warehouse_line1: address.line1 || undefined,
        warehouse_line2: address.line2 || undefined,
        warehouse_city: address.city || undefined,
        warehouse_region: address.region || undefined,
        warehouse_postal: address.postal || undefined,
        warehouse_country: address.country || undefined,
        warehouse_phone: address.phone || undefined,
        warehouse_contact_person: warehouseContactPerson || undefined,
        warehouse_email: warehouseEmail || undefined,
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
    "w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";

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
              placeholder={existing ? "Enter new key to replace" : "API key"}
              required={!existing}
              disabled={pending}
              aria-describedby={existing ? `${provider}-api-key-hint` : undefined}
              className={inputClass}
            />
            {/* Only send a key when the merchant types one — a blank field
                keeps the stored credential (see the shipping upsert action).
                Show the masked key so they can tell one is set and aren't
                left guessing whether the empty box means "no key". */}
            {existing?.api_key && (
              <p
                id={`${provider}-api-key-hint`}
                className="text-xs text-[color:var(--ink-900)]/55"
              >
                A key is saved (<span className="font-mono">{existing.api_key}</span>).
                Leave this blank to keep it — you only need to enter a key to
                replace it.
              </p>
            )}
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
        <AddressFieldset
          value={address}
          onChange={setAddress}
          defaultCountryCode={defaultCountryCode}
          lockCountry={Boolean(defaultCountryCode)}
          disabled={pending}
          idPrefix={`${provider}-wh`}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Contact person (optional)" htmlFor={`${provider}-wh-contact`}>
            <input
              id={`${provider}-wh-contact`}
              type="text"
              value={warehouseContactPerson}
              onChange={(e) => { setWarehouseContactPerson(e.target.value); setSuccess(false); }}
              disabled={pending}
              className={inputClass}
            />
            <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
              Who the courier should ask for when picking up. Falls back to the
              warehouse name if left blank.
            </p>
          </Field>
          <Field label="Contact email (optional)" htmlFor={`${provider}-wh-email`}>
            <input
              id={`${provider}-wh-email`}
              type="email"
              value={warehouseEmail}
              onChange={(e) => { setWarehouseEmail(e.target.value); setSuccess(false); }}
              disabled={pending}
              className={inputClass}
            />
            <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
              Used for shipping label pickup notifications. Falls back to the
              customer&apos;s email if left blank.
            </p>
          </Field>
        </div>
      </fieldset>

      {/* Pickup automation — only rendered for carriers that implement
          SchedulePickup. Showing it elsewhere advertised a Delhivery
          feature to (say) an Australian ShipEngine store, pre-ticked,
          where it silently no-ops. */}
      {showPickupAutomation && (
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
      )}

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
        <div role="alert" className="rounded-md border border-[color:var(--signal)]/30 bg-[color:var(--signal)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--signal)]">
          {error}
        </div>
      )}
      {success && (
        <div role="status" className="animate-in fade-in duration-300 rounded-md border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]">
          Configuration saved.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={pending || (!apiKey.trim() && !existing)}
          className="rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
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
