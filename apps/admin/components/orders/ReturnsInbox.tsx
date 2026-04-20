"use client";

// Admin returns inbox — two-pane editorial layout. The list sits on the
// page surface (no outer card), divided from the detail pane by a single
// vertical hairline. Tabs are underlined, not pilled. Row selection is a
// left-ink accent rather than a fill. When a request is selected, the
// detail pane opens with an editorial masthead (serif return number,
// link to parent order) and a lifecycle stepper.
//
// All action feedback is delivered via the shared toast system — no more
// inline success banners or warning cards.

import { useCallback, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";

import { useToast } from "@/components/feedback/Toaster";
import { formatMoney, timeAgo, formatDate } from "@/lib/format";
import type { AdminReturn, AdminReturnStatus } from "@/lib/api/marketplace-api";

import { ReturnLifecycleStepper } from "./ReturnLifecycleStepper";

type Tab = "pending" | "approved" | "received" | "refunded" | "rejected";

const TABS: { key: Tab; label: string; match: AdminReturnStatus[] }[] = [
  { key: "pending", label: "Pending", match: ["requested"] },
  { key: "approved", label: "Approved", match: ["approved"] },
  { key: "received", label: "Received", match: ["received"] },
  { key: "refunded", label: "Refunded", match: ["refunded"] },
  { key: "rejected", label: "Rejected", match: ["rejected"] },
];

interface Props {
  storeId: string;
  initialReturns: AdminReturn[];
}

export function ReturnsInbox({ storeId, initialReturns }: Props) {
  const router = useRouter();
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("pending");
  const [returns, setReturns] = useState<AdminReturn[]>(initialReturns);
  const [selectedId, setSelectedId] = useState<string | null>(
    initialReturns.find((r) => r.status === "requested")?.id ?? null,
  );
  const [busy, setBusy] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [pickupDetails, setPickupDetails] = useState("");
  const [rejectReason, setRejectReason] = useState("");
  const prevSelectedRef = useRef<string | null>(null);

  const visible = useMemo(() => {
    const statuses = TABS.find((t) => t.key === tab)?.match ?? [];
    return returns.filter((r) => statuses.includes(r.status));
  }, [tab, returns]);

  const selected = useMemo(
    () => returns.find((r) => r.id === selectedId) ?? null,
    [returns, selectedId],
  );

  // Reset the form when the selected row changes so we don't leak the
  // previous return's pickup text into a different request.
  if (prevSelectedRef.current !== selectedId) {
    prevSelectedRef.current = selectedId;
    setPickupDetails(selected?.pickup_details ?? "");
    setRejectReason("");
    setValidationError(null);
  }

  const refresh = useCallback(async () => {
    try {
      const res = await fetch(
        `/api/admin/stores/${encodeURIComponent(storeId)}/returns`,
        { credentials: "include", cache: "no-store" },
      );
      if (res.ok) {
        const body = (await res.json()) as { data?: AdminReturn[] };
        setReturns(body.data ?? []);
      }
    } catch {
      /* silent — refresh is opportunistic */
    }
  }, [storeId]);

  async function handleApprove() {
    if (!selected || busy) return;
    setBusy(true);
    setValidationError(null);
    try {
      const res = await fetch(
        `/api/admin/stores/${encodeURIComponent(storeId)}/returns/${encodeURIComponent(selected.id)}/approve`,
        {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ pickup_details: pickupDetails.trim() }),
        },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { message?: string };
        toast.error("Approve failed", body.message || `Status ${res.status}`);
        return;
      }
      const updated = (await res.json()) as AdminReturn;
      setReturns((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      setTab("approved");
      toast.success(
        selected.type === "replace" ? "Replacement approved" : "Return approved",
        "Customer has been notified.",
      );
      router.refresh();
    } catch (err) {
      toast.error("Approve failed", (err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleReject() {
    if (!selected || busy) return;
    if (!rejectReason.trim()) {
      setValidationError("A reason is required to reject.");
      return;
    }
    setBusy(true);
    setValidationError(null);
    try {
      const res = await fetch(
        `/api/admin/stores/${encodeURIComponent(storeId)}/returns/${encodeURIComponent(selected.id)}/reject`,
        {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reason: rejectReason.trim() }),
        },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { message?: string };
        toast.error("Reject failed", body.message || `Status ${res.status}`);
        return;
      }
      const updated = (await res.json()) as AdminReturn;
      setReturns((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      setTab("rejected");
      toast.success("Return rejected", "Customer has been notified.");
      router.refresh();
    } catch (err) {
      toast.error("Reject failed", (err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdatePickup() {
    if (!selected || busy) return;
    setBusy(true);
    setValidationError(null);
    try {
      const res = await fetch(
        `/api/admin/stores/${encodeURIComponent(storeId)}/returns/${encodeURIComponent(selected.id)}/pickup`,
        {
          method: "PATCH",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ pickup_details: pickupDetails.trim() }),
        },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { message?: string };
        toast.error("Update failed", body.message || `Status ${res.status}`);
        return;
      }
      const updated = (await res.json()) as AdminReturn;
      setReturns((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      toast.success("Pickup details updated");
      router.refresh();
    } catch (err) {
      toast.error("Update failed", (err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const fieldClass =
    "w-full resize-y rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary transition-colors focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]";

  return (
    <div className="grid min-h-[640px] grid-cols-1 border-y border-border-subtle lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)]">
      <aside className="flex flex-col border-b border-border-subtle lg:border-b-0 lg:border-r">
        <TabBar tab={tab} setTab={setTab} returns={returns} />
        <div className="flex-1 overflow-y-auto">
          {visible.length === 0 ? (
            <p className="px-6 py-10 text-sm text-foreground-tertiary">
              No returns in this bucket yet.
            </p>
          ) : (
            <ul role="list" className="flex flex-col">
              {visible.map((r) => (
                <li key={r.id}>
                  <ReturnRow
                    rma={r}
                    selected={selectedId === r.id}
                    onSelect={() => {
                      setSelectedId(r.id);
                      refresh().catch(() => {});
                    }}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      </aside>

      <section className="flex flex-col overflow-hidden">
        {!selected ? (
          <EmptyState />
        ) : (
          <div className="flex flex-col gap-8 p-6 lg:p-8">
            <ReturnMasthead rma={selected} storeId={storeId} />
            <ReturnLifecycleStepper rma={selected} />
            <ItemsList rma={selected} />

            {selected.status === "requested" && (
              <PendingActions
                pickupDetails={pickupDetails}
                setPickupDetails={setPickupDetails}
                rejectReason={rejectReason}
                setRejectReason={setRejectReason}
                validationError={validationError}
                type={selected.type}
                busy={busy}
                fieldClass={fieldClass}
                onApprove={handleApprove}
                onReject={handleReject}
              />
            )}

            {selected.status === "approved" && (
              <ApprovedActions
                pickupDetails={pickupDetails}
                setPickupDetails={setPickupDetails}
                busy={busy}
                fieldClass={fieldClass}
                onUpdate={handleUpdatePickup}
              />
            )}

            {selected.status === "rejected" && selected.reject_reason && (
              <RejectedBlock reason={selected.reject_reason} />
            )}
          </div>
        )}
      </section>
    </div>
  );
}

// ─── Tabs ─────────────────────────────────────────────────────────────

function TabBar({
  tab,
  setTab,
  returns,
}: {
  tab: Tab;
  setTab: (t: Tab) => void;
  returns: AdminReturn[];
}) {
  return (
    <nav
      aria-label="Return status filter"
      className="flex items-center gap-0 border-b border-border-subtle px-2"
    >
      {TABS.map((t) => {
        const count = returns.filter((r) => t.match.includes(r.status)).length;
        const active = tab === t.key;
        return (
          <button
            key={t.key}
            type="button"
            onClick={() => setTab(t.key)}
            className={
              "-mb-px flex items-baseline gap-1.5 border-b-2 px-2.5 py-3 text-[11px] font-semibold uppercase tracking-[0.12em] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
              (active
                ? "border-[color:var(--ink-900)] text-foreground"
                : "border-transparent text-foreground-tertiary hover:text-foreground")
            }
          >
            <span>{t.label}</span>
            {count > 0 && (
              <span
                className={
                  "text-[11px] tabular-nums " +
                  (active ? "text-foreground-secondary" : "text-foreground-tertiary")
                }
              >
                {count}
              </span>
            )}
          </button>
        );
      })}
    </nav>
  );
}

// ─── List row ─────────────────────────────────────────────────────────

function ReturnRow({
  rma,
  selected,
  onSelect,
}: {
  rma: AdminReturn;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      className={
        "relative w-full border-b border-border-subtle px-5 py-4 pl-[18px] text-left transition-colors focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)] " +
        (selected
          ? "before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-[color:var(--ink-900)]"
          : "hover:bg-[color:var(--ink-900)]/[0.02]")
      }
    >
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-serif text-sm tabular-nums text-foreground">
          {rma.return_number}
        </span>
        <span className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
          {rma.type === "replace" ? "Replace" : "Return"}
        </span>
      </div>
      <p className="mt-1.5 truncate text-sm text-foreground-secondary">
        {rma.reason ?? "No reason given"}
      </p>
      <p className="mt-1 text-[11px] tabular-nums text-foreground-tertiary">
        {timeAgo(rma.requested_at)}
      </p>
    </button>
  );
}

// ─── Empty state ──────────────────────────────────────────────────────

function EmptyState() {
  return (
    <div className="flex h-full items-center justify-center p-10">
      <div className="max-w-xs text-sm text-foreground-tertiary">
        Pick a request on the left to approve, reject, or edit pickup details.
      </div>
    </div>
  );
}

// ─── Masthead ─────────────────────────────────────────────────────────

function ReturnMasthead({ rma, storeId: _storeId }: { rma: AdminReturn; storeId: string }) {
  const itemCount = rma.items.reduce((sum, it) => sum + it.quantity, 0);
  const hasRefund =
    rma.status === "refunded" &&
    rma.refund_amount &&
    Number.parseFloat(rma.refund_amount) > 0;

  return (
    <header className="flex flex-col gap-3">
      <p className="eyebrow">
        {rma.type === "replace" ? "Replacement request" : "Return request"}
      </p>
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h2
          id="return-heading"
          className="font-serif text-3xl font-medium tracking-tight text-foreground tabular-nums"
        >
          {rma.return_number}
        </h2>
        {hasRefund && (
          <>
            <span
              aria-hidden="true"
              className="font-serif text-2xl font-light text-foreground-tertiary"
            >
              —
            </span>
            <span
              aria-label={`Refunded ${rma.refund_amount} ${rma.currency_code}`}
              className="font-serif text-2xl font-light text-foreground-secondary tabular-nums"
            >
              {formatMoney(rma.refund_amount, rma.currency_code)}
            </span>
          </>
        )}
      </div>
      <p className="flex flex-wrap items-center gap-x-5 gap-y-1 text-sm text-foreground-secondary">
        <span>Requested {formatDate(rma.requested_at)}</span>
        <span aria-hidden="true" className="text-foreground-tertiary">
          ·
        </span>
        <Link
          href={`/orders/${rma.order_id}`}
          className="text-[color:var(--moss-700)] underline-offset-4 hover:underline"
        >
          View parent order
        </Link>
        <span aria-hidden="true" className="text-foreground-tertiary">
          ·
        </span>
        <span>
          {itemCount} {itemCount === 1 ? "item" : "items"}
        </span>
      </p>
      {rma.notes && (
        <p className="mt-1 whitespace-pre-wrap border-l-2 border-border-subtle pl-4 font-serif text-sm italic text-foreground-secondary">
          {rma.notes}
        </p>
      )}
    </header>
  );
}

// ─── Items list ───────────────────────────────────────────────────────

function ItemsList({ rma }: { rma: AdminReturn }) {
  return (
    <section aria-labelledby="items-heading" className="flex flex-col gap-3">
      <h3
        id="items-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Items requested
      </h3>
      <ul role="list" className="flex flex-col">
        {rma.items.map((it) => (
          <li
            key={it.id}
            className="flex items-baseline gap-4 border-b border-border-subtle py-3 last:border-b-0"
          >
            <span className="font-serif text-base tabular-nums text-foreground">
              {it.quantity}×
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate font-mono text-xs text-foreground-secondary">
                {it.order_item_id}
              </span>
              {it.reason && (
                <span className="mt-0.5 block text-sm text-foreground-tertiary">
                  {it.reason}
                </span>
              )}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

// ─── Pending actions (approve / reject) ───────────────────────────────

function PendingActions({
  pickupDetails,
  setPickupDetails,
  rejectReason,
  setRejectReason,
  validationError,
  type,
  busy,
  fieldClass,
  onApprove,
  onReject,
}: {
  pickupDetails: string;
  setPickupDetails: (v: string) => void;
  rejectReason: string;
  setRejectReason: (v: string) => void;
  validationError: string | null;
  type: "return" | "replace";
  busy: boolean;
  fieldClass: string;
  onApprove: () => void;
  onReject: () => void;
}) {
  return (
    <div className="flex flex-col gap-8 border-t border-border-subtle pt-6">
      <div className="flex flex-col gap-3">
        <label
          htmlFor="pickup"
          className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
        >
          Pickup / logistics
          <span className="text-[11px] font-normal normal-case tracking-normal text-foreground-tertiary">
            — customer sees this
          </span>
        </label>
        <textarea
          id="pickup"
          value={pickupDetails}
          onChange={(e) => setPickupDetails(e.target.value)}
          rows={3}
          placeholder={
            type === "replace"
              ? "e.g. Replacement ships via Delhivery on Monday, tracking added shortly."
              : "e.g. Delhivery will collect on Monday 10am–2pm, no packing required."
          }
          className={fieldClass}
        />
        <button
          type="button"
          onClick={onApprove}
          disabled={busy}
          className="inline-flex w-fit items-center rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {busy ? "Working…" : type === "replace" ? "Approve replacement" : "Approve return"}
        </button>
      </div>

      <div className="flex flex-col gap-3 border-t border-border-subtle pt-6">
        <label
          htmlFor="reject"
          className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
        >
          Reject with reason
        </label>
        <input
          id="reject"
          type="text"
          value={rejectReason}
          onChange={(e) => setRejectReason(e.target.value)}
          placeholder="Reason shown to the customer"
          className={fieldClass}
        />
        {validationError && (
          <p role="alert" className="text-xs text-[color:var(--danger)]">
            {validationError}
          </p>
        )}
        <button
          type="button"
          onClick={onReject}
          disabled={busy}
          className="inline-flex w-fit items-center rounded-md border border-[color:var(--danger)] px-4 py-2 text-sm font-medium text-[color:var(--danger)] transition-colors hover:bg-[color:var(--danger)]/[0.06] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          Reject
        </button>
      </div>
    </div>
  );
}

// ─── Approved actions (update pickup) ─────────────────────────────────

function ApprovedActions({
  pickupDetails,
  setPickupDetails,
  busy,
  fieldClass,
  onUpdate,
}: {
  pickupDetails: string;
  setPickupDetails: (v: string) => void;
  busy: boolean;
  fieldClass: string;
  onUpdate: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 border-t border-border-subtle pt-6">
      <label
        htmlFor="pickup-edit"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Update pickup / logistics
      </label>
      <textarea
        id="pickup-edit"
        value={pickupDetails}
        onChange={(e) => setPickupDetails(e.target.value)}
        rows={3}
        className={fieldClass}
      />
      <button
        type="button"
        onClick={onUpdate}
        disabled={busy}
        className="inline-flex w-fit items-center rounded-md border border-[color:var(--ink-900)]/40 px-4 py-2 text-sm font-medium text-foreground transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
      >
        {busy ? "Saving…" : "Save pickup details"}
      </button>
    </div>
  );
}

// ─── Rejected block ──────────────────────────────────────────────────

function RejectedBlock({ reason }: { reason: string }) {
  return (
    <div className="flex flex-col gap-2 border-t border-border-subtle pt-6">
      <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-[color:var(--danger)]">
        Rejected reason
      </h3>
      <p className="text-sm text-foreground">{reason}</p>
    </div>
  );
}
