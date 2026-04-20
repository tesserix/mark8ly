"use client";

import type { AdminCampaign, CampaignStatus } from "@/lib/api/campaigns-api";
import { CampaignActions } from "./CampaignActions";

interface CampaignAnalyticsProps {
  campaign: AdminCampaign;
}

function statusBadge(status: CampaignStatus) {
  const colors: Record<string, string> = {
    draft: "bg-ink-100 text-ink-500",
    scheduled: "bg-ink-100 text-ink-700",
    sending: "bg-moss-700/10 text-moss-700",
    sent: "bg-moss-700/10 text-moss-700",
    paused: "bg-ink-100 text-ink-500",
    cancelled: "bg-ink-100 text-ink-500",
    failed: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  };
  return (
    <span
      className={`inline-block rounded-full px-2.5 py-1 text-xs font-medium capitalize ${colors[status] ?? "bg-ink-100 text-ink-500"}`}
    >
      {status}
    </span>
  );
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return "--";
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "long",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

function pct(numerator: number, denominator: number): string {
  if (denominator === 0) return "0%";
  return `${Math.round((numerator / denominator) * 100)}%`;
}

interface MetricCardProps {
  label: string;
  value: number;
  total: number;
  variant?: "default" | "danger";
}

function MetricCard({ label, value, total, variant = "default" }: MetricCardProps) {
  const percentage = pct(value, total);
  return (
    <div className="rounded-md bg-background-elevated p-4 shadow-sm">
      <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
        {label}
      </p>
      <p
        className={`mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium ${
          variant === "danger" ? "text-signal-700" : "text-ink-900"
        }`}
      >
        {value.toLocaleString()}
      </p>
      <p className="mt-0.5 text-xs text-ink-500">{percentage} of total</p>
    </div>
  );
}

export function CampaignAnalytics({
  campaign,
}: CampaignAnalyticsProps) {
  const c = campaign;

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <p className="eyebrow">Campaign</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium tracking-tight text-ink-900">
            {c.name}
          </h1>
          <div className="flex items-center gap-3">
            {statusBadge(c.status)}
            <span className="text-sm text-ink-500 capitalize">{c.type}</span>
          </div>
        </div>
        <CampaignActions campaign={c} />
      </div>

      <hr className="border-ink-200" />

      {/* Dates */}
      <div className="flex gap-8 text-sm">
        {c.sent_at && (
          <div>
            <span className="font-medium text-ink-700">Sent:</span>{" "}
            <span className="text-ink-500">{formatDate(c.sent_at)}</span>
          </div>
        )}
        {c.scheduled_at && (
          <div>
            <span className="font-medium text-ink-700">Scheduled:</span>{" "}
            <span className="text-ink-500">{formatDate(c.scheduled_at)}</span>
          </div>
        )}
        <div>
          <span className="font-medium text-ink-700">Created:</span>{" "}
          <span className="text-ink-500">{formatDate(c.created_at)}</span>
        </div>
      </div>

      {/* Progress bar for sending campaigns */}
      {c.status === "sending" && c.total_recipients > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="text-ink-700">Sending progress</span>
            <span className="font-mono text-ink-500">
              {c.delivered.toLocaleString()} / {c.total_recipients.toLocaleString()}
            </span>
          </div>
          <div className="h-2 w-full rounded-full bg-ink-100">
            <div
              className="h-2 rounded-full bg-moss-700 transition-[width] duration-300 ease-out"
              style={{
                width: `${Math.min(100, (c.delivered / c.total_recipients) * 100)}%`,
              }}
            />
          </div>
        </div>
      )}

      {/* Analytics grid */}
      <div>
        <h2 className="mb-4 text-sm font-medium uppercase tracking-wider text-ink-500">
          Delivery analytics
        </h2>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          <div className="lg:col-span-2">
            <MetricCard
              label="Recipients"
              value={c.total_recipients}
              total={c.total_recipients}
            />
          </div>
          <div className="lg:col-span-2">
            <MetricCard
              label="Delivered"
              value={c.delivered}
              total={c.total_recipients}
            />
          </div>
          <MetricCard
            label="Opened"
            value={c.opened}
            total={c.total_recipients}
          />
          <MetricCard
            label="Clicked"
            value={c.clicked}
            total={c.total_recipients}
          />
          <MetricCard
            label="Converted"
            value={c.converted}
            total={c.total_recipients}
          />
          <MetricCard
            label="Failed"
            value={c.failed}
            total={c.total_recipients}
            variant="danger"
          />
        </div>
      </div>

      {/* Revenue */}
      {parseFloat(c.revenue) > 0 && (
        <div className="rounded-md bg-background-elevated p-4 shadow-sm">
          <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
            Revenue attributed
          </p>
          <p className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-moss-700">
            ${parseFloat(c.revenue).toLocaleString(undefined, { minimumFractionDigits: 2 })}
          </p>
        </div>
      )}

      {/* Unsubscribed */}
      {c.unsubscribed > 0 && (
        <div className="rounded-md bg-background-elevated p-4 shadow-sm">
          <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
            Unsubscribed
          </p>
          <p className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-signal-700">
            {c.unsubscribed.toLocaleString()}
          </p>
          <p className="mt-0.5 text-xs text-ink-500">
            {pct(c.unsubscribed, c.total_recipients)} of total
          </p>
        </div>
      )}

      {/* Content preview */}
      {c.content && (
        <div>
          <h2 className="mb-4 text-sm font-medium uppercase tracking-wider text-ink-500">
            Content preview
          </h2>
          {c.subject && (
            <p className="mb-2 text-sm text-ink-700">
              <span className="font-medium">Subject:</span> {c.subject}
            </p>
          )}
          <div
            className="prose prose-sm max-w-none rounded-md bg-background-elevated p-6 text-ink-700 shadow-sm"
            dangerouslySetInnerHTML={{ __html: c.content }}
          />
        </div>
      )}
    </div>
  );
}
