"use client";

import type { AdminCampaign, CampaignStatus } from "@/lib/api/campaigns-api";
import { sanitizeRichHtml } from "@/lib/sanitize";
import { CampaignActions } from "./CampaignActions";

interface CampaignAnalyticsProps {
  campaign: AdminCampaign;
}

function statusBadge(status: CampaignStatus) {
  const colors: Record<string, string> = {
    draft: "bg-[color:var(--ink-900)]/5 text-foreground-tertiary",
    scheduled: "bg-[color:var(--ink-900)]/5 text-foreground-secondary",
    sending: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
    sent: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
    paused: "bg-[color:var(--ink-900)]/5 text-foreground-tertiary",
    cancelled: "bg-[color:var(--ink-900)]/5 text-foreground-tertiary",
    failed: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  };
  return (
    <span
      className={`inline-block rounded-full px-2.5 py-1 text-xs font-medium capitalize ${colors[status] ?? "bg-[color:var(--ink-900)]/5 text-foreground-tertiary"}`}
    >
      {status}
    </span>
  );
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return "—";
  return new Date(dateStr).toLocaleDateString(undefined, {
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

interface MetricProps {
  label: string;
  value: number;
  total: number;
  variant?: "default" | "danger";
}

// Metric cell — hairline-grid member, not a boxed card. Reads as a
// tabular dl row: label above, serif numeral below, percent share last.
function Metric({ label, value, total, variant = "default" }: MetricProps) {
  const percentage = pct(value, total);
  return (
    <div className="border-t border-border-subtle pt-4 first:border-t-0 first:pt-0 sm:border-t-0 sm:pt-0">
      <p className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
        {label}
      </p>
      <p
        className={`mt-1 font-serif text-2xl font-medium tabular-nums ${
          variant === "danger"
            ? "text-[color:var(--signal)]"
            : "text-foreground"
        }`}
      >
        {value.toLocaleString()}
      </p>
      <p className="mt-0.5 text-xs text-foreground-tertiary">
        {percentage} of total
      </p>
    </div>
  );
}

export function CampaignAnalytics({
  campaign,
}: CampaignAnalyticsProps) {
  const c = campaign;

  return (
    <div className="space-y-10">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-2">
          <p className="eyebrow">Campaign</p>
          <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
            {c.name}
          </h1>
          <div className="flex items-center gap-3">
            {statusBadge(c.status)}
            <span className="text-sm capitalize text-foreground-tertiary">
              {c.type}
            </span>
          </div>
        </div>
        <CampaignActions campaign={c} />
      </div>

      {/* Dates */}
      <div className="flex flex-wrap gap-x-8 gap-y-2 border-t border-border-subtle pt-6 text-sm">
        {c.sent_at && (
          <div>
            <span className="font-medium text-foreground-secondary">Sent</span>{" "}
            <span className="text-foreground-tertiary">
              {formatDate(c.sent_at)}
            </span>
          </div>
        )}
        {c.scheduled_at && (
          <div>
            <span className="font-medium text-foreground-secondary">
              Scheduled
            </span>{" "}
            <span className="text-foreground-tertiary">
              {formatDate(c.scheduled_at)}
            </span>
          </div>
        )}
        <div>
          <span className="font-medium text-foreground-secondary">Created</span>{" "}
          <span className="text-foreground-tertiary">
            {formatDate(c.created_at)}
          </span>
        </div>
      </div>

      {/* Progress bar for sending campaigns */}
      {c.status === "sending" && c.total_recipients > 0 && (
        <div className="space-y-2 border-t border-border-subtle pt-6">
          <div className="flex items-center justify-between text-sm">
            <span className="text-foreground-secondary">Sending progress</span>
            <span className="font-mono tabular-nums text-foreground-tertiary">
              {c.delivered.toLocaleString()} /{" "}
              {c.total_recipients.toLocaleString()}
            </span>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-full bg-[color:var(--ink-900)]/10">
            <div
              className="h-2 rounded-full bg-[color:var(--moss-700)] transition-[width] duration-300 ease-out"
              style={{
                width: `${Math.min(100, (c.delivered / c.total_recipients) * 100)}%`,
              }}
            />
          </div>
        </div>
      )}

      {/* Delivery grid — hairline grid, not boxed metric cards */}
      <section className="space-y-4 border-t border-border-subtle pt-6">
        <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          Delivery
        </h2>
        <div className="grid grid-cols-2 gap-x-6 gap-y-6 sm:grid-cols-3 lg:grid-cols-6">
          <Metric
            label="Recipients"
            value={c.total_recipients}
            total={c.total_recipients}
          />
          <Metric
            label="Delivered"
            value={c.delivered}
            total={c.total_recipients}
          />
          <Metric
            label="Opened"
            value={c.opened}
            total={c.total_recipients}
          />
          <Metric
            label="Clicked"
            value={c.clicked}
            total={c.total_recipients}
          />
          <Metric
            label="Converted"
            value={c.converted}
            total={c.total_recipients}
          />
          <Metric
            label="Failed"
            value={c.failed}
            total={c.total_recipients}
            variant="danger"
          />
        </div>
      </section>

      {/* Revenue */}
      {parseFloat(c.revenue) > 0 && (
        <section className="space-y-2 border-t border-border-subtle pt-6">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
            Revenue attributed
          </p>
          <p className="font-serif text-3xl font-medium tabular-nums text-[color:var(--moss-700)]">
            $
            {parseFloat(c.revenue).toLocaleString(undefined, {
              minimumFractionDigits: 2,
            })}
          </p>
        </section>
      )}

      {/* Unsubscribed */}
      {c.unsubscribed > 0 && (
        <section className="space-y-1 border-t border-border-subtle pt-6">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
            Unsubscribed
          </p>
          <p className="font-serif text-2xl font-medium tabular-nums text-[color:var(--signal)]">
            {c.unsubscribed.toLocaleString()}
          </p>
          <p className="text-xs text-foreground-tertiary">
            {pct(c.unsubscribed, c.total_recipients)} of total
          </p>
        </section>
      )}

      {/* Content preview */}
      {c.content && (
        <section className="space-y-4 border-t border-border-subtle pt-6">
          <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Content preview
          </h2>
          {c.subject && (
            <p className="text-sm text-foreground-secondary">
              <span className="font-medium text-foreground">Subject</span>{" "}
              {c.subject}
            </p>
          )}
          <div
            className="prose prose-sm max-w-none rounded-md border border-border-subtle bg-[color:var(--background-elevated)] p-6 text-foreground-secondary"
            dangerouslySetInnerHTML={{ __html: sanitizeRichHtml(c.content) }}
          />
        </section>
      )}
    </div>
  );
}
