"use client";

import { useState, useTransition } from "react";
import { TierBuilder } from "./TierBuilder";
import type { LoyaltyProgram, LoyaltyTier } from "@/lib/api/loyalty-api";

interface ProgramConfigFormProps {
  program: LoyaltyProgram | null;
  storeId: string;
  editable: boolean;
  onSave: (data: Record<string, unknown>) => Promise<void>;
}

export function ProgramConfigForm({
  program,
  storeId,
  editable,
  onSave,
}: ProgramConfigFormProps) {
  const [isPending, startTransition] = useTransition();

  const [isActive, setIsActive] = useState(program?.is_active ?? false);
  const [pointsPerDollar, setPointsPerDollar] = useState(
    program?.points_per_dollar ?? "1.00",
  );
  const [pointsCurrency, setPointsCurrency] = useState(
    program?.points_currency ?? "points",
  );
  const [signupBonus, setSignupBonus] = useState(program?.signup_bonus ?? 0);
  const [referralBonus, setReferralBonus] = useState(
    program?.referral_bonus ?? 0,
  );
  const [refereeBonus, setRefereeBonus] = useState(
    program?.referee_bonus ?? 0,
  );
  const [pointExpiryDays, setPointExpiryDays] = useState<number | "">(
    program?.point_expiry_days ?? "",
  );
  const [minRedeemPoints, setMinRedeemPoints] = useState(
    program?.min_redeem_points ?? 100,
  );
  const [pointsValue, setPointsValue] = useState(
    program?.points_value ?? "0.01",
  );
  const [tiers, setTiers] = useState<LoyaltyTier[]>(program?.tiers ?? []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    startTransition(async () => {
      await onSave({
        is_active: isActive,
        points_per_dollar: pointsPerDollar,
        points_currency: pointsCurrency,
        signup_bonus: signupBonus,
        referral_bonus: referralBonus,
        referee_bonus: refereeBonus,
        point_expiry_days: pointExpiryDays === "" ? null : pointExpiryDays,
        min_redeem_points: minRedeemPoints,
        points_value: pointsValue,
        tiers,
      });
    });
  };

  const inputClass =
    "w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]";

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Active toggle */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-[color:var(--ink-900)]">
              Loyalty program
            </h2>
            <p className="text-sm text-ink-600">
              Enable the loyalty program for your store.
            </p>
          </div>
          <label className="relative inline-flex cursor-pointer items-center">
            <input
              type="checkbox"
              checked={isActive}
              onChange={(e) => setIsActive(e.target.checked)}
              disabled={!editable}
              className="peer sr-only"
              aria-label="Toggle loyalty program"
            />
            <div className="h-6 w-11 rounded-md bg-[color:var(--ink-900)]/10 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-md after:bg-white after:transition-all peer-checked:bg-[color:var(--moss-700)] peer-checked:after:translate-x-full" />
          </label>
        </div>
      </div>

      {/* Points configuration */}
      <div className="rounded-[6px] bg-white px-6 py-5 space-y-4">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
          Points configuration
        </h3>
        <hr className="border-t border-ink-200" />
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Points per dollar
            </label>
            <input
              type="text"
              value={pointsPerDollar}
              onChange={(e) => setPointsPerDollar(e.target.value)}
              disabled={!editable}
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Points display name
            </label>
            <input
              type="text"
              value={pointsCurrency}
              onChange={(e) => setPointsCurrency(e.target.value)}
              disabled={!editable}
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Point value (currency)
            </label>
            <input
              type="text"
              value={pointsValue}
              onChange={(e) => setPointsValue(e.target.value)}
              disabled={!editable}
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Min points to redeem
            </label>
            <input
              type="number"
              value={minRedeemPoints}
              onChange={(e) =>
                setMinRedeemPoints(parseInt(e.target.value) || 0)
              }
              disabled={!editable}
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Point expiry (days)
            </label>
            <input
              type="number"
              value={pointExpiryDays}
              onChange={(e) =>
                setPointExpiryDays(
                  e.target.value === "" ? "" : parseInt(e.target.value),
                )
              }
              disabled={!editable}
              placeholder="Never"
              className={`${inputClass} placeholder:text-[color:var(--ink-900)]/30`}
            />
          </div>
        </div>
      </div>

      {/* Bonuses */}
      <div className="rounded-[6px] bg-white px-6 py-5 space-y-4">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
          Bonuses
        </h3>
        <hr className="border-t border-ink-200" />
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Signup bonus
            </label>
            <input
              type="number"
              value={signupBonus}
              onChange={(e) => setSignupBonus(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Referral bonus (referrer)
            </label>
            <input
              type="number"
              value={referralBonus}
              onChange={(e) => setReferralBonus(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Referral bonus (referee)
            </label>
            <input
              type="number"
              value={refereeBonus}
              onChange={(e) => setRefereeBonus(parseInt(e.target.value) || 0)}
              disabled={!editable}
              className={inputClass}
            />
          </div>
        </div>
      </div>

      {/* Tiers */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <TierBuilder value={tiers} onChange={setTiers} disabled={!editable} />
      </div>

      {/* Save */}
      {editable && (
        <div className="flex justify-end">
          <button
            type="submit"
            disabled={isPending}
            className="rounded-[6px] bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
          >
            {isPending ? "Saving..." : "Save program"}
          </button>
        </div>
      )}
    </form>
  );
}
