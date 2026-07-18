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

import { useToast } from "@/components/feedback/Toaster";
import type { ShipmentResponse } from "@/lib/api/shipping-api";
import { shipmentCancellationCopy } from "@/lib/copy/shipmentCancellation";

import {
  createShipmentAction,
  getShipmentAction,
  updateShipmentStatusAction,
  emailShipmentLabelAction,
  refreshShipmentTrackingAction,
  deleteShipmentAction,
  schedulePickupAction,
  cancelShipmentAction,
  type ShippingActionResult,
} from "@/app/(admin)/orders/[id]/shipping-actions";

// ─────────────────────────────────────────────────────────────────────────
// Props
// ─────────────────────────────────────────────────────────────────────────

interface ShippingLabelPanelProps {
  storeId: string;
  orderId: string;
  orderStatus: string;
  /**
   * The carrier + service the customer picked at checkout (resolved from
   * the store's configured carrier list and persisted on the order). Used
   * as the form default so an AU store on ShipEngine doesn't pre-fill
   * Delhivery. Undefined for orders pre-dating migration 82.
   */
  customerCarrier?: string;
  customerService?: string;
}

// ─────────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────────

const ACTIONABLE_STATUSES = new Set(["pending", "confirmed", "fulfilled"]);

export function ShippingLabelPanel({
  storeId,
  orderId,
  orderStatus,
  customerCarrier,
  customerService,
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
        <p className="text-sm text-foreground-tertiary">
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
          customerCarrier={customerCarrier}
          customerService={customerService}
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

  // Cancel/return status. "succeeded" and "requested" are quiet
  // confirmations (DetailRow); "failed" and "unsupported" need the
  // merchant's attention (warning banner) — same split as isStub above.
  const cancelNeedsAttention =
    shipment.cancel_status === "failed" || shipment.cancel_status === "unsupported";
  const cancelIsQuiet =
    shipment.cancel_status === "succeeded" || shipment.cancel_status === "requested";

  const labelProxyURL = `/api/admin/stores/${storeId}/orders/${orderId}/shipments/${shipment.id}/label`;

  return (
    <div className="flex flex-col gap-5">
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
        {cancelIsQuiet && (
          <DetailRow
            label={shipmentCancellationCopy.detailRowLabel}
            value={shipmentCancellationCopy.statusRowValue(
              shipment.cancel_action,
              shipment.cancel_status,
            )}
          />
        )}
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
      {cancelNeedsAttention && (
        <div
          role="status"
          className="rounded-md border border-[color:var(--warning)]/30 bg-[color:var(--warning)]/[0.06] px-4 py-3 text-xs text-[color:var(--warning)]"
        >
          {shipmentCancellationCopy.statusWarningMessage({
            action: shipment.cancel_action,
            status: shipment.cancel_status,
            reason: shipment.cancel_reason,
            trackingNumber: shipment.tracking_number,
          })}
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
  const { toast } = useToast();
  const [showEmailForm, setShowEmailForm] = useState(false);
  const [recipient, setRecipient] = useState("");
  const [emailPending, emailStartTransition] = useTransition();
  const [refreshPending, refreshStartTransition] = useTransition();
  const [downloadPending, setDownloadPending] = useState(false);
  const [downloadErr, setDownloadErr] = useState<string | null>(null);
  const [deletePending, deleteStartTransition] = useTransition();
  const [cancelShipmentPending, cancelShipmentStartTransition] = useTransition();
  // Pickup reschedule popover state. Kept local to LabelActions so the
  // rest of the panel doesn't need to know the slot catalogue.
  const [showReschedule, setShowReschedule] = useState(false);
  const [rescheduleDate, setRescheduleDate] = useState(defaultRescheduleDate);
  const [rescheduleSlot, setRescheduleSlot] = useState("14:00:00");
  const [reschedulePending, rescheduleStartTransition] = useTransition();

  const sendEmail = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      emailStartTransition(async () => {
        const r = await emailShipmentLabelAction(
          storeId,
          orderId,
          shipment.id,
          recipient,
        );
        if (!r.ok) {
          toast.error("Couldn't email label", r.error.message);
          return;
        }
        toast.success("Label sent", `Delivered to ${recipient}.`);
        setRecipient("");
        setShowEmailForm(false);
      });
    },
    [storeId, orderId, shipment.id, recipient, toast],
  );

  const refresh = useCallback(() => {
    refreshStartTransition(async () => {
      const r = await refreshShipmentTrackingAction(storeId, orderId, shipment.id);
      if (!r.ok) {
        toast.error("Tracking refresh failed", r.error?.message ?? "Please try again.");
        return;
      }
      if (r.data) {
        onUpdated(r.data);
        toast.success("Tracking synced");
      }
    });
  }, [storeId, orderId, shipment.id, onUpdated, toast]);

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
      rescheduleStartTransition(async () => {
        const r = await schedulePickupAction(storeId, orderId, shipment.id, {
          date: rescheduleDate || undefined,
          slot_start: rescheduleSlot || undefined,
        });
        if (!r.ok) {
          toast.error("Reschedule failed", r.error?.message ?? "Please try again.");
          return;
        }
        if (r.data) {
          onUpdated(r.data);
          toast.success("Pickup rescheduled");
          setShowReschedule(false);
        }
      });
    },
    [storeId, orderId, shipment.id, rescheduleDate, rescheduleSlot, onUpdated, toast],
  );

  // Manual "Cancel / return shipment" — the merchant-triggered
  // counterpart to the backend's auto-cancel-on-refund/cancel behavior.
  // The cancel endpoint returns only the outcome (action/status/reason),
  // so we refetch the shipment afterwards to pick up the new
  // cancel_action/cancel_status/cancel_reason fields for display.
  const cancelReturnShipment = useCallback(() => {
    cancelShipmentStartTransition(async () => {
      const r = await cancelShipmentAction(storeId, orderId, shipment.id);
      if (!r.ok) {
        toast.error(
          shipmentCancellationCopy.toastErrorTitle,
          r.error?.message ?? "Please try again.",
        );
        return;
      }
      toast.success(shipmentCancellationCopy.toastSuccessTitle);
      const fresh = await getShipmentAction(storeId, orderId);
      if (fresh) {
        onUpdated(fresh);
      }
    });
  }, [storeId, orderId, shipment.id, onUpdated, toast]);

  const clearShipment = useCallback(() => {
    deleteStartTransition(async () => {
      const r = await deleteShipmentAction(storeId, orderId, shipment.id);
      if (!r.ok) {
        toast.error("Couldn't delete shipment", r.error.message);
        return;
      }
      toast.success("Shipment deleted");
      onCleared();
    });
  }, [storeId, orderId, shipment.id, onCleared, toast]);

  return (
    <div className="flex flex-col gap-3 pt-1">
      {/* Tier 1 — primary actions. Download is the action a merchant
          reaches for most often, so it takes the ink-filled weight;
          Email is its close sibling in a ghost treatment. */}
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
          onClick={() => setShowEmailForm((v) => !v)}
          disabled={isStub}
          active={showEmailForm}
        >
          {showEmailForm ? "Cancel email" : "Email label"}
        </GhostBtn>
      </div>

      {/* Tier 2 — utility actions. Text-only links with bullet separators
          so they visually recede behind the primary CTAs without needing
          their own bordered buttons fighting for attention. */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-foreground-secondary">
        <TextLink onClick={refresh} disabled={refreshPending || isStub}>
          {refreshPending ? "Syncing tracking…" : "Refresh tracking"}
        </TextLink>
        <span aria-hidden="true" className="text-foreground-tertiary">·</span>
        <TextLink
          onClick={cancelReturnShipment}
          disabled={cancelShipmentPending || isStub}
          tone={shipment.cancel_status === "failed" ? "danger" : "default"}
        >
          {cancelShipmentPending
            ? shipmentCancellationCopy.manualActionPending
            : shipmentCancellationCopy.manualActionLabel}
        </TextLink>
        <span aria-hidden="true" className="text-foreground-tertiary">·</span>
        <TextLink
          onClick={() => setShowReschedule((v) => !v)}
          disabled={isStub || reschedulePending}
          active={showReschedule}
        >
          {showReschedule ? "Cancel reschedule" : "Reschedule pickup"}
        </TextLink>
        <span aria-hidden="true" className="text-foreground-tertiary">·</span>
        <TextLink
          onClick={clearShipment}
          disabled={deletePending}
          tone="danger"
        >
          {deletePending ? "Deleting…" : "Delete shipment"}
        </TextLink>
      </div>
      {downloadErr && (
        <p role="alert" className="text-xs text-[color:var(--danger)]">
          {downloadErr}
        </p>
      )}
      {showReschedule && (
        <form
          onSubmit={reschedulePickup}
          className="flex flex-col gap-2 border-t border-border-subtle pt-3"
        >
          <div className="grid gap-2 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-xs uppercase tracking-wider text-foreground-tertiary">
              Pickup date
              <input
                type="date"
                value={rescheduleDate}
                onChange={(e) => setRescheduleDate(e.target.value)}
                className="rounded-md border border-[color:var(--ink-900)]/30 px-3 py-2 text-sm normal-case tracking-normal text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none"
              />
            </label>
            <div className="flex flex-col gap-1 text-xs uppercase tracking-wider text-foreground-tertiary">
              <span id="reschedule-slot-label">Slot</span>
              <Select
                value={rescheduleSlot}
                onValueChange={(next) => setRescheduleSlot(next)}
              >
                <SelectTrigger
                  aria-labelledby="reschedule-slot-label"
                  className="normal-case tracking-normal"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="10:00:00">Morning (10–14)</SelectItem>
                  <SelectItem value="14:00:00">Afternoon (14–18)</SelectItem>
                  <SelectItem value="16:00:00">Evening (16–20)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={reschedulePending}
              className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-800)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {reschedulePending ? "Scheduling…" : "Schedule pickup"}
            </button>
          </div>
        </form>
      )}
      {showEmailForm && (
        <form onSubmit={sendEmail} className="flex flex-col gap-2 pt-2">
          <label className="flex flex-col gap-1 text-xs uppercase tracking-wider text-foreground-tertiary">
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
              className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-800)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {emailPending ? "Sending…" : "Send label"}
            </button>
          </div>
        </form>
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
  const { toast } = useToast();
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
    const labelMap: Record<string, string> = {
      in_transit: "In transit",
      out_for_delivery: "Out for delivery",
      delivered: "Delivered",
    };
    startTransition(async () => {
      const r = await updateShipmentStatusAction(storeId, orderId, shipment.id, { status });
      setTarget(null);
      if (r.ok && r.data) {
        onUpdated(r.data);
        toast.success("Shipment updated", `Marked as ${labelMap[status] ?? status}.`);
      } else if (!r.ok) {
        toast.error("Couldn't update shipment", r.error?.message ?? "Please try again.");
      }
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

function TextLink({
  onClick,
  disabled,
  active,
  tone = "default",
  children,
}: {
  onClick: () => void;
  disabled?: boolean;
  active?: boolean;
  tone?: "default" | "danger";
  children: React.ReactNode;
}) {
  const hoverClass =
    tone === "danger"
      ? "hover:text-[color:var(--danger)]"
      : "hover:text-[color:var(--moss-700)]";
  const activeClass = active ? "text-[color:var(--moss-700)]" : "";
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`text-xs underline-offset-4 transition-colors hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40 ${hoverClass} ${activeClass}`}
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
  customerCarrier?: string;
  customerService?: string;
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

function isKnownCarrier(value: string): boolean {
  return CARRIERS.some((c) => c.value === value);
}

function isKnownService(value: string): boolean {
  return SERVICE_LEVELS.some((s) => s.value === value);
}

function CreateShipmentForm({
  storeId,
  orderId,
  customerCarrier,
  customerService,
  onCreated,
  onCancel,
}: CreateShipmentFormProps) {
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<ShippingActionResult["error"] | undefined>();
  // Default to whatever the customer picked at checkout (persisted on the
  // order). The carrier value is one of CARRIERS so we can sanity-check
  // it; the service is a carrier-specific code like
  // "auspost_parcel_post_australia" or "usps_priority_mail" — we MUST
  // pass it through unchanged because that's what ShipEngine's /v1/labels
  // expects. Filtering it through SERVICE_LEVELS (only standard|express)
  // collapsed every real code to "standard", which ShipEngine rejects
  // with `field_value_required: 'service_code' must not be empty`.
  const initialProvider =
    customerCarrier && isKnownCarrier(customerCarrier) ? customerCarrier : "shipengine";
  const initialService = customerService || "standard";
  const [provider, setProvider] = useState(initialProvider);
  const [service, setService] = useState(initialService);
  const [showOverride, setShowOverride] = useState(false);
  const haveCustomerChoice = Boolean(customerCarrier);

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
          toast.error("Couldn't generate label", r.error?.message ?? "Please try again.");
          return;
        }
        if (r.data) {
          toast.success("Shipping label generated");
          onCreated(r.data);
        }
      });
    },
    [storeId, orderId, provider, service, onCreated, toast],
  );

  return (
    <div className="flex flex-col gap-4 border-t border-border-subtle pt-4">
      <div className="flex flex-col gap-1">
        <h3 className="text-xs font-semibold uppercase tracking-[0.12em] text-foreground-tertiary">
          Approve &amp; generate label
        </h3>
        <p className="text-xs text-foreground-tertiary">
          {haveCustomerChoice ? "Picked by customer" : "Default"}:{" "}
          {CARRIERS.find((c) => c.value === provider)?.label ?? provider}
          {" · "}
          {SERVICE_LEVELS.find((s) => s.value === service)?.label ?? service}
        </p>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-4">
        {showOverride && (
          <>
            <div className="flex flex-col gap-2">
              <span className="text-xs uppercase tracking-wider text-foreground-tertiary">
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
              <span className="text-xs uppercase tracking-wider text-foreground-tertiary">
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
          <p role="alert" className="rounded-md border border-[color:var(--signal)]/30 bg-[color:var(--signal)]/[0.06] px-3 py-2 text-sm text-[color:var(--signal)]">
            {error.message}
          </p>
        )}
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={pending || !provider || !service}
            className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-800)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
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
            className="text-sm text-foreground-secondary hover:opacity-100 hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}
