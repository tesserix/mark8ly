"use client";

// Recharts wrappers for the analytics tabs. All charts share the same
// editorial axis + tooltip treatment: no vertical gridlines, hairline
// horizontal grid, muted tick labels, Source Serif numerals in tooltips.

import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type {
  CustomerSegmentPoint,
  OrderStatusSeriesPoint,
  RatingDistribution,
  TimeSeriesPoint,
} from "@/lib/api/marketplace-api";

const MOSS = "var(--moss-700)";
const INK = "var(--ink-900)";
const SIGNAL = "var(--signal)";
const GRID = "rgb(14 14 12 / 0.06)";
const TICK_FILL = "rgb(14 14 12 / 0.4)";

const AXIS_TICK = { fontSize: 11, fill: TICK_FILL } as const;

function formatDateShort(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function formatCurrencyShort(
  value: number,
  currency: string,
  { compact = true }: { compact?: boolean } = {},
): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
      notation: compact ? "compact" : "standard",
      minimumFractionDigits: 0,
      maximumFractionDigits: compact ? 1 : 0,
    }).format(value);
  } catch {
    return `${currency} ${value.toFixed(0)}`;
  }
}

// ---------- Shared tooltip ----------

interface TooltipRow {
  label: string;
  value: string;
  dot?: string;
}
interface EditorialTooltipProps {
  active?: boolean;
  title?: string;
  rows: TooltipRow[];
}

function EditorialTooltip({ active, title, rows }: EditorialTooltipProps) {
  if (!active) return null;
  return (
    <div
      className="rounded-md border border-border-subtle bg-background-elevated px-3 py-2 text-xs shadow-sm"
      role="status"
    >
      {title && (
        <p className="text-foreground-tertiary">{title}</p>
      )}
      <div className="mt-1 space-y-1">
        {rows.map((row, i) => (
          <div key={i} className="flex items-center gap-2">
            {row.dot && (
              <span
                aria-hidden
                className="inline-block h-1.5 w-1.5 rounded-full"
                style={{ backgroundColor: row.dot }}
              />
            )}
            <span className="text-foreground-tertiary">{row.label}</span>
            <span
              className="ml-auto font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-sm font-medium text-foreground"
              style={{ fontFeatureSettings: '"tnum" 1' }}
            >
              {row.value}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------- Empty state ----------

export function ChartEmpty({ message }: { message: string }) {
  return (
    <div className="flex h-[240px] items-center justify-center text-sm text-foreground-tertiary">
      {message}
    </div>
  );
}

// ---------- Revenue area ----------

interface RevenueAreaChartProps {
  data: TimeSeriesPoint[];
  currency: string;
  height?: number;
}

export function RevenueAreaChart({
  data,
  currency,
  height = 260,
}: RevenueAreaChartProps) {
  if (!data.length || data.every((d) => d.value === 0)) {
    return <ChartEmpty message="No revenue in this period." />;
  }

  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <AreaChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="revenueFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={MOSS} stopOpacity={0.12} />
              <stop offset="100%" stopColor={MOSS} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke={GRID} vertical={false} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDateShort}
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            tickFormatter={(v: number) =>
              formatCurrencyShort(v, currency, { compact: true })
            }
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            width={56}
          />
          <Tooltip
            cursor={{ stroke: "rgb(14 14 12 / 0.08)", strokeWidth: 1 }}
            content={(props) => {
              const p = props.payload?.[0];
              if (!p) return null;
              const row = p.payload as TimeSeriesPoint;
              return (
                <EditorialTooltip
                  active={props.active}
                  title={formatDateShort(row.date)}
                  rows={[
                    {
                      label: "Revenue",
                      value: formatCurrencyShort(row.value, currency, {
                        compact: false,
                      }),
                    },
                  ]}
                />
              );
            }}
          />
          <Area
            type="monotone"
            dataKey="value"
            stroke={MOSS}
            strokeWidth={1.5}
            fill="url(#revenueFill)"
            dot={false}
            activeDot={{ r: 3, fill: MOSS }}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

// ---------- Orders stacked bar ----------

interface OrdersStatusChartProps {
  data: OrderStatusSeriesPoint[];
  height?: number;
}

export function OrdersStatusChart({
  data,
  height = 260,
}: OrdersStatusChartProps) {
  const total = data.reduce(
    (sum, d) =>
      sum + d.pending + d.confirmed + d.in_progress + d.fulfilled + d.cancelled,
    0,
  );
  if (!data.length || total === 0) {
    return <ChartEmpty message="No orders in this period." />;
  }

  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <BarChart
          data={data}
          margin={{ top: 8, right: 12, left: 0, bottom: 0 }}
        >
          <CartesianGrid stroke={GRID} vertical={false} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDateShort}
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            allowDecimals={false}
            width={32}
          />
          <Tooltip
            cursor={{ fill: "rgb(14 14 12 / 0.04)" }}
            content={(props) => {
              const p = props.payload?.[0];
              if (!p) return null;
              const row = p.payload as OrderStatusSeriesPoint;
              const rows: TooltipRow[] = [
                {
                  label: "Fulfilled",
                  value: String(row.fulfilled),
                  dot: "rgb(14 14 12 / 0.85)",
                },
                {
                  label: "In progress",
                  value: String(row.in_progress),
                  dot: "rgb(14 14 12 / 0.55)",
                },
                {
                  label: "Confirmed",
                  value: String(row.confirmed),
                  dot: "rgb(14 14 12 / 0.4)",
                },
                {
                  label: "Pending",
                  value: String(row.pending),
                  dot: "rgb(14 14 12 / 0.25)",
                },
                {
                  label: "Cancelled",
                  value: String(row.cancelled),
                  dot: SIGNAL,
                },
              ];
              return (
                <EditorialTooltip
                  active={props.active}
                  title={formatDateShort(row.date)}
                  rows={rows}
                />
              );
            }}
          />
          <Bar dataKey="fulfilled" stackId="s" fill="rgb(14 14 12 / 0.85)" />
          <Bar dataKey="in_progress" stackId="s" fill="rgb(14 14 12 / 0.55)" />
          <Bar dataKey="confirmed" stackId="s" fill="rgb(14 14 12 / 0.4)" />
          <Bar dataKey="pending" stackId="s" fill="rgb(14 14 12 / 0.25)" />
          <Bar dataKey="cancelled" stackId="s" fill={SIGNAL} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

// ---------- Orders volume line (half-width companion) ----------

interface OrdersLineChartProps {
  data: TimeSeriesPoint[];
  height?: number;
}

export function OrdersLineChart({ data, height = 260 }: OrdersLineChartProps) {
  if (!data.length || data.every((d) => d.value === 0)) {
    return <ChartEmpty message="No orders in this period." />;
  }
  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <LineChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid stroke={GRID} vertical={false} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDateShort}
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            allowDecimals={false}
            width={32}
          />
          <Tooltip
            cursor={{ stroke: "rgb(14 14 12 / 0.08)", strokeWidth: 1 }}
            content={(props) => {
              const p = props.payload?.[0];
              if (!p) return null;
              const row = p.payload as TimeSeriesPoint;
              return (
                <EditorialTooltip
                  active={props.active}
                  title={formatDateShort(row.date)}
                  rows={[{ label: "Orders", value: String(row.value) }]}
                />
              );
            }}
          />
          <Line
            type="monotone"
            dataKey="value"
            stroke={INK}
            strokeOpacity={0.75}
            strokeWidth={1.5}
            dot={false}
            activeDot={{ r: 3, fill: INK }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

// ---------- Customers new vs returning ----------

interface CustomersSegmentChartProps {
  data: CustomerSegmentPoint[];
  height?: number;
}

export function CustomersSegmentChart({
  data,
  height = 260,
}: CustomersSegmentChartProps) {
  const total = data.reduce((sum, d) => sum + d.new + d.returning, 0);
  if (!data.length || total === 0) {
    return <ChartEmpty message="No customer activity in this period." />;
  }
  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <BarChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid stroke={GRID} vertical={false} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDateShort}
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            allowDecimals={false}
            width={32}
          />
          <Tooltip
            cursor={{ fill: "rgb(14 14 12 / 0.04)" }}
            content={(props) => {
              const p = props.payload?.[0];
              if (!p) return null;
              const row = p.payload as CustomerSegmentPoint;
              return (
                <EditorialTooltip
                  active={props.active}
                  title={formatDateShort(row.date)}
                  rows={[
                    {
                      label: "New",
                      value: String(row.new),
                      dot: MOSS,
                    },
                    {
                      label: "Returning",
                      value: String(row.returning),
                      dot: "rgb(14 14 12 / 0.5)",
                    },
                  ]}
                />
              );
            }}
          />
          <Bar dataKey="new" stackId="c" fill={MOSS} />
          <Bar dataKey="returning" stackId="c" fill="rgb(14 14 12 / 0.5)" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

// ---------- Rating distribution ----------

interface RatingDistributionChartProps {
  distribution: RatingDistribution;
}

export function RatingDistributionChart({
  distribution,
}: RatingDistributionChartProps) {
  const total =
    distribution.r1 +
    distribution.r2 +
    distribution.r3 +
    distribution.r4 +
    distribution.r5;

  if (total === 0) {
    return <ChartEmpty message="No reviews yet." />;
  }

  const rows = [
    { label: "5 ★", count: distribution.r5 },
    { label: "4 ★", count: distribution.r4 },
    { label: "3 ★", count: distribution.r3 },
    { label: "2 ★", count: distribution.r2 },
    { label: "1 ★", count: distribution.r1 },
  ];

  return (
    <div className="space-y-2 py-2">
      {rows.map((r) => {
        const pct = total > 0 ? (r.count / total) * 100 : 0;
        return (
          <div key={r.label} className="flex items-center gap-3 text-xs">
            <span className="w-10 shrink-0 text-foreground-tertiary">
              {r.label}
            </span>
            <div className="relative flex-1 h-2 overflow-hidden rounded-full bg-[color:var(--ink-900)]/6">
              <div
                className="h-full rounded-full bg-[color:var(--ink-900)]/85"
                style={{ width: `${pct}%` }}
              />
            </div>
            <span
              className="w-12 shrink-0 text-right font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-sm font-medium text-foreground"
              style={{ fontFeatureSettings: '"tnum" 1' }}
            >
              {r.count}
            </span>
          </div>
        );
      })}
    </div>
  );
}

// ---------- Reviews over time line ----------

export function ReviewsLineChart({
  data,
  height = 200,
}: {
  data: TimeSeriesPoint[];
  height?: number;
}) {
  if (!data.length || data.every((d) => d.value === 0)) {
    return <ChartEmpty message="No reviews in this period." />;
  }
  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <LineChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid stroke={GRID} vertical={false} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDateShort}
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            tick={AXIS_TICK}
            axisLine={false}
            tickLine={false}
            allowDecimals={false}
            width={32}
          />
          <Tooltip
            cursor={{ stroke: "rgb(14 14 12 / 0.08)", strokeWidth: 1 }}
            content={(props) => {
              const p = props.payload?.[0];
              if (!p) return null;
              const row = p.payload as TimeSeriesPoint;
              return (
                <EditorialTooltip
                  active={props.active}
                  title={formatDateShort(row.date)}
                  rows={[{ label: "Reviews", value: String(row.value) }]}
                />
              );
            }}
          />
          <Line
            type="monotone"
            dataKey="value"
            stroke={INK}
            strokeOpacity={0.75}
            strokeWidth={1.5}
            dot={false}
            activeDot={{ r: 3, fill: INK }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

