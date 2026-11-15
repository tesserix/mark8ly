"use client";

import * as React from "react";

import { type Cart, type CartAction, subtotal } from "@/lib/instore/cart";
import { formatMoney } from "@/lib/format";

const input = "h-11 rounded-md border border-border bg-surface px-3 text-base";

export function CartPanel({
  cart,
  currencyCode,
  canDiscount,
  lineNotes,
  charging,
  quoting,
  dispatch,
  onCharge,
  onDiscard,
}: {
  cart: Cart;
  currencyCode: string;
  canDiscount: boolean;
  // Per-variant warnings from the last refused sale, e.g. "Not enough stock".
  lineNotes: Record<string, string>;
  charging: boolean;
  // Only while the quote is in flight; the charge sheet may be open after.
  quoting: boolean;
  dispatch: (a: CartAction) => void;
  onCharge: () => void;
  onDiscard: () => void;
}) {
  const [custom, setCustom] = React.useState<{ title: string; price: string } | null>(null);
  const editable = !cart.locked;

  return (
    <section aria-label="Cart" className="flex flex-col gap-3">
      {cart.lines.length === 0 ? (
        <p className="text-sm text-foreground-secondary">Tap a product to add it.</p>
      ) : (
        <ul className="flex flex-col divide-y divide-border">
          {cart.lines.map((l) => {
            const note = l.variantId ? lineNotes[l.variantId] : undefined;
            return (
              <li key={l.key} className={`flex flex-col gap-1 py-2 ${note ? "bg-danger/10" : ""}`}>
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <p className="text-sm font-medium">{l.title}</p>
                    {l.optionSummary ? <p className="text-xs text-foreground-secondary">{l.optionSummary}</p> : null}
                  </div>
                  <p className="text-sm tabular-nums">
                    {formatMoney(Number(l.unitPrice) * l.quantity, currencyCode)}
                  </p>
                </div>
                {note ? <p className="text-xs text-danger">{note}</p> : null}
                {editable ? (
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      aria-label={`One fewer ${l.title}`}
                      onClick={() => dispatch({ type: "setQty", key: l.key, quantity: l.quantity - 1 })}
                      className="h-11 w-11 rounded-md border border-border"
                    >
                      −
                    </button>
                    <span className="w-8 text-center tabular-nums">{l.quantity}</span>
                    <button
                      type="button"
                      aria-label={`One more ${l.title}`}
                      onClick={() => dispatch({ type: "setQty", key: l.key, quantity: l.quantity + 1 })}
                      className="h-11 w-11 rounded-md border border-border"
                    >
                      +
                    </button>
                    <button
                      type="button"
                      aria-label={`Remove ${l.title}`}
                      onClick={() => dispatch({ type: "remove", key: l.key })}
                      className="ml-auto min-h-11 px-2 text-sm text-danger"
                    >
                      Remove
                    </button>
                  </div>
                ) : (
                  <p className="text-xs text-foreground-secondary">Qty {l.quantity}</p>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {canDiscount && editable ? (
        custom ? (
          <div className="flex flex-col gap-2 rounded-md border border-border p-3">
            <label className="flex flex-col gap-1 text-sm">
              Item name
              <input className={input} maxLength={200} value={custom.title}
                onChange={(e) => setCustom({ ...custom, title: e.target.value })} />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              Price
              <input className={input} inputMode="decimal" value={custom.price}
                onChange={(e) => setCustom({ ...custom, price: e.target.value })} />
            </label>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={!custom.title.trim() || !(Number(custom.price) > 0)}
                onClick={() => {
                  dispatch({ type: "add", line: { title: custom.title.trim(), unitPrice: Number(custom.price).toFixed(2), custom: true } });
                  setCustom(null);
                }}
                className="min-h-11 flex-1 rounded-md bg-primary px-4 text-primary-foreground disabled:opacity-50"
              >
                Add item
              </button>
              <button type="button" onClick={() => setCustom(null)} className="min-h-11 px-4 text-sm">
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <button type="button" onClick={() => setCustom({ title: "", price: "" })}
            className="min-h-11 rounded-md border border-dashed border-border text-sm">
            + Custom item
          </button>
        )
      ) : null}

      {canDiscount ? (
        <label className="flex items-center justify-between gap-2 text-sm">
          Discount
          <input className={`${input} w-28 text-right`} inputMode="decimal" disabled={!editable}
            value={cart.discount} onChange={(e) => dispatch({ type: "discount", value: e.target.value })} />
        </label>
      ) : null}
      <label className="flex flex-col gap-1 text-sm">
        Note
        <input className={input} disabled={!editable} value={cart.note}
          placeholder="e.g. engrave “Sam”"
          onChange={(e) => dispatch({ type: "note", value: e.target.value })} />
      </label>

      <p className="flex justify-between text-sm">
        <span>Subtotal</span>
        <span className="tabular-nums">{formatMoney(subtotal(cart), currencyCode)}</span>
      </p>
      <p className="text-xs text-foreground-secondary">Tax and the exact total are worked out when you charge.</p>

      <button
        type="button"
        disabled={cart.lines.length === 0 || charging}
        onClick={onCharge}
        className="min-h-12 rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
      >
        {quoting ? "Working out the total…" : "Charge"}
      </button>

      {cart.locked ? (
        <div className="rounded-md border border-danger/40 p-3 text-sm">
          <p>This sale may already be recorded. Check Your sales today before discarding it, or you could lose a sale.</p>
          <button type="button" onClick={onDiscard} className="mt-2 min-h-11 text-danger underline">
            Discard this sale
          </button>
        </div>
      ) : null}
    </section>
  );
}
