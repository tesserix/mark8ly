"use client";

// The in-store sale screen (#1105). The seller rings up, charges the
// server's quoted total on their own terminal, then taps Paid. "Sale
// recorded" appears only once the server has the order; anything less keeps
// the cart, and a sale with no answer is retried with the same key so it can
// never be recorded twice.

import * as React from "react";
import Link from "next/link";

import { quoteSaleAction, recordSaleAction } from "@/app/(admin)/sell/actions";
import type { InStoreProduct } from "@/lib/api/instore-api";
import { type Cart, type CartAction, cartReducer, emptyCart, loadCart, saveCart, subtotal, toSaleLines } from "@/lib/instore/cart";
import { formatMoney } from "@/lib/format";

import { CartPanel } from "./CartPanel";
import { ChargeSheet, type PaymentMethod } from "./ChargeSheet";
import { ProductGrid } from "./ProductGrid";
import { SaleDone } from "./SaleDone";

export function SaleScreen({
  storeId,
  currencyCode,
  catalog,
  canDiscount,
}: {
  storeId: string;
  currencyCode: string;
  catalog: InStoreProduct[];
  canDiscount: boolean;
}) {
  // Loaded after mount: the server render has no local storage.
  const [cart, setCart] = React.useState<Cart>(emptyCart);
  const [hydrated, setHydrated] = React.useState(false);
  const dispatch = React.useCallback((a: CartAction) => setCart((c) => cartReducer(c, a)), []);

  React.useEffect(() => {
    setCart(loadCart(storeId));
    setHydrated(true);
  }, [storeId]);
  React.useEffect(() => {
    if (hydrated) saveCart(storeId, cart);
  }, [hydrated, storeId, cart]);

  const [total, setTotal] = React.useState<string | null>(null);
  const [quoting, setQuoting] = React.useState(false);
  const [method, setMethod] = React.useState<PaymentMethod>("card_external");
  const [tendered, setTendered] = React.useState("");
  const [otherLabel, setOtherLabel] = React.useState("");
  const [pending, setPending] = React.useState(false);
  const [message, setMessage] = React.useState<string | null>(null);
  const [lineNotes, setLineNotes] = React.useState<Record<string, string>>({});
  const [done, setDone] = React.useState<{ orderNumber: string; grandTotal: string } | null>(null);
  const [cartOpen, setCartOpen] = React.useState(false);

  const discount = cart.discount.trim() || undefined;

  async function charge() {
    setQuoting(true);
    setMessage(null);
    setLineNotes({});
    const res = await quoteSaleAction(storeId, { lines: toSaleLines(cart), discount });
    setQuoting(false);
    if (!res.ok) {
      setMessage(res.error.message);
      return;
    }
    dispatch({ type: "beginCharge" });
    setTotal(res.data.grand_total);
  }

  async function paid() {
    if (!total || !cart.idempotencyKey) return;
    setPending(true);
    setMessage(null);
    const res = await recordSaleAction(storeId, {
      idempotency_key: cart.idempotencyKey,
      lines: toSaleLines(cart),
      discount,
      payment_method: method,
      payment_method_label: method === "other" ? otherLabel.trim() : undefined,
      cash_tendered: method === "cash" ? tendered.trim() : undefined,
      note: cart.note.trim() || undefined,
      expected_total: total,
    });
    setPending(false);

    if (res.ok) {
      setDone({ orderNumber: res.orderNumber, grandTotal: res.grandTotal || total });
      dispatch({ type: "reset" });
      setTotal(null);
      return;
    }
    switch (res.kind) {
      case "unreachable":
        dispatch({ type: "uncertain" });
        setMessage("No connection, sale not recorded yet. Check the connection and try again; it won't be recorded twice.");
        return;
      case "out_of_stock": {
        dispatch({ type: "refused" });
        const v = catalog.flatMap((p) => p.variants).find((x) => x.id === res.variantId);
        setLineNotes({ [res.variantId]: v && v.available > 0 ? `Only ${v.available} left` : "Out of stock" });
        setTotal(null);
        return;
      }
      case "total_changed":
        // Proven not recorded: charge the new amount as a fresh attempt.
        dispatch({ type: "refused" });
        dispatch({ type: "beginCharge" });
        setTotal(res.grandTotal);
        setMessage(`The total changed to ${formatMoney(res.grandTotal, currencyCode)}. Charge that amount instead.`);
        return;
      case "rejected":
        // After a lost answer the first attempt may exist, so a refusal
        // now proves nothing: stay locked until it is checked or discarded.
        if (!cart.locked) dispatch({ type: "refused" });
        setMessage(res.message);
        return;
    }
  }

  function newSale() {
    setDone(null);
    setMethod("card_external");
    setTendered("");
    setOtherLabel("");
    setMessage(null);
  }

  const count = cart.lines.reduce((n, l) => n + l.quantity, 0);

  return (
    <div className="flex flex-col gap-4 pb-20 md:pb-0">
      <div className="flex justify-end">
        <Link href="/logout" className="min-h-11 text-sm underline">
          Switch person
        </Link>
      </div>

      <div className="grid gap-6 md:grid-cols-[1fr_minmax(20rem,24rem)]">
        <ProductGrid
          catalog={catalog}
          currencyCode={currencyCode}
          disabled={cart.locked || total !== null || done !== null}
          onAdd={(line) => {
            setLineNotes({});
            dispatch({ type: "add", line });
          }}
        />

        <div className={`${cartOpen ? "fixed inset-x-0 bottom-0 z-40 max-h-[85vh] overflow-y-auto rounded-t-xl border-t border-border bg-background p-4 shadow-lg" : "hidden"} md:static md:block md:max-h-none md:rounded-none md:border-0 md:p-0 md:shadow-none`}>
          {done ? (
            <SaleDone orderNumber={done.orderNumber} grandTotal={done.grandTotal} currencyCode={currencyCode} onNewSale={newSale} />
          ) : total !== null ? (
            <ChargeSheet
              total={total}
              currencyCode={currencyCode}
              method={method}
              tendered={tendered}
              otherLabel={otherLabel}
              pending={pending}
              uncertain={cart.locked}
              message={message}
              onMethod={setMethod}
              onTendered={setTendered}
              onOtherLabel={setOtherLabel}
              onPaid={paid}
              onBack={() => {
                setTotal(null);
                setMessage(null);
              }}
            />
          ) : message ? (
            <p role="alert" className="mb-3 text-sm text-danger">{message}</p>
          ) : null}

          {done ? null : (
            <div className={total !== null ? "mt-4" : ""}>
              <CartPanel
                cart={cart}
                currencyCode={currencyCode}
                canDiscount={canDiscount}
                lineNotes={lineNotes}
                charging={quoting || total !== null}
                dispatch={dispatch}
                onCharge={charge}
                onDiscard={() => {
                  dispatch({ type: "reset" });
                  setTotal(null);
                  setMessage(null);
                }}
              />
            </div>
          )}
        </div>
      </div>

      <button
        type="button"
        onClick={() => setCartOpen((o) => !o)}
        className="fixed inset-x-4 bottom-4 z-50 min-h-12 rounded-md bg-primary px-4 text-base font-medium text-primary-foreground md:hidden"
      >
        {cartOpen ? "Back to products" : `Cart · ${count} · ${formatMoney(subtotal(cart), currencyCode)}`}
      </button>
    </div>
  );
}
