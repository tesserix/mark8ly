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
  deleteShipmentAction,
  schedulePickupAction,
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
          className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
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
          className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
        >
          Shipping
        </h2>
        <ShipmentDetails
          storeId={storeId}
          orderId={orderId}
          shipment={shipment}
          onUpdated={setShipment}
          onCleared={() => setShipment(null)}
        />
      </section>
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
  onCleared,
}: {
  storeId: string;
  orderId: string;
  shipment: ShipmentResponse;
  onUpdated: (s: ShipmentResponse) => void;
  onCleared: () => void;
}) {
  const eta = shipment.estimated_delivery
    ? new Date(shipment.estimated_delivery).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
      })
    : null;

  // Pickup line. Only render when the carrier has scheduled one —
  // pre-auto-schedule shipments leave this null and we hide the row
  // entirely. Format matches the task spec: "Fri, Apr 21, 14:00".
  const pickup = shipment.pickup_scheduled_for
    ? formatPickupDisplay(shipment.pickup_scheduled_for)
    : null;

  // Waybills starting with TEST-DLV- were written by the old test-mode
  // stub that fabricated a tracking number when Delhivery rejected the
  // create call. Delhivery never heard of these IDs, so every Download
  // / Email / Refresh action will fail. Flag them inline and offer
  // delete-and-retry instead of silently letting the user download a
  // JSON error as "label.txt".
  const isStub = shipment.tracking_number.startsWith("TEST-DLV-");

  const labelProxyURL = `/api/admin/stores/${storeId}/orders/${orderId}/shipments/${shipment.id}/label`;

  return (
    <div className="flex flex-col gap-5 rounded-md border border-border-subtle bg-[color:var(--background-elevated)] px-5 py-5">
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
        {shipment.provider && (
          <DetailRow label="Carrier" value={shipment.provider} />
        )}
        {shipment.service && (
          <DetailRow label="Service" value={shipment.service} />
        )}
        <DetailRow
          label="Tracking"
          value={shipment.tracking_number || "Pending"}
          mono
        />
        <DetailRow label="Status" value={shipment.status} />
        {eta && <DetailRow label="ETA" value={eta} />}
        {pickup && <DetailRow label="Pickup" value={pickup} />}
      </dl>
      {isStub && (
        <div
          role="status"
          className="rounded-md border border-[color:var(--warning)]/30 bg-[color:var(--warning)]/[0.06] px-4 py-3 text-xs text-[color:var(--warning)]"
        >
          This shipment uses a mock tracking number from an earlier test run
          and won&apos;t produce a real label. Clear it and create a new
          shipment to generate a real waybill.
        </div>
      )}
      <LabelActions
        storeId={storeId}
        orderId={orderId}
        shipment={shipment}
        labelProxyURL={labelProxyURL}
        isStub={isStub}
        onUpdated={onUpdated}
        onCleared={onCleared}
      />
      <AdvanceStatusBar
        storeId={storeId}
        orderId={orderId}
        shipment={shipment}
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
  isStub,
  onUpdated,
  onCleared,
}: {
  storeId: string;
  orderId: string;
  shipment: ShipmentResponse;
  labelProxyURL: string;
  isStub: boolean;
  onUpdated: (s: ShipmentResponse) => void;
  onCleared: () => void;
}) {
  const [showEmailForm, setShowEmailForm] = useState(false);
  const [recipient, setRecipient] = useState("");
  const [emailPending, emailStartTransition] = useTransition();
  const [emailMsg, setEmailMsg] = useState<
    { kind: "ok" | "err"; text: string } | null
  >(null);
  const [refreshPending, refreshStartTransition] = useTransition();
  const [refreshMsg, setRefreshMsg] = useState<string | null>(null);
  const [downloadPending, setDownloadPending] = useState(false);
  const [downloadErr, setDownloadErr] = useState<string | null>(null);
  const [deletePending, deleteStartTransition] = useTransition();
  const [deleteErr, setDeleteErr] = useState<string | null>(null);
  // Pickup reschedule popover state. Kept local to LabelActions so the
  // rest of the panel doesn't need to know the slot catalogue.
  const [showReschedule, setShowReschedule] = useState(false);
  const [rescheduleDate, setRescheduleDate] = useState(defaultRescheduleDate);
  const [rescheduleSlot, setRescheduleSlot] = useState("14:00:00");
  const [reschedulePending, rescheduleStartTransition] = useTransition();
  const [rescheduleMsg, setRescheduleMsg] = useState<
    { kind: "ok" | "err"; text: string } | null
  >(null);

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

  // Download via fetch so we can inspect the Content-Type. If the
  // carrier returned a real PDF we stream it through a Blob URL and
  // trigger a download. If the proxy returned a JSON error (wrong
  // token, unserviceable pincode, warehouse not registered) we surface
  // the message inline instead of letting the browser save "label.txt".
  const download = useCallback(async () => {
    setDownloadErr(null);
    setDownloadPending(true);
    try {
      const resp = await fetch(labelProxyURL, { cache: "no-store" });
      const contentType = resp.headers.get("content-type") ?? "";
      if (!resp.ok || !contentType.toLowerCase().includes("pdf")) {
        const body = await resp.text().catch(() => "");
        let msg = `Download failed (status ${resp.status}).`;
        try {
          const parsed = JSON.parse(body) as { message?: string };
          if (parsed?.message) msg = parsed.message;
        } catch {
          if (body) msg = body.slice(0, 400);
        }
        setDownloadErr(msg);
        return;
      }
      const blob = await resp.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `shipping-label-${shipment.tracking_number || shipment.id}.pdf`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e) {
      setDownloadErr(
        e instanceof Error ? e.message : "Download failed — network error.",
      );
    } finally {
      setDownloadPending(false);
    }
  }, [labelProxyURL, shipment.id, shipment.tracking_number]);

  const reschedulePickup = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      setRescheduleMsg(null);
      rescheduleStartTransition(async () => {
        const r = await schedulePickupAction(storeId, orderId, shipment.id, {
          date: rescheduleDate || undefined,
          slot_start: rescheduleSlot || undefined,
        });
        if (!r.ok) {
          setRescheduleMsg({
            kind: "err",
            text: r.error?.message ?? "Reschedule failed.",
          });
          return;
        }
        if (r.data) {
          onUpdated(r.data);
          setRescheduleMsg({ kind: "ok", text: "Pickup rescheduled." });
          setShowReschedule(false);
        }
      });
    },
    [storeId, orderId, shipment.id, rescheduleDate, rescheduleSlot, onUpdated],
  );

  const clearShipment = useCallback(() => {
    setDeleteErr(null);
    deleteStartTransition(async () => {
      const r = await deleteShipmentAction(storeId, orderId, shipment.id);
      if (!r.ok) {
        setDeleteErr(r.error.message);
        return;
      }
      onCleared();
    });
  }, [storeId, orderId, shipment.id, onCleared]);

  return (
    <div className="flex flex-col gap-3 pt-1">
      <div className="flex flex-wrap items-center gap-1.5">
        <button
          type="button"
          onClick={download}
          disabled={downloadPending || isStub}
          title={
            isStub
              ? "Mock tracking number — delete and recreate before downloading"
              : undefined
          }
          className="inline-flex h-8 items-center rounded-md bg-[color:var(--ink-900)] px-3 text-[13px] font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
        >
          {downloadPending ? "Fetching…" : "Download label"}
        </button>
        <GhostBtn
          onClick={() => {
            setShowEmailForm((v) => !v);
            setEmailMsg(null);
          }}
          disabled={isStub}
          active={showEmailForm}
        >
          {showEmailForm ? "Cancel email" : "Email label"}
        </GhostBtn>
        <GhostBtn onClick={refresh} disabled={refreshPending || isStub}>
          {refreshPending ? "Syncing…" : "Refresh tracking"}
        </GhostBtn>
        <GhostBtn
          onClick={() => {
            setShowReschedule((v) => !v);
            setRescheduleMsg(null);
          }}
          disabled={isStub || reschedulePending}
          active={showReschedule}
        >
          {showReschedule ? "Cancel" : "Reschedule pickup"}
        </GhostBtn>
        <button
          type="button"
          onClick={clearShipment}
          disabled={deletePending}
          className="inline-flex h-8 items-center rounded-md border border-[color:var(--ink-900)]/15 px-3 text-[13px] font-medium text-foreground-secondary transition-colors hover:border-[color:var(--danger)] hover:text-[color:var(--danger)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
        >
          {deletePending ? "Deleting…" : "Delete"}
        </button>
      </div>
      {downloadErr && (
        <p role="alert" className="rounded-md border border-[color:var(--danger)]/20 bg-[color:var(--danger)]/[0.05] px-3 py-2 text-xs text-[color:var(--danger)]">
          {downloadErr}
        </p>
      )}
      {deleteErr && (
        <p role="alert" className="rounded-md border border-[color:var(--danger)]/20 bg-[color:var(--danger)]/[0.05] px-3 py-2 text-xs text-[color:var(--danger)]">
          {deleteErr}
        </p>
      )}
      {refreshMsg && (
        <p className="text-xs text-foreground-tertiary">{refreshMsg}</p>
      )}
      {showReschedule && (
        <form
          onSubmit={reschedulePickup}
          className="flex flex-col gap-2 rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-3"
        >
          <div className="grid gap-2 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
              Pickup date
              <input
                type="date"
                value={rescheduleDate}
                onChange={(e) => setRescheduleDate(e.target.value)}
                className="rounded-md border border-[color:var(--ink-900)]/30 px-3 py-2 text-sm normal-case tracking-normal text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
              Slot
              <select
                value={rescheduleSlot}
                onChange={(e) => setRescheduleSlot(e.target.value)}
                className="rounded-md border border-[color:var(--ink-900)]/30 px-3 py-2 text-sm normal-case tracking-normal text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none"
              >
                <option value="10:00:00">Morning (10–14)</option>
                <option value="14:00:00">Afternoon (14–18)</option>
                <option value="16:00:00">Evening (16–20)</option>
              </select>
            </label>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={reschedulePending}
              className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {reschedulePending ? "Scheduling…" : "Schedule pickup"}
            </button>
          </div>
          {rescheduleMsg && (
            <p
              role={rescheduleMsg.kind === "err" ? "alert" : "status"}
              className={`text-xs ${
                rescheduleMsg.kind === "ok"
                  ? "text-[color:var(--moss-700)]"
                  : "text-red-700"
              }`}
            >
              {rescheduleMsg.text}
            </p>
          )}
        </form>
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
    <div className="flex flex-col gap-2 border-t border-border-subtle pt-3">
      <span className="text-[11px] font-semibold uppercase tracking-[0.14em] text-foreground-tertiary">
        Advance status
      </span>
      <div className="flex flex-wrap items-center gap-1.5">
        {steps.map((s) => (
          <button
            key={s.status}
            type="button"
            onClick={() => advance(s.status)}
            disabled={disabledFor(s.status)}
            className="inline-flex h-8 items-center rounded-md border border-[color:var(--ink-900)]/20 px-3 text-[13px] font-medium text-foreground transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
          >
            {target === s.status ? "Updating…" : s.label}
          </button>
        ))}
      </div>
    </div>
  );
}

// formatPickupDisplay turns the shipment's pickup_scheduled_for
// (RFC3339 UTC) into the admin-panel "Pickup: Fri, Apr 21, 14:00"
// copy. Rendered in the caller's locale so an Indian warehouse
// operator sees IST even though the column is UTC in the DB.
function formatPickupDisplay(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const day = d.toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
  const time = d.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
  return `${day}, ${time}`;
}

// defaultRescheduleDate pre-fills the reschedule input with
// "today + 1 day" so the merchant doesn't need to type a date for
// the common "bump by one day" case. Format must match <input
// type="date">: YYYY-MM-DD in local time.
function defaultRescheduleDate(): string {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function DetailRow({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <>
      <dt className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
        {label}
      </dt>
      <dd
        className={`min-w-0 text-sm text-foreground ${mono ? "font-mono tabular-nums" : ""}`}
      >
        {value || "—"}
      </dd>
    </>
  );
}

function GhostBtn({
  onClick,
  disabled,
  active,
  children,
}: {
  onClick: () => void;
  disabled?: boolean;
  active?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={
        "inline-flex h-8 items-center rounded-md border px-3 text-[13px] font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50 " +
        (active
          ? "border-[color:var(--moss-700)] text-[color:var(--moss-700)]"
          : "border-[color:var(--ink-900)]/20 text-foreground hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)]")
      }
    >
      {children}
    </button>
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
