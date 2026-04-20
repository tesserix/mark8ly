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

function statusBadge(status: CampaignStatus) {
  const colors: Record<string, string> = {
    draft: "bg-ink-100 text-ink-500",
    scheduled: "bg-ink-100 text-ink-700",
    sending: "bg-moss-700/10 text-moss-700",
    sent: "bg-moss-700/10 text-moss-700",
    paused: "bg-ink-100 text-ink-500",
    cancelled: "bg-ink-100 text-ink-500",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium capitalize ${colors[status] ?? "bg-ink-100 text-ink-500"}`}
    >
      {status}
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
    <div className="space-y-6">
      {/* Status filter tabs */}
      <nav className="flex gap-1 border-b border-ink-200">
        {STATUS_TABS.map((tab) => (
          <button
            key={tab.value}
            onClick={() => handleTabClick(tab.value)}
            className={`px-3 py-2 text-sm font-medium transition ${
              activeStatus === tab.value
                ? "border-b-2 border-moss-700 text-ink-900"
                : "text-ink-500 hover:text-ink-700"
            }`}
          >
            {tab.label}
          </button>
        ))}
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
          <tbody className="divide-y divide-[color:var(--ink-900)]/10">
            {campaigns.map((c) => (
              <tr
                key={c.id}
                className="group transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-within:bg-[color:var(--ink-900)]/[0.03]"
              >
                <td className="py-3 pl-4 pr-3">
                  <Link
                    href={`/marketing/campaigns/${c.id}`}
                    className="rounded-sm text-sm font-medium text-foreground underline-offset-2 transition-colors group-hover:text-[color:var(--moss-700)] group-hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                  >
                    {c.name}
                  </Link>
                </td>
                <td className="py-3 px-3 capitalize text-foreground-secondary">
                  {c.type}
                </td>
                <td className="py-3 px-3">{statusBadge(c.status)}</td>
                <td className="py-3 px-3 font-mono tabular-nums text-foreground">
                  {c.total_recipients.toLocaleString()}
                </td>
                <td className="py-3 px-3 text-foreground-secondary">
                  {c.delivered.toLocaleString()}
                  <span className="ml-1 text-xs text-foreground-tertiary">
                    ({pct(c.delivered, c.total_recipients)})
                  </span>
                </td>
                <td className="py-3 px-3 text-foreground-secondary">
                  {c.opened.toLocaleString()}
                  <span className="ml-1 text-xs text-foreground-tertiary">
                    ({pct(c.opened, c.total_recipients)})
                  </span>
                </td>
                <td className="py-3 px-3 pr-4 text-foreground-secondary">
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
