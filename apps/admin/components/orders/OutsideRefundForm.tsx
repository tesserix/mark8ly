"use client";

// Record a refund of an in-store sale made outside mark8ly (#1110): the money
// went back on the shop's own terminal or in cash. Nothing is charged or
// refunded through mark8ly; this records it and can put items back in stock.

import * as React from "react";
import { useRouter } from "next/navigation";

import { recordOutsideRefundAction } from "@/app/(admin)/orders/[id]/actions";
import { PAYMENT_METHODS } from "@/components/instore/ChargeSheet";
import { formatMoney } from "@/lib/format";
import { fromCents, toCents } from "@/lib/instore/cart";

type Line = { id: string; variant_id?: string; title_snapshot: string; quantity: number };

export function OutsideRefundForm({
  order,
}: {
  order: { id: string; store_id: string; grand_total: string; refunded_amount: string; currency_code: string; items: Line[] };
}) {
  const router = useRouter();
  const remainingCents = (toCents(order.grand_total) ?? 0) - (toCents(order.refunded_amount) ?? 0);
  const [amount, setAmount] = React.useState(fromCents(Math.max(remainingCents, 0)));
  const [method, setMethod] = React.useState("card_external");
  const [reason, setReason] = React.useState("");
  const [restock, setRestock] = React.useState<Record<string, number>>({});
  const [pending, setPending] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  // One id per refund attempt, reused on retry so a lost answer can't record it twice.
  const requestId = React.useRef(crypto.randomUUID());

  if (remainingCents <= 0) {
    return <p className="text-sm text-foreground-secondary">This sale has been fully refunded.</p>;
  }

  const stockable = order.items.filter((l) => l.variant_id);
  const amountCents = toCents(amount);
  const valid = amountCents !== null && amountCents > 0 && amountCents <= remainingCents;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setPending(true);
    setError(null);
    const res = await recordOutsideRefundAction(order.store_id, order.id, {
      refund_request_id: requestId.current,
      amount: amount.trim(),
      method,
      reason: reason.trim() || undefined,
      items: Object.entries(restock)
        .filter(([, q]) => q > 0)
        .map(([order_item_id, quantity]) => ({ order_item_id, quantity })),
    });
    setPending(false);
    if (!res.ok) return setError(res.error?.message ?? "Couldn't record the refund.");
    requestId.current = crypto.randomUUID();
    router.refresh();
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <p className="text-sm text-foreground-secondary">
        For money you gave back on your own terminal or in cash. Nothing is refunded through mark8ly.
      </p>
      <label className="flex flex-col gap-1 text-sm">
        Amount refunded
        <input
          inputMode="decimal"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          className="h-11 rounded-md border border-border bg-surface px-3 text-base tabular-nums"
        />
        <span className="text-xs text-foreground-tertiary">
          Up to {formatMoney(fromCents(remainingCents), order.currency_code)}
        </span>
      </label>

      <fieldset className="grid grid-cols-2 gap-2">
        <legend className="mb-1 text-sm font-medium">How did you refund it?</legend>
        {PAYMENT_METHODS.map((m) => (
          <label key={m.value} className="flex min-h-11 items-center gap-2 rounded-md border border-border px-3 text-sm">
            <input type="radio" name="refund-method" value={m.value} checked={method === m.value} onChange={() => setMethod(m.value)} />
            {m.label}
          </label>
        ))}
      </fieldset>

      {stockable.length > 0 ? (
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-1 text-sm font-medium">Put back in stock</legend>
          {stockable.map((l) => {
            const q = restock[l.id] ?? 0;
            const set = (n: number) => setRestock((r) => ({ ...r, [l.id]: n }));
            return (
              <div key={l.id} className="flex items-center justify-between gap-3 text-sm">
                <span className="min-w-0 truncate">{l.title_snapshot}</span>
                <span className="flex items-center gap-2">
                  <button type="button" aria-label={`One fewer ${l.title_snapshot} back in stock`} disabled={q === 0}
                    onClick={() => set(q - 1)} className="h-9 w-9 rounded-md border border-border disabled:opacity-40">−</button>
                  <span className="w-6 text-center tabular-nums">{q}</span>
                  <button type="button" aria-label={`One more ${l.title_snapshot} back in stock`} disabled={q >= l.quantity}
                    onClick={() => set(q + 1)} className="h-9 w-9 rounded-md border border-border disabled:opacity-40">+</button>
                </span>
              </div>
            );
          })}
        </fieldset>
      ) : null}

      <label className="flex flex-col gap-1 text-sm">
        Reason (optional)
        <input value={reason} maxLength={200} onChange={(e) => setReason(e.target.value)}
          className="h-11 rounded-md border border-border bg-surface px-3 text-base" />
      </label>

      {error ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
      <button type="submit" disabled={pending || !valid}
        className="min-h-11 self-start rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground disabled:cursor-not-allowed disabled:bg-ink-600">
        {pending ? "Recording…" : "Record refund"}
      </button>
    </form>
  );
}
