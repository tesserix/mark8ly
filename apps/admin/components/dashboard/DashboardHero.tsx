"use client";

import { Area, AreaChart, ResponsiveContainer, Tooltip } from "recharts";
import { ArrowDownRight, ArrowUpRight } from "lucide-react";

import type { DashboardStats } from "@/lib/api/marketplace-api";

interface DashboardHeroProps {
  stats: DashboardStats;
  currencyCode: string;
}

function formatCurrency(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(0)}`;
  }
}

/**
 * Dashboard masthead — the editorial replacement for the old 4-across stat
 * grid. Asymmetric: this-month revenue is the dominant serif headline with
 * its own trend line on the left; secondary figures stack in a narrow
 * hairline-ruled column on the right. No equal-weight cards.
 */
export function DashboardHero({ stats, currencyCode }: DashboardHeroProps) {
  const positive = stats.revenue_change_pct >= 0;
  const hasChange = stats.revenue_change_pct !== 0;

  return (
    <section className="grid grid-cols-1 gap-10 lg:grid-cols-[1.7fr_1fr] lg:gap-16">
      {/* Hero: this month's revenue — the one headline number. */}
      <div className="min-w-0">
        <p className="eyebrow">This month</p>
        <p className="mt-3 font-serif text-[clamp(2.75rem,1.8rem+4.2vw,4.75rem)] font-medium leading-[0.95] tracking-tight tabular-nums text-foreground">
          {formatCurrency(stats.revenue_month, currencyCode)}
        </p>

        <div className="mt-4 flex items-center gap-2 text-sm">
          {hasChange && (
            <span
              className={`inline-flex items-center gap-1 font-medium ${
                positive
                  ? "text-foreground"
                  : "text-[color:var(--danger)]"
              }`}
            >
              {positive ? (
                <ArrowUpRight className="h-4 w-4" strokeWidth={2.25} aria-hidden="true" />
              ) : (
                <ArrowDownRight className="h-4 w-4" strokeWidth={2.25} aria-hidden="true" />
              )}
              {Math.abs(stats.revenue_change_pct).toFixed(1)}%
            </span>
          )}
          <span className="text-foreground-tertiary">
            {hasChange ? "vs last month" : "No change vs last month"}
          </span>
        </div>

        <div className="mt-6">
          <HeroTrend data={stats.revenue_trend} currencyCode={currencyCode} />
        </div>
      </div>

      {/* Secondary revenue + customers — hairline-ruled rows, not cards. */}
      <div className="lg:border-l lg:border-border-subtle lg:pl-16">
        <dl>
          <SecondaryRow
            label="Today"
            value={formatCurrency(stats.revenue_today, currencyCode)}
          />
          <SecondaryRow
            label="This week"
            value={formatCurrency(stats.revenue_week, currencyCode)}
          />
          <SecondaryRow
            label="Customers"
            value={String(stats.customers_total)}
            hint={
              stats.customers_new_this_week > 0
                ? `+${stats.customers_new_this_week} this week`
                : undefined
            }
            last
          />
        </dl>
      </div>
    </section>
  );
}

function SecondaryRow({
  label,
  value,
  hint,
  last,
}: {
  label: string;
  value: string;
  hint?: string;
  last?: boolean;
}) {
  return (
    <div
      className={`flex items-baseline justify-between gap-4 py-4 ${
        last ? "" : "border-b border-border-subtle"
      }`}
    >
      <dt className="text-sm text-foreground-secondary">{label}</dt>
      <dd className="flex items-baseline gap-3">
        {hint && (
          <span className="text-xs text-[color:var(--moss-700)]">{hint}</span>
        )}
        <span className="font-serif text-2xl font-medium tabular-nums text-foreground">
          {value}
        </span>
      </dd>
    </div>
  );
}

// Taller than the compact StatCard sparkline — this trend is the visual
// anchor of the masthead, so it gets room to breathe and a soft moss fill.
function HeroTrend({
  data,
  currencyCode,
}: {
  data: number[];
  currencyCode: string;
}) {
  const allZero = data.every((v) => v === 0);

  if (allZero) {
    return (
      <div className="relative flex h-20 w-full items-end">
        <div className="mb-8 w-full border-t border-dashed border-foreground-tertiary/30" />
        <p className="absolute bottom-0 left-0 text-[11px] text-foreground-tertiary">
          No sales yet this month
        </p>
      </div>
    );
  }

  const today = new Date();
  const chartData = data.map((value, i) => {
    const d = new Date(today);
    d.setDate(d.getDate() - (data.length - 1 - i));
    return {
      day: d.toLocaleDateString("en-US", { month: "short", day: "numeric" }),
      value,
    };
  });

  return (
    <div className="h-20 w-full" aria-hidden="true">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={chartData} margin={{ top: 4, right: 0, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="heroRevenueFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--moss-700)" stopOpacity={0.14} />
              <stop offset="100%" stopColor="var(--moss-700)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <Tooltip content={<TrendTooltip currencyCode={currencyCode} />} cursor={false} />
          <Area
            type="monotone"
            dataKey="value"
            stroke="var(--moss-700)"
            strokeWidth={1.75}
            strokeLinecap="round"
            fill="url(#heroRevenueFill)"
            dot={false}
            activeDot={{ r: 3, fill: "var(--moss-700)" }}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function TrendTooltip({
  active,
  payload,
  currencyCode,
}: {
  active?: boolean;
  payload?: Array<{ payload: { day: string; value: number } }>;
  currencyCode: string;
}) {
  if (!active || !payload?.[0]) return null;
  const { day, value } = payload[0].payload;
  return (
    <div className="rounded bg-foreground px-2 py-1 text-xs text-background shadow">
      <p>{day}</p>
      <p className="font-medium">{formatCurrency(value, currencyCode)}</p>
    </div>
  );
}
