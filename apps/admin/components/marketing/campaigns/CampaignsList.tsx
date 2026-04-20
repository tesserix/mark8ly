"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import type { AdminCampaign, CampaignStatus } from "@/lib/api/campaigns-api";

interface CampaignsListProps {
  campaigns: AdminCampaign[];
  meta?: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
  storeId: string;
}

const STATUS_TABS: { label: string; value: string }[] = [
  { label: "All", value: "" },
  { label: "Draft", value: "draft" },
  { label: "Scheduled", value: "scheduled" },
  { label: "Sending", value: "sending" },
  { label: "Sent", value: "sent" },
  { label: "Paused", value: "paused" },
];

// Dot + label badge matching the orders/returns visual language.
// Variant semantics:
//   good     → sent / sending (positive terminal or active dispatch)
//   active   → scheduled (in-progress, not yet terminal)
//   muted    → draft / paused / cancelled (inactive states)
function statusBadge(status: CampaignStatus) {
  const variant: "good" | "active" | "muted" =
    status === "sent" || status === "sending"
      ? "good"
      : status === "scheduled"
        ? "active"
        : "muted";
  const dotClass =
    variant === "good"
      ? "bg-[color:var(--moss-700)]"
      : variant === "active"
        ? "border border-[color:var(--moss-700)] bg-transparent"
        : "bg-[color:var(--ink-900)] opacity-40";
  const label = status.charAt(0).toUpperCase() + status.slice(1);
  return (
    <span className="inline-flex items-center gap-2 text-sm text-foreground">
      <span
        aria-hidden="true"
        className={`inline-block h-2 w-2 rounded-full ${dotClass}`}
      />
      {label}
    </span>
  );
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return "--";
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function pct(numerator: number, denominator: number): string {
  if (denominator === 0) return "--";
  return `${Math.round((numerator / denominator) * 100)}%`;
}

export function CampaignsList({
  campaigns,
  storeId,
}: CampaignsListProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const activeStatus = searchParams.get("status") ?? "";

  function handleTabClick(status: string) {
    const params = new URLSearchParams(searchParams.toString());
    if (status) {
      params.set("status", status);
    } else {
      params.delete("status");
    }
    params.delete("page");
    router.push(`/marketing/campaigns?${params.toString()}`);
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Underline tabs — matches orders/returns pattern. */}
      <nav
        aria-label="Filter campaigns by status"
        className="flex flex-wrap items-center gap-x-1 gap-y-0 border-b border-border-subtle px-1"
      >
        {STATUS_TABS.map((tab) => {
          const active = activeStatus === tab.value;
          return (
            <button
              key={tab.value}
              onClick={() => handleTabClick(tab.value)}
              aria-current={active ? "true" : undefined}
              className={
                "flex shrink-0 items-baseline gap-1 border-b-2 px-2.5 py-2.5 text-[11px] font-semibold uppercase tracking-[0.08em] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                (active
                  ? "border-[color:var(--ink-900)] text-foreground"
                  : "border-transparent text-foreground-tertiary hover:text-foreground")
              }
            >
              {tab.label}
            </button>
          );
        })}
      </nav>

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-[color:var(--ink-900)]/15 text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
              <th className="pb-3 pl-4 pr-3">Name</th>
              <th className="pb-3 px-3">Type</th>
              <th className="pb-3 px-3">Status</th>
              <th className="pb-3 px-3">Recipients</th>
              <th className="pb-3 px-3">Delivered</th>
              <th className="pb-3 px-3">Opened</th>
              <th className="pb-3 px-3 pr-4">Date</th>
            </tr>
          </thead>
          <tbody>
            {campaigns.map((c) => (
              <tr
                key={c.id}
                className="group border-b border-[color:var(--ink-900)]/10 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-within:bg-[color:var(--ink-900)]/[0.03]"
              >
                <td className="py-3 pl-4 pr-3">
                  <Link
                    href={`/marketing/campaigns/${c.id}`}
                    className="rounded-sm font-serif text-base text-foreground transition-colors group-hover:text-[color:var(--moss-700)] group-hover:underline underline-offset-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                  >
                    {c.name}
                  </Link>
                </td>
                <td className="py-3 px-3 capitalize text-foreground-secondary">
                  {c.type}
                </td>
                <td className="py-3 px-3">{statusBadge(c.status)}</td>
                <td className="py-3 px-3 tabular-nums text-foreground">
                  {c.total_recipients.toLocaleString()}
                </td>
                <td className="py-3 px-3 tabular-nums text-foreground-secondary">
                  {c.delivered.toLocaleString()}
                  <span className="ml-1 text-xs text-foreground-tertiary">
                    ({pct(c.delivered, c.total_recipients)})
                  </span>
                </td>
                <td className="py-3 px-3 tabular-nums text-foreground-secondary">
                  {c.opened.toLocaleString()}
                  <span className="ml-1 text-xs text-foreground-tertiary">
                    ({pct(c.opened, c.total_recipients)})
                  </span>
                </td>
                <td className="py-3 px-3 pr-4 text-foreground-tertiary">
                  {formatDate(c.sent_at ?? c.scheduled_at ?? c.created_at)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
