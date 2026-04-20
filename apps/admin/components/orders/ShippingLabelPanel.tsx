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
  updateShipmentStatusAction,
  emailShipmentLabelAction,
  refreshShipmentTrackingAction,
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
        <ShipmentDetails
          storeId={storeId}
          orderId={orderId}
          shipment={shipment}
          onUpdated={setShipment}
        />
        <AdvanceStatusBar
          storeId={storeId}
          orderId={orderId}
          shipment={shipment}
          onUpdated={setShipment}
        />
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

function ShipmentDetails({
  storeId,
  orderId,
  shipment,
  onUpdated,
}: {
  storeId: string;
  orderId: string;
  shipment: ShipmentResponse;
  onUpdated: (s: ShipmentResponse) => void;
}) {
  const eta = shipment.estimated_delivery
    ? new Date(shipment.estimated_delivery).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
      })
    : null;

  // The download always goes through the Next.js proxy route so the
  // browser can reuse the admin session cookie instead of exposing
  // the carrier token. The backend canonical label_url may already
  // point here, but we build it locally too so the button stays wired
  // even for legacy shipment rows that were saved with an empty URL.
  const labelProxyURL = `/api/admin/stores/${storeId}/orders/${orderId}/shipments/${shipment.id}/label`;

  return (
    <div className="flex flex-col gap-3 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-5 py-4 shadow-sm">
      <div className="flex flex-col gap-2">
        <DetailRow label="Carrier" value={shipment.provider} />
        <DetailRow label="Service" value={shipment.service} />
        <DetailRow label="Tracking" value={shipment.tracking_number || "Pending"} />
        <DetailRow label="Status" value={shipment.status} />
        {eta && <DetailRow label="ETA" value={eta} />}
      </div>
      <LabelActions
        storeId={storeId}
        orderId={orderId}
        shipment={shipment}
        labelProxyURL={labelProxyURL}
        onUpdated={onUpdated}
      />
    </div>
  );
}

function LabelActions({
  storeId,
  orderId,
  shipment,
  labelProxyURL,
  onUpdated,
}: {
  storeId: string;
  orderId: string;
  shipment: ShipmentResponse;
  labelProxyURL: string;
  onUpdated: (s: ShipmentResponse) => void;
}) {
  const [showEmailForm, setShowEmailForm] = useState(false);
  const [recipient, setRecipient] = useState("");
  const [emailPending, emailStartTransition] = useTransition();
  const [emailMsg, setEmailMsg] = useState<
    { kind: "ok" | "err"; text: string } | null
  >(null);
  const [refreshPending, refreshStartTransition] = useTransition();
  const [refreshMsg, setRefreshMsg] = useState<string | null>(null);

  const sendEmail = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      setEmailMsg(null);
      emailStartTransition(async () => {
        const r = await emailShipmentLabelAction(
          storeId,
          orderId,
          shipment.id,
          recipient,
        );
        if (!r.ok) {
          setEmailMsg({ kind: "err", text: r.error.message });
          return;
        }
        setEmailMsg({ kind: "ok", text: `Label sent to ${recipient}.` });
        setRecipient("");
        setShowEmailForm(false);
      });
    },
    [storeId, orderId, shipment.id, recipient],
  );

  const refresh = useCallback(() => {
    setRefreshMsg(null);
    refreshStartTransition(async () => {
      const r = await refreshShipmentTrackingAction(storeId, orderId, shipment.id);
      if (!r.ok) {
        setRefreshMsg(r.error?.message ?? "Tracking refresh failed.");
        return;
      }
      if (r.data) {
        onUpdated(r.data);
        setRefreshMsg("Tracking synced.");
      }
    });
  }, [storeId, orderId, shipment.id, onUpdated]);

  return (
    <div className="flex flex-col gap-2 pt-1">
      <div className="flex flex-wrap items-center gap-2">
        <a
          href={labelProxyURL}
          download
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Download label
        </a>
        <button
          type="button"
          onClick={() => {
            setShowEmailForm((v) => !v);
            setEmailMsg(null);
          }}
          className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/30 px-4 py-2 text-sm text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          {showEmailForm ? "Cancel email" : "Email label"}
        </button>
        <button
          type="button"
          onClick={refresh}
          disabled={refreshPending}
          className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/30 px-4 py-2 text-sm text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
        >
          {refreshPending ? "Syncing…" : "Refresh tracking"}
        </button>
      </div>
      {refreshMsg && (
        <p className="text-xs text-[color:var(--ink-900)] opacity-70">
          {refreshMsg}
        </p>
      )}
      {showEmailForm && (
        <form onSubmit={sendEmail} className="flex flex-col gap-2 pt-2">
          <label className="flex flex-col gap-1 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
            Send to
            <input
              type="email"
              value={recipient}
              onChange={(e) => setRecipient(e.target.value)}
              placeholder="warehouse@example.com"
              required
              className="rounded-md border border-[color:var(--ink-900)]/30 px-3 py-2 text-sm normal-case tracking-normal text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none"
            />
          </label>
          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={emailPending || !recipient}
              className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {emailPending ? "Sending…" : "Send label"}
            </button>
          </div>
        </form>
      )}
      {emailMsg && (
        <p
          role="status"
          className={`text-xs ${
            emailMsg.kind === "ok"
              ? "text-[color:var(--moss-700)]"
              : "text-red-700"
          }`}
        >
          {emailMsg.text}
        </p>
      )}
    </div>
  );
}

// AdvanceStatusBar — three inline buttons the merchant clicks as the
// package moves. Each writes an order_event row so the customer's
// delivery timeline reflects the new state on its next poll.
function AdvanceStatusBar({
  storeId,
  orderId,
  shipment,
  onUpdated,
}: {
  storeId: string;
  orderId: string;
  shipment: ShipmentResponse;
  onUpdated: (s: ShipmentResponse) => void;
}) {
  const [pending, startTransition] = useTransition();
  const [target, setTarget] = useState<string | null>(null);

  const disabledFor = (status: string): boolean => {
    if (pending) return true;
    const order = ["created", "in_transit", "out_for_delivery", "delivered"];
    const cur = order.indexOf(shipment.status);
    const next = order.indexOf(status);
    return cur >= 0 && next >= 0 && next <= cur;
  };

  function advance(status: string) {
    setTarget(status);
    startTransition(async () => {
      const r = await updateShipmentStatusAction(storeId, orderId, shipment.id, { status });
      setTarget(null);
      if (r.ok && r.data) onUpdated(r.data);
    });
  }

  const steps: Array<{ status: string; label: string }> = [
    { status: "in_transit", label: "Mark in transit" },
    { status: "out_for_delivery", label: "Out for delivery" },
    { status: "delivered", label: "Mark delivered" },
  ];

  return (
    <div className="flex flex-wrap items-center gap-2 pt-1">
      {steps.map((s) => (
        <button
          key={s.status}
          type="button"
          onClick={() => advance(s.status)}
          disabled={disabledFor(s.status)}
          className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/20 px-3 py-1.5 text-xs text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {target === s.status ? "Updating…" : s.label}
        </button>
      ))}
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
  // The customer already picked a carrier + service level at checkout;
  // the admin step is just "approve and generate the label". Defaults
  // reflect the most common case (Delhivery + Standard) so staff can
  // click through in one action. An "Override" disclosure lets ops pick
  // a different carrier when needed.
  const [provider, setProvider] = useState("delhivery");
  const [service, setService] = useState("standard");
  const [showOverride, setShowOverride] = useState(false);

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
    <div className="flex flex-col gap-4 rounded-md border border-[color:var(--ink-900)]/10 bg-white px-5 py-4 shadow-sm">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-base text-[color:var(--ink-900)]">
          Approve &amp; generate label
        </h3>
        <p className="text-xs text-[color:var(--ink-900)] opacity-60">
          Picked by customer: {CARRIERS.find((c) => c.value === provider)?.label ?? provider}
          {" · "}
          {SERVICE_LEVELS.find((s) => s.value === service)?.label ?? service}
        </p>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-4">
        {showOverride && (
          <>
            <div className="flex flex-col gap-2">
              <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
                Carrier
              </span>
              <Select value={provider} onValueChange={(v) => setProvider(v)}>
                <SelectTrigger>
                  <SelectValue placeholder="Select carrier" />
                </SelectTrigger>
                <SelectContent>
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
              <Select value={service} onValueChange={(v) => setService(v)}>
                <SelectTrigger>
                  <SelectValue placeholder="Select service" />
                </SelectTrigger>
                <SelectContent>
                  {SERVICE_LEVELS.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </>
        )}
        {error && (
          <p role="alert" className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
            {error.message}
          </p>
        )}
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={pending || !provider || !service}
            className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? "Generating…" : "Approve & generate label"}
          </button>
          <button
            type="button"
            onClick={() => setShowOverride((v) => !v)}
            className="text-xs text-[color:var(--ink-900)] underline-offset-4 opacity-70 hover:opacity-100 hover:underline"
          >
            {showOverride ? "Hide override" : "Override carrier/service"}
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
