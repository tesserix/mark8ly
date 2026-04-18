"use client";

import { useEffect, useState } from "react";

// AuditEvent mirrors services/otto/internal/conversation/audit.go.
// Kept local to this component — it's only consumed here, so we
// don't need to export a shared type.
interface AuditEvent {
  id: string;
  conversation_id: string;
  case_id?: string;
  action: string;
  actor: { type: "staff" | "customer" | "system"; id?: string; name?: string; email?: string };
  meta?: Record<string, unknown>;
  at: string;
}

const ACTION_LABELS: Record<string, string> = {
  "case.created": "Case created",
  "case.accepted": "Accepted (manual)",
  "case.accepted_next": "Accepted (FIFO)",
  "case.closed": "Closed by staff",
  "case.closed_by_customer": "Closed by customer",
  "case.closed_inactivity": "Auto-closed — inactivity",
  "case.reopened": "Reopened",
  "feedback.submitted": "Feedback submitted",
  "staff.available": "Staff marked available",
  "staff.paused": "Staff paused",
};

function actorLabel(a: AuditEvent["actor"]): string {
  if (a.type === "system") return "System";
  const name = a.name || a.email || a.id || "Unknown";
  return a.type === "staff" ? `${name} (staff)` : `${name} (customer)`;
}

function formatWhen(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

export function OttoAuditLog() {
  const [events, setEvents] = useState<AuditEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await fetch("/api/admin/otto/audit", {
          credentials: "include",
        });
        if (!res.ok) throw new Error(`audit fetch failed (${res.status})`);
        const body = (await res.json()) as { events: AuditEvent[] };
        if (!cancelled) setEvents(body.events);
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (error) {
    return (
      <p role="alert" className="text-sm text-[color:var(--danger,#8e1a1a)]">
        Could not load audit log: {error}
      </p>
    );
  }
  if (events === null) {
    return <p className="text-sm text-foreground-tertiary">Loading…</p>;
  }
  if (events.length === 0) {
    return (
      <p className="text-sm text-foreground-tertiary">
        No events yet. As soon as a customer opens a case or a staff
        member toggles their availability, entries will show up here.
      </p>
    );
  }
  return (
    <div className="overflow-auto border border-border-subtle rounded-sm">
      <table className="w-full text-sm">
        <thead className="text-left text-xs uppercase tracking-[0.12em] text-foreground-tertiary">
          <tr className="border-b border-border-subtle">
            <th className="px-3 py-2 font-semibold">When</th>
            <th className="px-3 py-2 font-semibold">Action</th>
            <th className="px-3 py-2 font-semibold">Case</th>
            <th className="px-3 py-2 font-semibold">Actor</th>
            <th className="px-3 py-2 font-semibold">Details</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => (
            <tr key={e.id} className="border-b border-border-subtle last:border-b-0">
              <td className="px-3 py-2 whitespace-nowrap text-foreground-secondary">
                {formatWhen(e.at)}
              </td>
              <td className="px-3 py-2 font-medium text-foreground">
                {ACTION_LABELS[e.action] ?? e.action}
              </td>
              <td className="px-3 py-2 font-mono text-xs text-foreground-secondary">
                {e.case_id ?? "—"}
              </td>
              <td className="px-3 py-2 text-foreground-secondary">
                {actorLabel(e.actor)}
              </td>
              <td className="px-3 py-2 text-foreground-secondary">
                {e.meta ? (
                  <code className="font-mono text-xs">
                    {Object.entries(e.meta)
                      .map(([k, v]) => `${k}=${JSON.stringify(v)}`)
                      .join(" ")}
                  </code>
                ) : (
                  <span className="text-foreground-tertiary">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
