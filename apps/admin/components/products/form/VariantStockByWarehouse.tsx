"use client";

// VariantStockByWarehouse — per-warehouse stock for one variant
// (#177 PR 5e).
//
// Only rendered once a store has TWO OR MORE warehouses. With one, the
// merchant keeps the single Stock field and the ordinary product save,
// untouched: a store with one warehouse must see exactly what it saw
// before this slice, and "which of your one locations?" is not a question
// worth asking.
//
// Saving here is its own action rather than part of the product save,
// because the backend conserves the total by clearing the variant's
// sentinel row in the same transaction — see SetVariantStockByLocationInTx.

import { useEffect, useMemo, useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { SENTINEL_LOCATION_ID } from "@/lib/api/marketplace-api";
import type { Warehouse } from "@/lib/api/warehouses-api";
import { saveVariantStockByLocation } from "@/app/(admin)/products/actions";

interface VariantStockByWarehouseProps {
  storeId: string;
  productId: string;
  variantId: string;
  warehouses: Warehouse[];
  /** Current breakdown from the product detail response. */
  byLocation: Record<string, number>;
  /**
   * Move focus to the first warehouse input on mount. Set when the panel is
   * opened by a disclosure control: activating it is a deliberate
   * drill-down, and a keyboard or screen-reader user expects to land
   * somewhere rather than stay parked on a button whose state silently
   * changed.
   */
  autoFocus?: boolean;
}

export function VariantStockByWarehouse({
  storeId,
  productId,
  variantId,
  warehouses,
  byLocation,
  autoFocus = false,
}: VariantStockByWarehouseProps) {
  const router = useRouter();
  const firstInputRef = useRef<HTMLInputElement>(null);
  const [pending, startTransition] = useTransition();

  useEffect(() => {
    if (autoFocus) firstInputRef.current?.focus();
  }, [autoFocus]);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  // Units still on the sentinel are not at any warehouse yet. They are
  // shown, not silently folded into the first one: the merchant is about
  // to decide where they actually are, and pre-filling that decision with
  // a guess is how stock ends up recorded in the wrong place.
  const unassigned = byLocation[SENTINEL_LOCATION_ID] ?? 0;

  const [draft, setDraft] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {};
    for (const w of warehouses) {
      initial[w.id] = String(byLocation[w.id] ?? 0);
    }
    return initial;
  });

  const total = useMemo(
    () =>
      warehouses.reduce((sum, w) => {
        const n = Number.parseInt(draft[w.id] ?? "0", 10);
        return sum + (Number.isFinite(n) ? n : 0);
      }, 0),
    [draft, warehouses],
  );

  function handleSave() {
    setError(null);
    setSaved(false);

    const byWarehouse: Record<string, number> = {};
    for (const w of warehouses) {
      const n = Number.parseInt(draft[w.id] ?? "0", 10);
      if (!Number.isFinite(n) || n < 0) {
        setError(`${w.name} needs a whole number, zero or more.`);
        return;
      }
      byWarehouse[w.id] = n;
    }

    startTransition(async () => {
      const result = await saveVariantStockByLocation(
        storeId,
        productId,
        variantId,
        byWarehouse,
      );
      if (!result.ok) {
        setError(result.error?.message ?? "Could not save stock.");
        return;
      }
      setSaved(true);
      router.refresh();
    });
  }

  const inputClass =
    "w-28 rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

  return (
    <section className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium text-[color:var(--ink-900)]">
          Stock by warehouse
        </h3>
        <p className="text-xs text-[color:var(--ink-900)] opacity-60">
          Orders are filled from your warehouses in the order set on the
          Shipping page.
        </p>
      </div>

      {unassigned > 0 && (
        <div
          role="status"
          className="rounded-md border border-[color:var(--warning)]/30 bg-[color:var(--warning)]/[0.05] px-4 py-3 text-sm text-[color:var(--ink-900)]"
        >
          <strong className="font-medium">
            {unassigned} unit{unassigned === 1 ? "" : "s"} not yet assigned to a
            warehouse.
          </strong>{" "}
          They are still sellable, but nothing knows where they are. Set the
          numbers below and save to place them — the totals replace the
          unassigned count rather than adding to it.
        </div>
      )}

      <ul className="space-y-3">
        {warehouses.map((w, index) => (
          <li key={w.id} className="flex items-center justify-between gap-4">
            <label
              htmlFor={`stock-${variantId}-${w.id}`}
              className="min-w-0 text-sm text-[color:var(--ink-900)]"
            >
              {w.name}
              <span className="ml-2 opacity-50">{w.city}</span>
            </label>
            <input
              ref={index === 0 ? firstInputRef : undefined}
              id={`stock-${variantId}-${w.id}`}
              type="text"
              inputMode="numeric"
              value={draft[w.id] ?? "0"}
              disabled={pending}
              onChange={(e) => {
                setDraft((prev) => ({ ...prev, [w.id]: e.target.value }));
                setSaved(false);
              }}
              className={inputClass}
            />
          </li>
        ))}
      </ul>

      <div className="flex items-center justify-between border-t border-border-subtle pt-3">
        <span className="text-sm text-[color:var(--ink-900)] opacity-70">
          Total
        </span>
        <span className="text-sm font-medium text-[color:var(--ink-900)]">
          {total}
        </span>
      </div>

      {/* Status and the save control share one row, and the button is
          right-aligned so it sits under the column of stock inputs above
          rather than floating at the left margin. That alignment is what
          stops it reading as "dangling in the middle of the page" — the
          same complaint the product form's own Save had before it was
          docked under the header.

          It is deliberately SECONDARY. The page already has one filled
          --ink-900 primary, the docked "Save changes". Two filled primaries
          asking for different commits is precisely the "one accent per
          view" rule this system sets, and the weaker weight also tells the
          truth: warehouse stock is a smaller, self-contained commit than
          saving the product. */}
      <div className="flex items-center justify-between gap-4">
        <div aria-live="polite" className="min-w-0">
          {error && (
            <div
              role="alert"
              className="rounded-md border border-[color:var(--danger)]/25 bg-[color:var(--danger)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--danger)]"
            >
              {error}
            </div>
          )}
          {saved && !error && (
            <p className="text-sm text-[color:var(--moss-700)]">Stock saved.</p>
          )}
        </div>

        <button
          type="button"
          onClick={handleSave}
          disabled={pending}
          className="shrink-0 rounded-md border border-[color:var(--ink-200)] px-4 py-2 text-sm font-medium text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {pending ? "Saving…" : "Save stock"}
        </button>
      </div>
    </section>
  );
}
