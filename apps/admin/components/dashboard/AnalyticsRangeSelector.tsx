"use client";

import type { AnalyticsRange } from "@/lib/api/marketplace-api";

interface AnalyticsRangeSelectorProps {
  value: AnalyticsRange;
  onChange: (range: AnalyticsRange) => void;
  disabled?: boolean;
}

const OPTIONS: ReadonlyArray<{ value: AnalyticsRange; label: string }> = [
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
];

// Editorial underline-selected segmented control. Three buttons in a
// role="group" rather than Tabs, so the tabs used for Sales/Orders/etc.
// below it keep their navigational semantics unambiguous.
export function AnalyticsRangeSelector({
  value,
  onChange,
  disabled,
}: AnalyticsRangeSelectorProps) {
  return (
    <div
      role="group"
      aria-label="Analytics time range"
      className="flex items-baseline gap-5"
    >
      {OPTIONS.map((opt) => {
        const active = value === opt.value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            disabled={disabled}
            aria-pressed={active}
            className={`
              relative text-xs font-medium uppercase tracking-[0.14em]
              transition-[color,opacity] duration-150
              focus-visible:outline-2 focus-visible:outline-offset-4
              focus-visible:outline-[color:var(--moss-700)]
              disabled:cursor-not-allowed disabled:opacity-60
              ${
                active
                  ? "text-[color:var(--ink-900)] after:content-[''] after:absolute after:-bottom-1.5 after:left-0 after:right-0 after:h-px after:bg-[color:var(--ink-900)]"
                  : "text-foreground-tertiary hover:text-[color:var(--ink-900)]/80"
              }
            `}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
