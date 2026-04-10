"use client";

import { Line, LineChart, ResponsiveContainer, Tooltip } from "recharts";

interface RevenueSparklineProps {
  data: number[];
}

function formatCurrency(value: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(value);
}

function SparklineTooltip({
  active,
  payload,
}: {
  active?: boolean;
  payload?: Array<{ payload: { day: string; value: number } }>;
}) {
  if (!active || !payload?.[0]) return null;
  const { day, value } = payload[0].payload;
  return (
    <div className="rounded bg-foreground px-2 py-1 text-xs text-background shadow">
      <p>{day}</p>
      <p className="font-medium">{formatCurrency(value)}</p>
    </div>
  );
}

export function RevenueSparkline({ data }: RevenueSparklineProps) {
  const allZero = data.every((v) => v === 0);

  if (allZero) {
    return (
      <div className="flex h-[50px] w-[100px] items-end justify-center">
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
    <div className="h-[50px] w-[100px]">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData}>
          <Tooltip
            content={<SparklineTooltip />}
            cursor={false}
          />
          <Line
            type="monotone"
            dataKey="value"
            stroke="var(--moss-700)"
            strokeWidth={2}
            strokeLinecap="round"
            dot={false}
            activeDot={{ r: 3, fill: "var(--moss-700)" }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
