"use client";

import { change } from "@/lib/instore/cart";
import { formatMoney } from "@/lib/format";

export type PaymentMethod = "card_external" | "cash" | "bank_transfer" | "other";

const METHODS: { value: PaymentMethod; label: string }[] = [
  { value: "card_external", label: "Card (EFTPOS)" },
  { value: "cash", label: "Cash" },
  { value: "bank_transfer", label: "Bank transfer" },
  { value: "other", label: "Other" },
];

const input = "h-11 rounded-md border border-border bg-surface px-3 text-base";

export function ChargeSheet({
  total,
  currencyCode,
  method,
  tendered,
  otherLabel,
  pending,
  uncertain,
  message,
  onMethod,
  onTendered,
  onOtherLabel,
  onPaid,
  onBack,
}: {
  total: string;
  currencyCode: string;
  method: PaymentMethod;
  tendered: string;
  otherLabel: string;
  pending: boolean;
  // The last Paid got no answer: the next one retries the same sale.
  uncertain: boolean;
  message: string | null;
  onMethod: (m: PaymentMethod) => void;
  onTendered: (v: string) => void;
  onOtherLabel: (v: string) => void;
  onPaid: () => void;
  onBack: () => void;
}) {
  const due = method === "cash" ? change(tendered, total) : null;
  const ready =
    !pending &&
    (method !== "cash" || due !== null) &&
    (method !== "other" || otherLabel.trim().length > 0);

  return (
    <section aria-label="Charge" className="flex flex-col gap-4 rounded-md border border-border p-4">
      <p className="text-2xl font-semibold tabular-nums">Charge {formatMoney(total, currencyCode)}</p>
      <p className="text-sm text-foreground-secondary">
        Key this amount into your terminal, or take it in cash, then tap Paid.
      </p>

      <fieldset className="grid grid-cols-2 gap-2">
        <legend className="mb-1 text-sm font-medium">How did they pay?</legend>
        {METHODS.map((m) => (
          <label key={m.value}
            className="flex min-h-11 items-center gap-2 rounded-md border border-border px-3 text-sm has-[:checked]:border-primary">
            <input type="radio" name="payment-method" value={m.value} checked={method === m.value}
              disabled={pending || uncertain} onChange={() => onMethod(m.value)} />
            {m.label}
          </label>
        ))}
      </fieldset>

      {method === "cash" ? (
        <label className="flex flex-col gap-1 text-sm">
          Cash tendered
          <input className={input} inputMode="decimal" value={tendered} disabled={pending || uncertain}
            onChange={(e) => onTendered(e.target.value)} />
          {due !== null ? (
            <span className="text-base font-medium">Change {formatMoney(due, currencyCode)}</span>
          ) : tendered ? (
            <span className="text-danger">Not enough to cover the total.</span>
          ) : null}
        </label>
      ) : null}
      {method === "other" ? (
        <label className="flex flex-col gap-1 text-sm">
          Payment method name
          <input className={input} maxLength={40} placeholder="e.g. UPI" value={otherLabel}
            disabled={pending || uncertain} onChange={(e) => onOtherLabel(e.target.value)} />
        </label>
      ) : null}

      {message ? <p role="alert" className="text-sm text-danger">{message}</p> : null}

      <div className="flex gap-2">
        <button type="button" onClick={onBack} disabled={pending} className="min-h-12 px-4 text-sm">
          Back
        </button>
        <button
          type="button"
          disabled={!ready}
          onClick={onPaid}
          className="min-h-12 flex-1 rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? "Recording…" : uncertain ? "Try again" : "Paid"}
        </button>
      </div>
    </section>
  );
}
