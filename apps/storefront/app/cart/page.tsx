"use client";

// apps/storefront/app/cart/page.tsx
//
// Cart page. Lists items from useCart(), qty controls, remove,
// subtotal, and a disabled checkout button. Per-store via the
// CartProvider's slug scoping.

import Image from "next/image";
import Link from "next/link";
import { useCart } from "@/components/CartProvider";

export default function CartPage() {
  const { items, updateQty, remove, subtotal, count, clear } = useCart();

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-3xl px-6 py-12 sm:px-8">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--ink-900)]">
          Cart
        </h1>

        {items.length === 0 ? (
          <EmptyCart />
        ) : (
          <>
            <ul className="mt-8 divide-y divide-[color:var(--ink-900)]/10">
              {items.map((item) => (
                <CartRow
                  key={`${item.productId}-${item.variantId}`}
                  item={item}
                  onQtyChange={(qty) =>
                    updateQty(item.productId, item.variantId, qty)
                  }
                  onRemove={() => remove(item.productId, item.variantId)}
                />
              ))}
            </ul>

            <footer className="mt-8 border-t border-[color:var(--ink-900)]/10 pt-6">
              <div aria-live="polite" className="flex items-baseline justify-between">
                <p className="text-sm text-[color:var(--ink-900)] opacity-60">
                  {count} {count === 1 ? "item" : "items"}
                </p>
                <p
                  className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]"
                  style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
                >
                  {formatSubtotal(subtotal, items[0]?.currencyCode ?? "USD")}
                </p>
              </div>

              <div className="mt-6 flex items-center gap-4">
                <Link
                  href="/checkout"
                  className="rounded-md bg-[color:var(--ink-900)] px-6 py-3 text-sm font-medium text-[color:var(--paper-200)] transition-all duration-150 hover:opacity-90 active:scale-[0.99] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                >
                  Checkout
                </Link>
                <button
                  type="button"
                  onClick={clear}
                  className="text-sm text-[color:var(--ink-900)] opacity-50 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                >
                  Clear cart
                </button>
              </div>
            </footer>
          </>
        )}
      </div>
    </main>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function EmptyCart() {
  return (
    <div className="mt-12 text-center">
      <p className="text-lg text-[color:var(--ink-900)] opacity-60">
        Your cart is empty.
      </p>
      <Link
        href="/products"
        className="mt-4 inline-block text-sm font-semibold text-[color:var(--moss-700)] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Continue shopping →
      </Link>
    </div>
  );
}

interface CartRowProps {
  item: {
    productId: string;
    variantId: string;
    handle: string;
    title: string;
    priceAmount: string;
    currencyCode: string;
    qty: number;
    imageUrl?: string;
  };
  onQtyChange: (qty: number) => void;
  onRemove: () => void;
}

function CartRow({ item, onQtyChange, onRemove }: CartRowProps) {
  const lineTotal = Number.parseFloat(item.priceAmount) * item.qty;

  return (
    <li className="flex gap-4 py-6">
      {item.imageUrl ? (
        <div className="relative h-20 w-20 shrink-0 overflow-hidden rounded-md bg-[color:var(--paper-200)]">
          <Image
            src={item.imageUrl}
            alt={item.title}
            fill
            sizes="80px"
            className="object-cover"
          />
        </div>
      ) : (
        <div className="h-20 w-20 shrink-0 rounded-md bg-[color:var(--paper-200)]" />
      )}

      <div className="flex flex-1 flex-col justify-between">
        <div className="flex items-start justify-between">
          <Link
            href={`/products/${item.handle}`}
            className="text-sm font-medium text-[color:var(--ink-900)] hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            {item.title}
          </Link>
          <p
            className="text-sm text-[color:var(--ink-900)]"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {formatSubtotal(lineTotal, item.currencyCode)}
          </p>
        </div>

        <div className="mt-2 flex items-center gap-3">
          <label className="sr-only" htmlFor={`qty-${item.variantId}`}>
            Quantity for {item.title}
          </label>
          <div className="flex items-center rounded-md border border-[color:var(--ink-900)]/15">
            <button
              type="button"
              disabled={item.qty <= 1}
              onClick={() => onQtyChange(item.qty - 1)}
              aria-label={`Decrease quantity of ${item.title}`}
              className="px-2 py-1 text-sm text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 disabled:opacity-20 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              −
            </button>
            <input
              id={`qty-${item.variantId}`}
              type="number"
              min={1}
              max={999}
              value={item.qty}
              onChange={(e) => {
                const n = Number.parseInt(e.target.value, 10);
                if (Number.isFinite(n) && n > 0) onQtyChange(n);
              }}
              className="w-10 bg-transparent text-center text-sm text-[color:var(--ink-900)] [appearance:textfield] focus:outline-none [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            />
            <button
              type="button"
              onClick={() => onQtyChange(item.qty + 1)}
              aria-label={`Increase quantity of ${item.title}`}
              className="px-2 py-1 text-sm text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              +
            </button>
          </div>

          <button
            type="button"
            onClick={onRemove}
            className="text-xs text-[color:var(--ink-900)] opacity-40 transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Remove
          </button>
        </div>
      </div>
    </li>
  );
}

function formatSubtotal(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(2)}`;
  }
}
