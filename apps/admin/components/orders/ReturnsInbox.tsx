"use client";

// Returns inbox — two-pane layout. Pending tab is the landing tab (that's
// where new requests arrive from customers). Clicking a row opens the
// detail drawer on the right with the approve / reject / pickup-edit
// actions. Everything calls the same-origin /api/admin/stores/[storeId]
// proxy so the admin middleware injects the session headers.

import { useCallback, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { AdminReturn, AdminReturnStatus } from "@/lib/api/marketplace-api";

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
  const [tab, setTab] = useState<Tab>("pending");
  const [returns, setReturns] = useState<AdminReturn[]>(initialReturns);
  const [selectedId, setSelectedId] = useState<string | null>(
    initialReturns.find((r) => r.status === "requested")?.id ?? null,
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Form state for the detail pane's action section.
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
    setError(null);
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
      /* silent */
    }
  }, [storeId]);

  async function handleApprove() {
    if (!selected || busy) return;
    setBusy(true);
    setError(null);
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
        setError(body.message || `Approve failed (${res.status})`);
        return;
      }
      const updated = (await res.json()) as AdminReturn;
      setReturns((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      setTab("approved");
      router.refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleReject() {
    if (!selected || busy) return;
    if (!rejectReason.trim()) {
      setError("A reason is required to reject.");
      return;
    }
    setBusy(true);
    setError(null);
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
        setError(body.message || `Reject failed (${res.status})`);
        return;
      }
      const updated = (await res.json()) as AdminReturn;
      setReturns((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      setTab("rejected");
      router.refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdatePickup() {
    if (!selected || busy) return;
    setBusy(true);
    setError(null);
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
        setError(body.message || `Update failed (${res.status})`);
        return;
      }
      const updated = (await res.json()) as AdminReturn;
      setReturns((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      router.refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const fieldClass =
    "w-full resize-y rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary transition-colors focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]";

  return (
    <div className="grid min-h-[640px] grid-cols-1 overflow-hidden rounded-md border border-border-subtle bg-[color:var(--background-elevated)] lg:grid-cols-[minmax(0,360px)_minmax(0,1fr)]">
      <aside className="flex flex-col border-b border-border-subtle lg:border-b-0 lg:border-r">
        <div className="flex flex-wrap gap-1 border-b border-border-subtle p-2">
          {TABS.map((t) => {
            const count = returns.filter((r) => t.match.includes(r.status)).length;
            const active = tab === t.key;
            return (
              <button
                key={t.key}
                type="button"
                onClick={() => setTab(t.key)}
                className={
                  "rounded-sm px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                  (active
                    ? "bg-[color:var(--ink-900)] text-[color:var(--primary-foreground)]"
                    : "text-foreground-secondary hover:bg-[color:var(--ink-900)]/[0.06] hover:text-foreground")
                }
              >
                {t.label}
                {count > 0 && (
                  <span
                    className={
                      "ml-1.5 rounded-full px-1.5 text-[10px] tabular-nums " +
                      (active
                        ? "bg-[color:var(--primary-foreground)]/20"
                        : "bg-[color:var(--ink-900)]/10")
                    }
                  >
                    {count}
                  </span>
                )}
              </button>
            );
          })}
        </div>
        <div className="max-h-[640px] overflow-y-auto">
          {visible.length === 0 ? (
            <p className="p-6 text-center text-xs text-foreground-tertiary">
              No returns in this bucket yet.
            </p>
          ) : (
            visible.map((r) => (
              <button
                key={r.id}
                type="button"
                onClick={() => {
                  setSelectedId(r.id);
                  refresh().catch(() => {});
                }}
                className={
                  "w-full border-b border-border-subtle px-4 py-3 text-left transition-colors focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)] " +
                  (selectedId === r.id
                    ? "bg-[color:var(--ink-900)]/[0.04]"
                    : "hover:bg-[color:var(--ink-900)]/[0.03]")
                }
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-[11px] text-foreground-tertiary">
                    {r.return_number}
                  </span>
                  <span className="rounded-sm bg-[color:var(--ink-900)]/[0.08] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-foreground-secondary">
                    {r.type}
                  </span>
                </div>
                <p className="mt-1 truncate text-sm font-medium text-foreground">
                  {r.reason ?? "(no reason given)"}
                </p>
                <p className="mt-0.5 text-[11px] text-foreground-tertiary">
                  {formatRelative(r.requested_at)}
                </p>
              </button>
            ))
          )}
        </div>
      </aside>

      <section className="flex flex-col overflow-hidden">
        {!selected ? (
          <div className="m-auto max-w-xs p-6 text-center text-sm text-foreground-tertiary">
            Pick a request on the left to approve, reject, or edit pickup
            details.
          </div>
        ) : (
          <>
            <header className="flex items-start justify-between gap-4 border-b border-border-subtle p-6">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-xs text-foreground-tertiary">
                  <span className="font-mono">{selected.return_number}</span>
                  <span aria-hidden="true">·</span>
                  <span className="uppercase tracking-wider">{selected.type}</span>
                </div>
                <h3 className="mt-1.5 font-serif text-lg font-medium text-foreground">
                  {selected.reason ?? "(no reason given)"}
                </h3>
                {selected.notes && (
                  <p className="mt-2 whitespace-pre-wrap text-sm text-foreground-secondary">
                    {selected.notes}
                  </p>
                )}
              </div>
            </header>

            <div className="flex flex-col gap-6 p-6">
              <div className="flex flex-col gap-2">
                <h4 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                  Items requested
                </h4>
                <ul className="flex flex-col gap-1 text-sm text-foreground">
                  {selected.items.map((it) => (
                    <li
                      key={it.id}
                      className="rounded-md bg-[color:var(--ink-900)]/[0.04] px-3 py-2 font-mono text-xs"
                    >
                      {it.quantity}× {it.order_item_id}
                      {it.reason && (
                        <span className="ml-2 text-foreground-tertiary">
                          · {it.reason}
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
              </div>

              {selected.status === "requested" && (
                <div className="flex flex-col gap-5">
                  <div className="flex flex-col gap-2">
                    <label
                      htmlFor="pickup"
                      className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
                    >
                      Pickup / logistics <span className="font-normal normal-case tracking-normal text-foreground-tertiary">— customer sees this</span>
                    </label>
                    <textarea
                      id="pickup"
                      value={pickupDetails}
                      onChange={(e) => setPickupDetails(e.target.value)}
                      rows={3}
                      placeholder={selected.type === "replace"
                        ? "e.g. Replacement ships via Delhivery on Monday, tracking added shortly."
                        : "e.g. Delhivery will collect on Monday 10am–2pm, no packing required."}
                      className={fieldClass}
                    />
                    <button
                      type="button"
                      onClick={handleApprove}
                      disabled={busy}
                      className="inline-flex w-fit items-center rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      {busy ? "Working..." : "Approve"}
                    </button>
                  </div>

                  <div className="flex flex-col gap-2 border-t border-border-subtle pt-5">
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
                    <button
                      type="button"
                      onClick={handleReject}
                      disabled={busy}
                      className="inline-flex w-fit items-center rounded-md border border-[color:var(--danger)] px-4 py-2 text-sm font-medium text-[color:var(--danger)] transition-colors hover:bg-[color:var(--danger)]/[0.06] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
                    >
                      Reject
                    </button>
                  </div>
                </div>
              )}

              {selected.status === "approved" && (
                <div className="flex flex-col gap-2">
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
                    onClick={handleUpdatePickup}
                    disabled={busy}
                    className="inline-flex w-fit items-center rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-4 py-2 text-sm font-medium text-foreground transition-colors hover:border-[color:var(--ink-900)]/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    Save pickup details
                  </button>
                </div>
              )}

              {selected.status === "rejected" && selected.reject_reason && (
                <div className="rounded-md border border-[color:var(--warning)]/30 bg-[color:var(--warning)]/[0.06] px-4 py-3 text-sm text-[color:var(--warning)]">
                  <strong className="font-semibold">Rejected:</strong> {selected.reject_reason}
                </div>
              )}

              {error && (
                <p role="alert" className="text-xs text-[color:var(--danger)]">
                  {error}
                </p>
              )}
            </div>
          </>
        )}
      </section>
    </div>
  );
}

function formatRelative(iso: string): string {
  try {
    const ms = Date.now() - new Date(iso).getTime();
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s}s ago`;
    if (s < 3600) return `${Math.round(s / 60)}m ago`;
    if (s < 86400) return `${Math.round(s / 3600)}h ago`;
    return `${Math.round(s / 86400)}d ago`;
  } catch {
    return "";
  }
}
