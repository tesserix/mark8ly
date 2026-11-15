"use client";

import * as React from "react";

import type { InStoreProduct, InStoreVariant } from "@/lib/api/instore-api";
import type { CartLine } from "@/lib/instore/cart";
import { formatMoney } from "@/lib/format";

type NewLine = Omit<CartLine, "key" | "quantity">;

const soldOut = (v: InStoreVariant) => v.available <= 0 && v.inventory_policy === "deny";

export function ProductGrid({
  catalog,
  currencyCode,
  disabled,
  onAdd,
}: {
  catalog: InStoreProduct[];
  currencyCode: string;
  disabled: boolean;
  onAdd: (line: NewLine) => void;
}) {
  const [query, setQuery] = React.useState("");
  const [picking, setPicking] = React.useState<InStoreProduct | null>(null);

  const q = query.trim().toLowerCase();
  const shown = q ? catalog.filter((p) => p.title.toLowerCase().includes(q)) : catalog;

  const add = (p: InStoreProduct, v: InStoreVariant) => {
    onAdd({
      variantId: v.id,
      title: p.title,
      optionSummary: v.option_summary,
      imageUrl: p.image_url,
      unitPrice: v.price,
    });
    setPicking(null);
  };

  return (
    <div className="flex min-h-0 flex-col gap-3">
      <input
        type="search"
        aria-label="Search products"
        placeholder="Search products"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="h-11 rounded-md border border-border bg-surface px-3 text-base"
      />

      {picking ? (
        <div className="flex flex-col gap-2">
          <p className="text-sm font-medium">{picking.title}</p>
          {picking.variants.map((v) => (
            <button
              key={v.id}
              type="button"
              disabled={disabled || soldOut(v)}
              onClick={() => add(picking, v)}
              className="flex min-h-11 items-center justify-between rounded-md border border-border px-3 text-left disabled:opacity-50"
            >
              <span>
                {v.option_summary ?? "Default"} · {formatMoney(v.price, currencyCode)}
              </span>
              {soldOut(v) ? <span className="text-xs text-danger">Sold out</span> : null}
            </button>
          ))}
          <button type="button" onClick={() => setPicking(null)} className="min-h-11 text-sm underline">
            Back to products
          </button>
        </div>
      ) : shown.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No products match “{query}”.</p>
      ) : (
        <div className="grid grid-cols-2 gap-3 overflow-y-auto sm:grid-cols-3 lg:grid-cols-4">
          {shown.map((p) => {
            const only = p.variants.length === 1 ? p.variants[0]! : null;
            return (
              <button
                key={p.id}
                type="button"
                disabled={disabled || (only !== null && soldOut(only))}
                onClick={() => (only ? add(p, only) : setPicking(p))}
                className="flex min-h-24 flex-col overflow-hidden rounded-md border border-border bg-surface text-left disabled:opacity-50"
              >
                {p.image_url ? (
                  <img src={p.image_url} alt="" className="aspect-square w-full object-cover" />
                ) : null}
                <span className="p-2 text-sm font-medium">{p.title}</span>
                <span className="px-2 pb-2 text-xs text-foreground-secondary">
                  {only ? formatMoney(only.price, currencyCode) : `${p.variants.length} options`}
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
