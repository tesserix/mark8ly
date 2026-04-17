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

  return (
    <div className="grid min-h-[520px] grid-cols-[320px_1fr] overflow-hidden rounded-lg border border-border-subtle bg-background">
      <aside className="border-r border-border-subtle bg-background-elevated">
        <div className="flex flex-wrap gap-1 border-b border-border-subtle p-2">
          {TABS.map((t) => {
            const count = returns.filter((r) => t.match.includes(r.status)).length;
            const active = tab === t.key;
            return (
              <button
                key={t.key}
                type="button"
                onClick={() => setTab(t.key)}
                className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                  active
                    ? "bg-background text-foreground shadow-sm"
                    : "text-foreground-tertiary hover:text-foreground"
                }`}
              >
                {t.label}
                {count > 0 && (
                  <span className="ml-1.5 rounded-full bg-foreground/10 px-1.5 text-[10px] tabular-nums">
                    {count}
                  </span>
                )}
              </button>
            );
          })}
        </div>
        <div className="max-h-[520px] overflow-y-auto">
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
                className={`w-full border-b border-border-subtle px-4 py-3 text-left transition-colors ${
                  selectedId === r.id
                    ? "bg-background"
                    : "hover:bg-background"
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-[11px] text-foreground-tertiary">
                    {r.return_number}
                  </span>
                  <span
                    className={`rounded-full px-2 py-0.5 text-[10px] uppercase tracking-wider ${
                      r.type === "replace"
                        ? "bg-indigo-100 text-indigo-800"
                        : "bg-slate-100 text-slate-700"
                    }`}
                  >
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
            <header className="flex items-start justify-between gap-4 border-b border-border-subtle p-5">
              <div>
                <div className="flex items-center gap-2 text-xs font-mono text-foreground-tertiary">
                  <span>{selected.return_number}</span>
                  <span className="opacity-50">·</span>
                  <span className="uppercase">{selected.type}</span>
                </div>
                <h3 className="mt-1 text-lg font-medium text-foreground">
                  {selected.reason ?? "(no reason given)"}
                </h3>
                {selected.notes && (
                  <p className="mt-1 whitespace-pre-wrap text-sm text-foreground-secondary">
                    {selected.notes}
                  </p>
                )}
              </div>
              <StatusPill status={selected.status} />
            </header>

            <div className="grid gap-4 p-5">
              <div className="grid gap-1">
                <h4 className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
                  Items requested
                </h4>
                <ul className="space-y-1 text-sm text-foreground">
                  {selected.items.map((it) => (
                    <li
                      key={it.id}
                      className="rounded-md bg-background-elevated px-3 py-2 font-mono text-xs"
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
                <ActionBlock>
                  <label
                    htmlFor="pickup"
                    className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary"
                  >
                    Pickup / logistics (customer sees this)
                  </label>
                  <textarea
                    id="pickup"
                    value={pickupDetails}
                    onChange={(e) => setPickupDetails(e.target.value)}
                    rows={3}
                    placeholder={selected.type === "replace"
                      ? "e.g. Replacement ships via Delhivery on Monday, tracking added shortly."
                      : "e.g. Delhivery will collect on Monday 10am–2pm, no packing required."}
                    className="w-full resize-y rounded-md border border-border-subtle bg-background px-3 py-2 text-sm"
                  />
                  <div className="flex flex-wrap items-center gap-2">
                    <button
                      type="button"
                      onClick={handleApprove}
                      disabled={busy}
                      className="rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background hover:opacity-90 disabled:opacity-50"
                    >
                      Approve
                    </button>
                    <span className="mx-2 text-xs text-foreground-tertiary">
                      or
                    </span>
                    <input
                      type="text"
                      value={rejectReason}
                      onChange={(e) => setRejectReason(e.target.value)}
                      placeholder="Rejection reason"
                      className="flex-1 min-w-[200px] rounded-md border border-border-subtle bg-background px-3 py-2 text-sm"
                    />
                    <button
                      type="button"
                      onClick={handleReject}
                      disabled={busy}
                      className="rounded-md border border-border-subtle bg-background px-4 py-2 text-sm text-foreground hover:bg-background-elevated disabled:opacity-50"
                    >
                      Reject
                    </button>
                  </div>
                </ActionBlock>
              )}

              {selected.status === "approved" && (
                <ActionBlock>
                  <label
                    htmlFor="pickup-edit"
                    className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary"
                  >
                    Update pickup / logistics
                  </label>
                  <textarea
                    id="pickup-edit"
                    value={pickupDetails}
                    onChange={(e) => setPickupDetails(e.target.value)}
                    rows={3}
                    className="w-full resize-y rounded-md border border-border-subtle bg-background px-3 py-2 text-sm"
                  />
                  <button
                    type="button"
                    onClick={handleUpdatePickup}
                    disabled={busy}
                    className="self-start rounded-md border border-border-subtle bg-background px-4 py-2 text-sm hover:bg-background-elevated disabled:opacity-50"
                  >
                    Save pickup details
                  </button>
                </ActionBlock>
              )}

              {selected.status === "rejected" && selected.reject_reason && (
                <div className="rounded-md border border-amber-300/40 bg-amber-50 px-3 py-2 text-xs text-amber-900">
                  <strong>Rejected:</strong> {selected.reject_reason}
                </div>
              )}

              {error && (
                <p role="alert" className="text-xs text-red-600">
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

function ActionBlock({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid gap-2 rounded-lg border border-border-subtle bg-background-elevated p-4">
      {children}
    </div>
  );
}

function StatusPill({ status }: { status: AdminReturnStatus }) {
  const cls =
    status === "requested"
      ? "bg-amber-100 text-amber-900"
      : status === "approved"
        ? "bg-emerald-100 text-emerald-800"
        : status === "received"
          ? "bg-sky-100 text-sky-800"
          : status === "refunded"
            ? "bg-violet-100 text-violet-800"
            : "bg-slate-200 text-slate-700";
  return (
    <span
      className={`rounded-full px-3 py-1 text-[11px] font-medium uppercase tracking-wider ${cls}`}
    >
      {status}
    </span>
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
