"use client";

// ShippingConfigForm — inline form for configuring a shipping carrier:
// API credentials, which warehouse it ships from, and fee settings.
//
// #177 PR 5d took the ADDRESS out of this form. It used to embed
// AddressFieldset and save a warehouse behind the carrier, keyed on the
// name the merchant typed — so a name that did not match exactly created a
// second, stockless warehouse instead of editing the first, and every order
// allocated to it was unshippable. A free-text field that must exactly
// match an existing record is the wrong contract. The merchant now picks
// from their real warehouses and the form sends an id.

import { useState, useTransition } from "react";
import Link from "next/link";
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
import { defaultCarrierActive } from "@/lib/settings/carrier-active";
import type { Warehouse } from "@/lib/api/warehouses-api";

interface ShippingConfigFormProps {
  provider: string;
  existing?: ShippingConfig;
  /** The store's live warehouses, for the picker. */
  warehouses: Warehouse[];
}

export function ShippingConfigForm({
  provider,
  existing,
  warehouses,
}: ShippingConfigFormProps) {
  const router = useRouter();
  const [apiKey, setApiKey] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [mode, setMode] = useState<"test" | "live">(
    (existing?.mode as "test" | "live") ?? "test",
  );
  // New configs default ON: an inactive carrier quotes nothing and says
  // nothing. A saved choice always wins. See lib/settings/carrier-active.
  const [isActive, setIsActive] = useState(defaultCarrierActive(existing?.enabled));
  const [handlingFee, setHandlingFee] = useState(existing?.handling_fee ?? "0");
  const [freeShippingMin, setFreeShippingMin] = useState(
    existing?.free_shipping_threshold ?? "0",
  );
  // 500g is what checkout hardcoded before this was configurable, so an
  // untouched store keeps quoting exactly as it did.
  const [parcelWeight, setParcelWeight] = useState(
    String(existing?.default_parcel_weight_grams ?? 500),
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

  // Which warehouse this carrier ships from. With exactly one there is no
  // choice to make, so it binds silently and the card shows it read-only —
  // presenting a one-item dropdown would be asking a question with one
  // answer.
  const [warehouseId, setWarehouseId] = useState(
    existing?.warehouse_id ?? (warehouses.length === 1 ? warehouses[0]!.id : ""),
  );

  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    // The phone rule that used to live here moved to the warehouse form
    // with the address — one rule, one place. Two validators for the same
    // requirement drift, and the one nobody remembers is the one that
    // stops matching.
    if (warehouses.length > 0 && !warehouseId) {
      setError("Choose which warehouse this carrier ships from.");
      return;
    }

    startTransition(async () => {
      const result = await saveShippingConfig(provider, {
        api_key: apiKey,
        secret_key: secretKey || undefined,
        mode,
        is_active: isActive,
        handling_fee: parseFloat(handlingFee) || 0,
        free_shipping_min: parseFloat(freeShippingMin) || 0,
        // 0 or unparseable means "leave it alone" server-side, so a blank
        // box cannot silently reset a merchant's chosen weight.
        default_parcel_weight_grams: parseInt(parcelWeight, 10) || 0,
        // Only the id. The address is the warehouse's own; sending it from
        // here too would put one address behind two forms.
        warehouse_id: warehouseId || undefined,
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

        </div>

        {/* Kept out of the Mode row: "Test/Live" and "Active" are
            unrelated, and sitting on one line read as though Active
            modified the mode. */}
        <label className="flex items-start gap-2 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={isActive}
            onChange={(e) => { setIsActive(e.target.checked); setSuccess(false); }}
            disabled={pending}
            className="mt-0.5 h-4 w-4 rounded border-[color:var(--ink-900)]/20 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
          />
          <span className="text-sm text-[color:var(--ink-900)]">
            Active
            <span className="block text-xs text-[color:var(--ink-900)]/40">
              Only active carriers are quoted at checkout. Untick to pause
              this carrier without deleting its credentials.
            </span>
          </span>
        </label>
      </fieldset>

      {/* Ships from — a picker over real warehouses, never free text.
          The address itself lives on /settings/warehouses. */}
      <fieldset className="space-y-4">
        <legend className="mb-2 text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
          Ships from
        </legend>

        {warehouses.length === 0 ? (
          <div className="rounded-md border border-[color:var(--signal,#B7410E)]/25 bg-[color:var(--signal,#B7410E)]/[0.04] px-4 py-3">
            <p className="text-sm text-[color:var(--ink-900)]">
              You have no warehouses yet, and a carrier cannot quote a rate
              without an origin address.
            </p>
            <Link
              href="/settings/warehouses"
              className="mt-2 inline-block text-sm font-medium text-[color:var(--moss-700)] underline underline-offset-4"
            >
              Add a warehouse
            </Link>
          </div>
        ) : warehouses.length === 1 ? (
          <div>
            <p className="text-sm text-[color:var(--ink-900)]">
              {warehouses[0]!.name}
            </p>
            <p className="mt-0.5 text-sm text-foreground-secondary">
              {[
                warehouses[0]!.line1,
                warehouses[0]!.city,
                warehouses[0]!.postal_code,
                warehouses[0]!.country_code,
              ]
                .filter(Boolean)
                .join(", ")}
            </p>
            <p className="mt-1.5 text-xs text-[color:var(--ink-900)]/40">
              Your only warehouse, so this carrier ships from it.{" "}
              <Link
                href="/settings/warehouses"
                className="text-[color:var(--moss-700)] underline underline-offset-4"
              >
                Edit the address
              </Link>
              .
            </p>
          </div>
        ) : (
          <Field label="Warehouse" htmlFor={`${provider}-warehouse`}>
            <Select
              value={warehouseId}
              onValueChange={(value) => {
                setWarehouseId(value);
                setSuccess(false);
              }}
              disabled={pending}
            >
              <SelectTrigger id={`${provider}-warehouse`} className="w-full">
                <SelectValue placeholder="Choose a warehouse" />
              </SelectTrigger>
              <SelectContent>
                {warehouses.map((w) => (
                  <SelectItem key={w.id} value={w.id}>
                    {w.name} — {w.city}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="mt-1 text-xs text-[color:var(--ink-900)]/40">
              Addresses are managed on the{" "}
              <Link
                href="/settings/warehouses"
                className="text-[color:var(--moss-700)] underline underline-offset-4"
              >
                Warehouses
              </Link>{" "}
              page.
            </p>
          </Field>
        )}
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
          <Field label="Default parcel weight (g)" htmlFor={`${provider}-parcel-weight`}>
            <input
              id={`${provider}-parcel-weight`}
              type="number"
              step="1"
              min="1"
              value={parcelWeight}
              onChange={(e) => { setParcelWeight(e.target.value); setSuccess(false); }}
              disabled={pending}
              className={inputClass}
            />
            <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
              Used only when a product has no weight of its own. Carriers price
              on this, so a wrong value over- or under-charges every such item.
              Set weights on your products for accurate rates.
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
