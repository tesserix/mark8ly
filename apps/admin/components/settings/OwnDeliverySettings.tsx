"use client";

// Own-delivery settings: flat-rate options + the seller's own couriers.
// Spec: "The seller's experience -> Settings -> Shipping".

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import type {
  Courier,
  DeliverySettings,
  ShippingMode,
} from "@/lib/api/delivery-api";
import { useToast } from "@/components/feedback/Toaster";
import {
  createCourier,
  deleteCourier,
  saveDeliveryOptions,
  setShippingMode,
  updateCourier,
} from "@/app/(admin)/settings/shipping/delivery-actions";

const MAX_OPTIONS = 5;

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";
const labelClass = "mb-1.5 block text-sm text-[color:var(--ink-900)]/70";
const primaryBtn =
  "rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40";
const secondaryBtn =
  "rounded-md border border-[color:var(--ink-900)]/10 px-4 py-2 text-sm font-medium text-[color:var(--ink-900)]/70 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40";

function ErrorBox({ message }: { message: string | null }) {
  return (
    <div aria-live="polite">
      {message && (
        <div
          role="alert"
          className="rounded-md border border-[color:var(--danger)]/25 bg-[color:var(--danger)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--danger)]"
        >
          {message}
        </div>
      )}
    </div>
  );
}

// ── Mode switch ──────────────────────────────────────────────────────────

export function ModeSwitch({
  mode,
  editable,
}: {
  mode: ShippingMode;
  editable: boolean;
}) {
  const router = useRouter();
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();

  function choose(next: ShippingMode) {
    if (next === mode) return;
    startTransition(async () => {
      const r = await setShippingMode(next);
      if (!r.ok) {
        toast.error("Couldn't change delivery mode", r.message);
        return;
      }
      toast.success(
        next === "own" ? "Switched to your own delivery" : "Switched to Mark8ly couriers",
      );
      router.refresh();
    });
  }

  return (
    <fieldset className="space-y-2" disabled={!editable || pending}>
      <legend className="mb-1 text-sm font-medium text-[color:var(--ink-900)]">
        How do you ship?
      </legend>
      {(
        [
          ["carrier", "Mark8ly couriers"],
          ["own", "Own delivery"],
        ] as const
      ).map(([value, label]) => (
        <label key={value} className="flex items-center gap-2 text-sm text-[color:var(--ink-900)]">
          <input
            type="radio"
            name="shipping-mode"
            checked={mode === value}
            onChange={() => choose(value)}
          />
          {label}
        </label>
      ))}
      <p className="text-xs text-[color:var(--ink-900)]/50">
        Switching keeps both setups — nothing you&rsquo;ve configured is deleted.
      </p>
    </fieldset>
  );
}

// ── Options ──────────────────────────────────────────────────────────────

interface OptionRow {
  id?: string;
  name: string;
  price: string;
  estimate: string;
  enabled: boolean;
}

function OptionsEditor({
  settings,
  editable,
}: {
  settings: DeliverySettings;
  editable: boolean;
}) {
  const router = useRouter();
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();
  const [rows, setRows] = useState<OptionRow[]>(
    settings.options.length > 0
      ? settings.options.map((o) => ({
          id: o.id,
          name: o.name,
          price: o.price,
          estimate: o.estimate ?? "",
          enabled: o.enabled,
        }))
      : [{ name: "Standard", price: "", estimate: "", enabled: true }],
  );
  const [freeMin, setFreeMin] = useState(settings.free_shipping_min ?? "");
  const [error, setError] = useState<string | null>(null);

  const patch = (i: number, p: Partial<OptionRow>) => {
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...p } : r)));
    setError(null);
  };

  function save() {
    const bad = rows.findIndex((r) => !r.name.trim() || r.price.trim() === "");
    if (bad >= 0) {
      setError(`Option ${bad + 1} needs a name and a price (0 for free).`);
      return;
    }
    setError(null);
    startTransition(async () => {
      const r = await saveDeliveryOptions(
        rows.map((o) => ({
          id: o.id,
          name: o.name.trim(),
          price: o.price.trim(),
          estimate: o.estimate.trim() || undefined,
          enabled: o.enabled,
        })),
        freeMin.trim() === "" ? null : freeMin.trim(),
      );
      if (!r.ok) {
        setError(r.message);
        return;
      }
      toast.success("Delivery options saved");
      router.refresh();
    });
  }

  return (
    <section className="space-y-4" aria-labelledby="own-options-h">
      <h3 id="own-options-h" className="font-serif text-lg text-[color:var(--ink-900)]">
        Delivery options
      </h3>
      {rows.map((r, i) => (
        <div
          key={r.id ?? i}
          className="grid gap-3 rounded-md border border-[color:var(--ink-900)]/10 p-4 sm:grid-cols-[2fr_1fr_2fr_auto_auto]"
        >
          <div>
            <label htmlFor={`opt-${i}-name`} className={labelClass}>
              Option {i + 1} name
            </label>
            <input id={`opt-${i}-name`} className={inputClass} maxLength={40} value={r.name}
              disabled={!editable || pending} onChange={(e) => patch(i, { name: e.target.value })} />
          </div>
          <div>
            <label htmlFor={`opt-${i}-price`} className={labelClass}>
              Option {i + 1} price
            </label>
            <input id={`opt-${i}-price`} className={inputClass} inputMode="decimal" value={r.price}
              disabled={!editable || pending} onChange={(e) => patch(i, { price: e.target.value })} />
          </div>
          <div>
            <label htmlFor={`opt-${i}-est`} className={labelClass}>
              Option {i + 1} estimate (optional)
            </label>
            <input id={`opt-${i}-est`} className={inputClass} maxLength={40} placeholder="2–4 days"
              value={r.estimate} disabled={!editable || pending}
              onChange={(e) => patch(i, { estimate: e.target.value })} />
          </div>
          <label className="flex items-end gap-2 pb-2 text-sm">
            <input type="checkbox" checked={r.enabled} disabled={!editable || pending}
              onChange={(e) => patch(i, { enabled: e.target.checked })} />
            On
          </label>
          <button type="button" className={`${secondaryBtn} self-end`} disabled={!editable || pending}
            aria-label={`Remove option ${i + 1}`}
            onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}>
            Remove
          </button>
        </div>
      ))}

      <button type="button" className={secondaryBtn}
        disabled={!editable || pending || rows.length >= MAX_OPTIONS}
        onClick={() =>
          setRows((rs) => [...rs, { name: "", price: "", estimate: "", enabled: true }])
        }>
        Add option
      </button>

      <div className="max-w-xs">
        <label htmlFor="free-min" className={labelClass}>
          Free shipping over (leave blank for none)
        </label>
        <input id="free-min" className={inputClass} inputMode="decimal" value={freeMin}
          disabled={!editable || pending}
          onChange={(e) => { setFreeMin(e.target.value); setError(null); }} />
      </div>

      <ErrorBox message={error} />
      <button type="button" className={primaryBtn} disabled={!editable || pending} onClick={save}>
        {pending ? "Saving..." : "Save options"}
      </button>
    </section>
  );
}

// ── Couriers ─────────────────────────────────────────────────────────────

// Mirrors the server (authority): https, exactly one {number}, <= 500.
function templateError(t: string): string | null {
  if (!t) return null;
  if (!t.startsWith("https://")) return "Tracking link must start with https://";
  if (t.split("{number}").length !== 2) return "Tracking link must contain {number} exactly once.";
  if (t.length > 500) return "Tracking link is too long (500 characters max).";
  return null;
}

function CourierForm({
  courier,
  editable,
  onDone,
}: {
  courier?: Courier;
  editable: boolean;
  onDone?: () => void;
}) {
  const router = useRouter();
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();
  const [name, setName] = useState(courier?.name ?? "");
  const [template, setTemplate] = useState(courier?.tracking_url_template ?? "");
  const [error, setError] = useState<string | null>(null);
  const idp = courier ? `c-${courier.id}` : "c-new";

  function save() {
    const t = template.trim();
    const invalid = !name.trim() ? "Give this courier a name." : templateError(t);
    if (invalid) {
      setError(invalid);
      return;
    }
    setError(null);
    startTransition(async () => {
      const input = { name: name.trim(), tracking_url_template: t || undefined };
      const r = courier ? await updateCourier(courier.id, input) : await createCourier(input);
      if (!r.ok) {
        setError(r.message);
        return;
      }
      toast.success("Courier saved");
      router.refresh();
      onDone?.();
    });
  }

  function remove() {
    if (!courier) return;
    startTransition(async () => {
      const r = await deleteCourier(courier.id);
      if (!r.ok) {
        setError(r.message);
        return;
      }
      toast.success("Courier removed");
      router.refresh();
    });
  }

  return (
    <div className="space-y-3 rounded-md border border-[color:var(--ink-900)]/10 p-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label htmlFor={`${idp}-name`} className={labelClass}>Courier name</label>
          <input id={`${idp}-name`} className={inputClass} maxLength={40} value={name}
            disabled={!editable || pending}
            onChange={(e) => { setName(e.target.value); setError(null); }} />
        </div>
        <div>
          <label htmlFor={`${idp}-tpl`} className={labelClass}>Tracking link</label>
          <input id={`${idp}-tpl`} className={inputClass} value={template}
            placeholder="https://courier.com/track/{number}" disabled={!editable || pending}
            onChange={(e) => { setTemplate(e.target.value); setError(null); }} />
          <p className="mt-1 text-xs text-[color:var(--ink-900)]/40">
            Use {"{number}"} where the tracking number goes. Optional.
          </p>
        </div>
      </div>
      <ErrorBox message={error} />
      <div className="flex flex-wrap items-center gap-3">
        <button type="button" className={primaryBtn} disabled={!editable || pending} onClick={save}>
          Save courier
        </button>
        <button type="button" className={secondaryBtn}
          disabled={!template.trim() || templateError(template.trim()) !== null}
          onClick={() =>
            window.open(template.trim().replace("{number}", "TEST123"), "_blank", "noopener,noreferrer")
          }>
          Test link
        </button>
        {courier ? (
          <button type="button" className={secondaryBtn} disabled={!editable || pending} onClick={remove}>
            Remove
          </button>
        ) : (
          <button type="button" className={secondaryBtn} disabled={pending} onClick={onDone}>
            Cancel
          </button>
        )}
      </div>
    </div>
  );
}

function CouriersEditor({
  couriers,
  editable,
}: {
  couriers: Courier[];
  editable: boolean;
}) {
  const [adding, setAdding] = useState(false);
  return (
    <section className="space-y-4" aria-labelledby="own-couriers-h">
      <div className="space-y-1">
        <h3 id="own-couriers-h" className="font-serif text-lg text-[color:var(--ink-900)]">
          Your couriers
        </h3>
        <p className="text-sm text-[color:var(--ink-900)]/60">
          Pick one when you mark an order shipped. The tracking link is optional.
        </p>
      </div>
      {couriers.map((c) => (
        <CourierForm key={c.id} courier={c} editable={editable} />
      ))}
      {adding ? (
        <CourierForm editable={editable} onDone={() => setAdding(false)} />
      ) : (
        <button type="button" className={secondaryBtn} disabled={!editable} onClick={() => setAdding(true)}>
          Add courier
        </button>
      )}
    </section>
  );
}

export function OwnDeliverySettings({
  settings,
  editable,
}: {
  settings: DeliverySettings;
  editable: boolean;
}) {
  const enabledCount = settings.options.filter((o) => o.enabled).length;
  return (
    <div className="space-y-8">
      {enabledCount === 0 && (
        <div
          role="status"
          className="rounded-md border border-[color:var(--signal,#B7410E)]/25 bg-[color:var(--signal,#B7410E)]/[0.04] px-4 py-3 text-sm text-[color:var(--ink-900)]"
        >
          Buyers can&apos;t check out until at least one option is on.
        </div>
      )}
      <OptionsEditor settings={settings} editable={editable} />
      <CouriersEditor couriers={settings.couriers} editable={editable} />
    </div>
  );
}
