"use client";

// components/orders/ShippingLabelPanel.tsx
//
// Shipping label panel for the order detail page. On mount, fetches any
// existing shipment. If one exists, displays details (carrier, tracking,
// label link). If not, offers an inline form to create a shipment via
// the carrier API. Only shown for actionable order statuses.

import { useCallback, useEffect, useState, useTransition } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { ShipmentResponse } from "@/lib/api/shipping-api";

import {
  createShipmentAction,
  getShipmentAction,
  type ShippingActionResult,
} from "@/app/(admin)/orders/[id]/shipping-actions";

// ─────────────────────────────────────────────────────────────────────────
// Props
// ─────────────────────────────────────────────────────────────────────────

interface ShippingLabelPanelProps {
  storeId: string;
  orderId: string;
  orderStatus: string;
}

// ─────────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────────

const ACTIONABLE_STATUSES = new Set(["pending", "confirmed", "fulfilled"]);

export function ShippingLabelPanel({
  storeId,
  orderId,
  orderStatus,
}: ShippingLabelPanelProps) {
  const [shipment, setShipment] = useState<ShipmentResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const data = await getShipmentAction(storeId, orderId);
      if (!cancelled) {
        setShipment(data);
        setLoading(false);
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, [storeId, orderId]);

  if (!ACTIONABLE_STATUSES.has(orderStatus)) {
    return null;
  }

  if (loading) {
    return (
      <section aria-labelledby="shipping-heading" className="flex flex-col gap-4">
        <h2
          id="shipping-heading"
          className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
        >
          Shipping
        </h2>
        <p className="text-sm text-[color:var(--ink-900)] opacity-60">
          Loading shipment details...
        </p>
      </section>
    );
  }

  if (shipment) {
    return (
      <section aria-labelledby="shipping-heading" className="flex flex-col gap-4">
        <h2
          id="shipping-heading"
          className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
        >
          Shipping
        </h2>
        <ShipmentDetails shipment={shipment} />
      </section>
    );
  }

  return (
    <section aria-labelledby="shipping-heading" className="flex flex-col gap-4">
      <h2
        id="shipping-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Shipping
      </h2>
      {!showForm ? (
        <button
          type="button"
          onClick={() => setShowForm(true)}
          className="inline-flex w-fit items-center gap-2 rounded-md border border-[color:var(--ink-900)] border-opacity-40 px-4 py-2 text-sm text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Create shipping label
        </button>
      ) : (
        <CreateShipmentForm
          storeId={storeId}
          orderId={orderId}
          onCreated={(s) => {
            setShipment(s);
            setShowForm(false);
          }}
          onCancel={() => setShowForm(false)}
        />
      )}
    </section>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Shipment details (existing shipment)
// ─────────────────────────────────────────────────────────────────────────

function ShipmentDetails({ shipment }: { shipment: ShipmentResponse }) {
  const eta = shipment.estimated_delivery
    ? new Date(shipment.estimated_delivery).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
      })
    : null;

  return (
    <div className="flex flex-col gap-3 border-l-2 border-[color:var(--moss-700)] bg-[color:var(--ink-900)] bg-opacity-[0.02] px-5 py-4">
      <div className="flex flex-col gap-2">
        <DetailRow label="Carrier" value={shipment.provider} />
        <DetailRow label="Service" value={shipment.service} />
        <DetailRow label="Tracking" value={shipment.tracking_number || "Pending"} />
        <DetailRow label="Status" value={shipment.status} />
        {eta && <DetailRow label="ETA" value={eta} />}
      </div>
      {shipment.label_url && (
        <a
          href={shipment.label_url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Print label
        </a>
      )}
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-20 shrink-0 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
        {label}
      </span>
      <span className="text-sm text-[color:var(--ink-900)]">{value}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Create shipment form
// ─────────────────────────────────────────────────────────────────────────

interface CreateShipmentFormProps {
  storeId: string;
  orderId: string;
  onCreated: (shipment: ShipmentResponse) => void;
  onCancel: () => void;
}

const CARRIERS = [
  { value: "delhivery", label: "Delhivery" },
  { value: "shipengine", label: "ShipEngine" },
  { value: "ninjavan", label: "NinjaVan" },
] as const;

const SERVICE_LEVELS = [
  { value: "standard", label: "Standard" },
  { value: "express", label: "Express" },
] as const;

function CreateShipmentForm({
  storeId,
  orderId,
  onCreated,
  onCancel,
}: CreateShipmentFormProps) {
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<ShippingActionResult["error"] | undefined>();
  const [provider, setProvider] = useState("");
  const [service, setService] = useState("");

  const submit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      setError(undefined);
      startTransition(async () => {
        const r = await createShipmentAction(storeId, orderId, {
          provider,
          service,
        });
        if (!r.ok) {
          setError(r.error);
          return;
        }
        if (r.data) {
          onCreated(r.data);
        }
      });
    },
    [storeId, orderId, provider, service, onCreated],
  );

  return (
    <div className="flex flex-col gap-4 border-l-2 border-[color:var(--moss-700)] bg-[color:var(--ink-900)] bg-opacity-[0.02] px-5 py-4">
      <h3 className="text-base text-[color:var(--ink-900)]">
        Create shipping label
      </h3>
      <form onSubmit={submit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
            Carrier
          </span>
          <Select
            value={provider || "__noop__"}
            onValueChange={(v) => setProvider(v === "__noop__" ? "" : v)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Select carrier" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__noop__" disabled>
                Select carrier
              </SelectItem>
              {CARRIERS.map((c) => (
                <SelectItem key={c.value} value={c.value}>
                  {c.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-2">
          <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
            Service level
          </span>
          <Select
            value={service || "__noop__"}
            onValueChange={(v) => setService(v === "__noop__" ? "" : v)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Select service" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__noop__" disabled>
                Select service
              </SelectItem>
              {SERVICE_LEVELS.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {error && (
          <p role="alert" className="text-sm text-[color:var(--danger,#5a1010)]">
            {error.message}
          </p>
        )}
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={pending || !provider || !service}
            className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? "Creating..." : "Create label"}
          </button>
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            className="text-sm text-[color:var(--ink-900)] opacity-70 hover:opacity-100 hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}
