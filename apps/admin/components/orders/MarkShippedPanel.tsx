"use client";

// Own-delivery shipping panel: the seller picks a saved courier, pastes a
// tracking number and marks the order shipped. Carrier orders use
// ShippingLabelPanel instead.

import Link from "next/link";
import { useEffect, useState, useTransition } from "react";

import {
  markShippedAction,
  updateTrackingAction,
} from "@/app/(admin)/orders/[id]/shipping-actions";
import type { Courier } from "@/lib/api/delivery-api";
import type { ShipmentResponse } from "@/lib/api/shipping-api";

interface Props {
  storeId: string;
  orderId: string;
  orderStatus: string;
  couriers: Courier[];
  initialShipment: ShipmentResponse | null;
}

const ACTIONABLE_STATUSES = new Set(["pending", "confirmed", "fulfilled"]);
const inputCls =
  "rounded-md border border-border-subtle bg-transparent px-3 py-2 text-sm text-foreground";
const btnCls =
  "inline-flex w-fit items-center rounded-md border border-[color:var(--ink-900)]/40 px-4 py-2 text-sm text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] disabled:opacity-50";

const lastKey = (storeId: string) => `m8.lastCourier.${storeId}`;

function readLast(storeId: string): string | null {
  try {
    return window.localStorage.getItem(lastKey(storeId));
  } catch {
    return null;
  }
}

function writeLast(storeId: string, id: string) {
  try {
    window.localStorage.setItem(lastKey(storeId), id);
  } catch {
    /* per-viewer convenience only */
  }
}

export function MarkShippedPanel({
  storeId,
  orderId,
  orderStatus,
  couriers,
  initialShipment,
}: Props) {
  const [shipment, setShipment] = useState(initialShipment);
  const [courierId, setCourierId] = useState(couriers[0]?.id ?? "");
  const [tracking, setTracking] = useState("");
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  // localStorage is client-only, so apply the remembered courier after mount.
  useEffect(() => {
    const last = readLast(storeId);
    if (last && couriers.some((c) => c.id === last)) setCourierId(last);
  }, [storeId, couriers]);

  if (!shipment && !ACTIONABLE_STATUSES.has(orderStatus)) return null;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (pending) return;
    setError(null);
    startTransition(async () => {
      const r = shipment
        ? await updateTrackingAction(storeId, orderId, shipment.id, {
            trackingNumber: tracking,
            courierId,
          })
        : await markShippedAction(storeId, orderId, {
            courierId,
            trackingNumber: tracking,
          });
      if (!r.ok || !r.data) {
        setError(r.error?.message ?? "Something went wrong.");
        return;
      }
      writeLast(storeId, courierId);
      setShipment(r.data);
      setEditing(false);
    });
  };

  const courierSelect = (
    <label className="flex flex-col gap-1 text-sm">
      Courier
      <select
        className={inputCls}
        value={courierId}
        onChange={(e) => setCourierId(e.target.value)}
      >
        {couriers.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
      </select>
    </label>
  );

  const trackingInput = (
    <label className="flex flex-col gap-1 text-sm">
      Tracking number
      <input
        className={inputCls}
        value={tracking}
        maxLength={100}
        onChange={(e) => setTracking(e.target.value)}
      />
    </label>
  );

  const errorEl = error ? (
    <p role="alert" className="text-xs text-[color:var(--danger)]">
      {error}
    </p>
  ) : null;

  let body;
  if (shipment && !editing) {
    const url = shipment.tracking_url?.startsWith("https://") ? shipment.tracking_url : null;
    body = (
      <>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
          <dt className="text-foreground-tertiary">Courier</dt>
          <dd>{shipment.courier_name ?? "Own delivery"}</dd>
          <dt className="text-foreground-tertiary">Tracking</dt>
          <dd className="font-mono">
            {url ? (
              <a href={url} target="_blank" rel="noopener noreferrer" className="underline">
                {shipment.tracking_number}
              </a>
            ) : (
              shipment.tracking_number || "None"
            )}
          </dd>
        </dl>
        <button
          type="button"
          className={btnCls}
          onClick={() => {
            setTracking(shipment.tracking_number);
            setCourierId(shipment.courier_id ?? courierId);
            setError(null);
            setEditing(true);
          }}
        >
          Edit tracking
        </button>
      </>
    );
  } else if (shipment) {
    body = (
      <form onSubmit={submit} className="flex flex-col gap-3">
        {courierSelect}
        {trackingInput}
        {errorEl}
        <div className="flex gap-2">
          <button type="submit" className={btnCls} disabled={pending || !tracking.trim()}>
            Save
          </button>
          <button type="button" className={btnCls} onClick={() => setEditing(false)}>
            Cancel
          </button>
        </div>
      </form>
    );
  } else if (couriers.length === 0) {
    body = (
      <p className="text-sm text-foreground-tertiary">
        Add a courier in{" "}
        <Link href="/settings/shipping" className="underline">
          Settings → Shipping
        </Link>{" "}
        to mark this order shipped.
      </p>
    );
  } else {
    body = (
      <form onSubmit={submit} className="flex flex-col gap-3">
        {courierSelect}
        {trackingInput}
        {errorEl}
        <button type="submit" className={btnCls} disabled={pending}>
          Mark as shipped
        </button>
      </form>
    );
  }

  return (
    <section aria-labelledby="shipping-heading" className="flex flex-col gap-4">
      <h2
        id="shipping-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Shipping
      </h2>
      {body}
    </section>
  );
}
