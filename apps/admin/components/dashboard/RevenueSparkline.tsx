"use client";

import { Line, LineChart, ResponsiveContainer, Tooltip } from "recharts";

interface RevenueSparklineProps {
  data: number[];
  /** ISO 4217 code of the store's currency — the tooltip must match the
      headline value, not assume USD. */
  currencyCode: string;
}

function formatCurrency(value: number, currencyCode: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: currencyCode,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(value);
}

function SparklineTooltip({
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

export function RevenueSparkline({ data, currencyCode }: RevenueSparklineProps) {
  const allZero = data.every((v) => v === 0);

  if (allZero) {
    return (
      <div className="relative flex h-10 w-full items-end justify-center">
        <div className="mb-2 w-full border-t border-dashed border-foreground-tertiary/40" />
        <p className="absolute text-[10px] text-foreground-tertiary">
          No sales yet
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
    <div className="h-10 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData}>
          <Tooltip
            content={<SparklineTooltip currencyCode={currencyCode} />}
            cursor={false}
          />
          <Line
            type="monotone"
            dataKey="value"
            stroke="var(--moss-700)"
            strokeWidth={1.5}
            strokeOpacity={0.7}
            strokeLinecap="round"
            dot={false}
            activeDot={{ r: 3, fill: "var(--moss-700)" }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
