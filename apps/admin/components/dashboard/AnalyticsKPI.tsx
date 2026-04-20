"use client";

interface AnalyticsKPIProps {
  label: string;
  value: string;
  deltaPercent?: number | null;
  subtitle?: string;
}

// Compact stat tile used inside analytics tabs — smaller than the
// top-level StatCard, no hover affordance, no link. Renders numerals in
// Source Serif 4 with tabular figures to match the editorial treatment.
export function AnalyticsKPI({
  label,
  value,
  deltaPercent,
  subtitle,
}: AnalyticsKPIProps) {
  return (
    <div className="px-5 py-4">
      <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
        {label}
      </p>
      <p
        className="mt-2 font-serif text-2xl font-medium text-foreground"
        style={{ fontFeatureSettings: '"tnum" 1' }}
      >
        {value}
      </p>
      {(deltaPercent !== undefined && deltaPercent !== null && deltaPercent !== 0) ||
      subtitle ? (
        <div className="mt-1 flex items-center gap-2 text-xs">
          {deltaPercent !== undefined &&
            deltaPercent !== null &&
            deltaPercent !== 0 && (
              <span
                className={`font-medium ${
                  deltaPercent > 0
                    ? "text-[color:var(--moss-700)]"
                    : "text-[color:var(--signal)]"
                }`}
              >
                {deltaPercent > 0 ? "+" : ""}
                {deltaPercent.toFixed(1)}%
              </span>
            )}
          {subtitle && (
            <span className="text-foreground-secondary">{subtitle}</span>
          )}
        </div>
      ) : null}
    </div>
  );
}
