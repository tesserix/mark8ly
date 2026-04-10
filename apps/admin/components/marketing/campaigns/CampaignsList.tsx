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
    scheduled: "bg-amber-50 text-amber-700",
    sending: "bg-blue-50 text-blue-700",
    sent: "bg-moss-50 text-moss-700",
    paused: "bg-signal-50 text-signal-700",
    cancelled: "bg-ink-100 text-ink-400",
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
                ? "border-b-2 border-ink-900 text-ink-900"
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
            <tr className="border-b border-ink-200 text-xs font-medium uppercase tracking-wider text-ink-500">
              <th className="pb-3 pr-4">Name</th>
              <th className="pb-3 pr-4">Type</th>
              <th className="pb-3 pr-4">Status</th>
              <th className="pb-3 pr-4">Recipients</th>
              <th className="pb-3 pr-4">Delivered</th>
              <th className="pb-3 pr-4">Opened</th>
              <th className="pb-3 pr-4">Date</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-ink-100">
            {campaigns.map((c) => (
              <tr key={c.id} className="group">
                <td className="py-3 pr-4">
                  <Link
                    href={`/marketing/campaigns/${c.id}`}
                    className="text-sm font-medium text-moss-700 underline-offset-2 group-hover:underline"
                  >
                    {c.name}
                  </Link>
                </td>
                <td className="py-3 pr-4 text-ink-500 capitalize">
                  {c.type}
                </td>
                <td className="py-3 pr-4">{statusBadge(c.status)}</td>
                <td className="py-3 pr-4 font-mono text-ink-700">
                  {c.total_recipients.toLocaleString()}
                </td>
                <td className="py-3 pr-4 text-ink-500">
                  {c.delivered.toLocaleString()}
                  <span className="ml-1 text-xs text-ink-400">
                    ({pct(c.delivered, c.total_recipients)})
                  </span>
                </td>
                <td className="py-3 pr-4 text-ink-500">
                  {c.opened.toLocaleString()}
                  <span className="ml-1 text-xs text-ink-400">
                    ({pct(c.opened, c.total_recipients)})
                  </span>
                </td>
                <td className="py-3 pr-4 text-ink-500">
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
