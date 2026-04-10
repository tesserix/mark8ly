"use client";

import type { ReactNode } from "react";

interface StatCardProps {
  label: string;
  value: string;
  changePercent?: number;
  subtitle?: string;
  sparkline?: ReactNode;
}

export function StatCard({
  label,
  value,
  changePercent,
  subtitle,
  sparkline,
}: StatCardProps) {
  return (
    <div className="px-6 py-5 border-b border-border-subtle">
      <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
        {label}
      </p>
      <div className="mt-3 flex items-end justify-between gap-4">
        <div className="space-y-1">
          <p
            className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium text-foreground"
            style={{ fontFeatureSettings: '"tnum" 1' }}
          >
            {value}
          </p>
          <div className="flex items-center gap-2">
            {changePercent !== undefined && changePercent !== 0 && (
              <span
                className={`text-xs font-medium ${
                  changePercent > 0
                    ? "text-[color:var(--moss-700)]"
                    : "text-[color:var(--signal)]"
                }`}
              >
                {changePercent > 0 ? "+" : ""}
                {changePercent.toFixed(1)}%
              </span>
            )}
            {subtitle && (
              <span className="text-xs text-foreground-secondary">
                {subtitle}
              </span>
            )}
          </div>
        </div>
        {sparkline && <div className="shrink-0">{sparkline}</div>}
      </div>
    </div>
  );
}
