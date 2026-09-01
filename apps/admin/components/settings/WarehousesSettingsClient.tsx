"use client";

// WarehousesSettingsClient — the store's pickup locations (#177 PR 5c).
//
// List, add, edit, reorder, set default, remove. Ordering is not cosmetic:
// the allocator fills warehouses in this order, so moving one to the top
// changes which location ships an order.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { AlertDialog } from "@tesserix/web";

import type { Warehouse, WarehouseWriteInput } from "@/lib/api/warehouses-api";
import { WarehouseForm } from "./WarehouseForm";
import {
  saveWarehouse,
  deleteWarehouse,
  makeDefaultWarehouse,
  reorderWarehouseList,
} from "@/app/(admin)/settings/warehouses/actions";

interface WarehousesSettingsClientProps {
  warehouses: Warehouse[];
  editable: boolean;
  storeCountry: string;
}

type Editing = { mode: "none" } | { mode: "new" } | { mode: "edit"; id: string };

export function WarehousesSettingsClient({
  warehouses,
  editable,
  storeCountry,
}: WarehousesSettingsClientProps) {
  const router = useRouter();
  const [editing, setEditing] = useState<Editing>({ mode: "none" });
  const [confirmRemove, setConfirmRemove] = useState<Warehouse | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function done(message?: string) {
    setEditing({ mode: "none" });
    setError(null);
    if (message) setNotice(message);
    router.refresh();
  }

  async function handleSave(id: string | null, input: WarehouseWriteInput) {
    const result = await saveWarehouse(id, input);
    if (!result.ok) return { ok: false, message: result.message };
    done(id ? "Warehouse updated." : "Warehouse added.");
    return { ok: true };
  }

  function handleRemoveConfirmed() {
    const target = confirmRemove;
    if (!target) return;
    setConfirmRemove(null);
    setError(null);
    startTransition(async () => {
      const result = await deleteWarehouse(target.id);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      // Say which of the two things happened. An archive that reported
      // itself as a delete would leave the merchant expecting the row to
      // be gone from past orders, where it deliberately still appears.
      const removal = result.data;
      if (removal?.outcome === "archived") {
        setNotice(
          removal.units_remaining > 0
            ? `${target.name} was archived because it has order history. ${removal.units_remaining} unit${removal.units_remaining === 1 ? "" : "s"} of stock stayed there and can no longer be sold — move it to another warehouse if you still need it.`
            : `${target.name} was archived because it has order history. It stays on past orders but will not receive new ones.`,
        );
      } else {
        setNotice(`${target.name} was removed.`);
      }
      router.refresh();
    });
  }

  function handleMakeDefault(w: Warehouse) {
    setError(null);
    startTransition(async () => {
      const result = await makeDefaultWarehouse(w.id);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setNotice(`${w.name} is now the default warehouse.`);
      router.refresh();
    });
  }

  // move sends the COMPLETE reordered set, which is what the API requires:
  // a delta over a list that changed underneath reorders the wrong rows.
  function move(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= warehouses.length) return;
    const next = warehouses.map((w, i) => {
      if (i === index) return warehouses[target] as Warehouse;
      if (i === target) return warehouses[index] as Warehouse;
      return w;
    });

    setError(null);
    startTransition(async () => {
      const result = await reorderWarehouseList(next.map((w) => w.id));
      if (!result.ok) {
        setError(result.message);
        return;
      }
      router.refresh();
    });
  }

  if (editing.mode === "new") {
    return (
      <section className="border-t border-border-subtle pt-8">
        <h2 className="mb-6 font-serif text-xl text-[color:var(--ink-900)]">
          Add a warehouse
        </h2>
        <WarehouseForm
          storeCountry={storeCountry}
          onSubmit={(input) => handleSave(null, input)}
          onCancel={() => setEditing({ mode: "none" })}
        />
      </section>
    );
  }

  if (editing.mode === "edit") {
    const target = warehouses.find((w) => w.id === editing.id);
    if (target) {
      return (
        <section className="border-t border-border-subtle pt-8">
          <h2 className="mb-6 font-serif text-xl text-[color:var(--ink-900)]">
            Edit {target.name}
          </h2>
          <WarehouseForm
            existing={target}
            storeCountry={storeCountry}
            onSubmit={(input) => handleSave(target.id, input)}
            onCancel={() => setEditing({ mode: "none" })}
          />
        </section>
      );
    }
  }

  return (
    <section>
      <div aria-live="polite" className="space-y-3">
        {notice && (
          <div
            role="status"
            className="rounded-md border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]"
          >
            {notice}
          </div>
        )}
        {error && (
          <div
            role="alert"
            className="rounded-md border border-[color:var(--danger)]/25 bg-[color:var(--danger)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--danger)]"
          >
            {error}
          </div>
        )}
      </div>

      {warehouses.length === 0 ? (
        <div className="border-t border-border-subtle py-10">
          <p className="text-sm text-foreground-tertiary">
            No warehouses yet. Carriers cannot quote a rate without an origin
            address, so add the location you ship from.
          </p>
          {editable && (
            <button
              type="button"
              onClick={() => setEditing({ mode: "new" })}
              className="mt-4 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Add warehouse
            </button>
          )}
        </div>
      ) : (
        <>
          <ul className="mt-6 border-t border-border-subtle">
            {warehouses.map((w, i) => (
              <li
                key={w.id}
                className="flex flex-wrap items-start justify-between gap-4 border-b border-border-subtle py-5"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2.5">
                    <h3 className="font-serif text-lg text-[color:var(--ink-900)]">
                      {w.name}
                    </h3>
                    {w.is_default && (
                      <span className="rounded-full bg-[color:var(--moss-700)]/10 px-2.5 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
                        Default
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-sm text-foreground-secondary">
                    {[w.line1, w.line2, w.city, w.region, w.postal_code, w.country_code]
                      .filter(Boolean)
                      .join(", ")}
                  </p>
                  <p className="mt-0.5 text-sm text-foreground-tertiary">{w.phone}</p>
                </div>

                {editable && (
                  <div className="flex items-center gap-2">
                    {warehouses.length > 1 && (
                      <div className="flex items-center gap-1">
                        <OrderButton
                          label={`Move ${w.name} earlier in the fill order`}
                          glyph="↑"
                          disabled={pending || i === 0}
                          onClick={() => move(i, -1)}
                        />
                        <OrderButton
                          label={`Move ${w.name} later in the fill order`}
                          glyph="↓"
                          disabled={pending || i === warehouses.length - 1}
                          onClick={() => move(i, 1)}
                        />
                      </div>
                    )}
                    {!w.is_default && (
                      <SecondaryButton
                        disabled={pending}
                        onClick={() => handleMakeDefault(w)}
                      >
                        Make default
                      </SecondaryButton>
                    )}
                    <SecondaryButton
                      disabled={pending}
                      onClick={() => setEditing({ mode: "edit", id: w.id })}
                    >
                      Edit
                    </SecondaryButton>
                    <button
                      type="button"
                      disabled={pending}
                      onClick={() => setConfirmRemove(w)}
                      className="rounded-md border border-[color:var(--ink-900)]/10 px-3 py-1.5 text-sm font-medium text-[color:var(--danger)] transition-colors hover:border-[color:var(--danger)]/30 hover:bg-[color:var(--danger)]/[0.04] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      Remove
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>

          {warehouses.length > 1 && (
            <p className="mt-4 text-xs text-foreground-tertiary">
              Orders are allocated from the top of this list down. A warehouse
              lower in the order is only used once the ones above it run out.
            </p>
          )}

          {editable && (
            <button
              type="button"
              onClick={() => setEditing({ mode: "new" })}
              disabled={pending}
              className="mt-6 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
            >
              Add warehouse
            </button>
          )}
        </>
      )}

      <AlertDialog
        isOpen={confirmRemove !== null}
        onClose={() => setConfirmRemove(null)}
        title={confirmRemove ? `Remove ${confirmRemove.name}?` : ""}
        message={
          confirmRemove
            ? "If this warehouse has shipped anything it is archived rather than deleted, so past orders keep their record of where they shipped from. Either way it stops receiving new orders."
            : ""
        }
        type="confirm"
        confirmLabel="Remove"
        cancelLabel="Cancel"
        onConfirm={handleRemoveConfirmed}
        onCancel={() => setConfirmRemove(null)}
      />
    </section>
  );
}

function SecondaryButton({
  children,
  disabled,
  onClick,
}: {
  children: React.ReactNode;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="rounded-md border border-[color:var(--ink-900)]/10 px-3 py-1.5 text-sm font-medium text-[color:var(--ink-900)]/70 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
    >
      {children}
    </button>
  );
}

// Reordering is keyboard-operable by construction: two real buttons with
// spelled-out labels, not a drag handle. Drag-only reordering would put the
// fill order — which decides where an order ships from — out of reach of
// anyone not using a mouse.
function OrderButton({
  label,
  glyph,
  disabled,
  onClick,
}: {
  label: string;
  glyph: string;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="rounded-md border border-[color:var(--ink-900)]/10 px-2 py-1.5 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-30"
    >
      <span aria-hidden="true">{glyph}</span>
    </button>
  );
}
