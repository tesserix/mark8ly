"use client";

import type { LoyaltyTier } from "@/lib/api/loyalty-api";

interface TierBuilderProps {
  value: LoyaltyTier[];
  onChange: (tiers: LoyaltyTier[]) => void;
  disabled?: boolean;
}

const MAX_TIERS = 4;

export function TierBuilder({ value, onChange, disabled }: TierBuilderProps) {
  const addTier = () => {
    if (value.length >= MAX_TIERS) return;
    const lastMinPoints =
      value.length > 0 ? value[value.length - 1]!.min_points + 500 : 0;
    onChange([
      ...value,
      { name: "", min_points: lastMinPoints, multiplier: "1.0" },
    ]);
  };

  const updateTier = (
    index: number,
    field: keyof LoyaltyTier,
    fieldValue: string | number,
  ) => {
    const updated = value.map((tier, i) =>
      i === index ? { ...tier, [field]: fieldValue } : tier,
    );
    onChange(updated);
  };

  const removeTier = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
          Tiers ({value.length}/{MAX_TIERS})
        </h3>
        {!disabled && value.length < MAX_TIERS && (
          <button
            type="button"
            onClick={addTier}
            className="rounded-[6px] bg-[color:var(--ink-900)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90"
          >
            Add tier
          </button>
        )}
      </div>

      {value.length === 0 && (
        <p className="text-sm text-ink-500">
          No tiers configured. All members earn at 1x rate.
        </p>
      )}

      <div className="space-y-3">
        {value.map((tier, index) => (
          <div
            key={index}
            className="flex items-end gap-3 rounded-[6px] bg-[color:var(--paper-200)] px-4 py-3"
          >
            <div className="flex-1 space-y-1">
              <label className="text-xs font-medium text-ink-600">
                Name
              </label>
              <input
                type="text"
                value={tier.name}
                onChange={(e) => updateTier(index, "name", e.target.value)}
                disabled={disabled}
                placeholder="e.g. Silver"
                className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2.5 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
              />
            </div>
            <div className="w-32 space-y-1">
              <label className="text-xs font-medium text-ink-600">
                Min points
              </label>
              <input
                type="number"
                value={tier.min_points}
                onChange={(e) =>
                  updateTier(index, "min_points", parseInt(e.target.value) || 0)
                }
                disabled={disabled}
                min={0}
                className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2.5 text-sm text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
              />
            </div>
            <div className="w-28 space-y-1">
              <label className="text-xs font-medium text-ink-600">
                Multiplier
              </label>
              <input
                type="text"
                value={tier.multiplier}
                onChange={(e) =>
                  updateTier(index, "multiplier", e.target.value)
                }
                disabled={disabled}
                placeholder="1.5"
                className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2.5 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
              />
            </div>
            {!disabled && (
              <button
                type="button"
                onClick={() => removeTier(index)}
                className="mb-0.5 rounded-[6px] px-2 py-1.5 text-xs text-ink-500 transition-colors hover:bg-[color:var(--ink-900)]/5 hover:text-[color:var(--ink-900)]/70"
                aria-label={`Remove tier ${tier.name || `tier ${index + 1}`}`}
              >
                Remove
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
