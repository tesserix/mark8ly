"use client";

import { useState, useTransition, type ReactNode } from "react";
import { TierBuilder } from "./TierBuilder";
import { useToast } from "@/components/feedback/Toaster";
import type { LoyaltyProgram, LoyaltyTier } from "@/lib/api/loyalty-api";

interface ProgramConfigFormProps {
  program: LoyaltyProgram | null;
  storeId: string;
  storeCurrency: string;
  editable: boolean;
  onSave: (
    data: Record<string, unknown>,
  ) => Promise<{ ok: boolean; error?: string }>;
}

// Sensible defaults for a first-time setup — mirror the recommendations
// surfaced by the planner + match industry-standard 1% cashback feel.
const DEFAULTS = {
  isActive: true,
  pointsPerUnit: "1.00",
  pointsCurrency: "points",
  pointsValue: "0.01",
  minRedeemPoints: 100,
  pointExpiryDays: 365,
  signupBonus: 100,
  referralBonus: 0,
  refereeBonus: 0,
} as const;

export function ProgramConfigForm({
  program,
  storeId: _storeId,
  storeCurrency,
  editable,
  onSave,
}: ProgramConfigFormProps) {
  const [isPending, startTransition] = useTransition();
  const { toast } = useToast();

  const isEmpty = program === null;
  const [isEditing, setIsEditing] = useState(false);

  const [isActive, setIsActive] = useState(
    program?.is_active ?? DEFAULTS.isActive,
  );
  const [pointsPerUnit, setPointsPerUnit] = useState(
    program?.points_per_unit ?? DEFAULTS.pointsPerUnit,
  );
  const [pointsCurrency, setPointsCurrency] = useState(
    program?.points_currency ?? DEFAULTS.pointsCurrency,
  );
  const [signupBonus, setSignupBonus] = useState(
    program?.signup_bonus ?? DEFAULTS.signupBonus,
  );
  const [referralBonus, setReferralBonus] = useState(
    program?.referral_bonus ?? DEFAULTS.referralBonus,
  );
  const [refereeBonus, setRefereeBonus] = useState(
    program?.referee_bonus ?? DEFAULTS.refereeBonus,
  );
  const [pointExpiryDays, setPointExpiryDays] = useState<number | "">(
    program?.point_expiry_days ?? DEFAULTS.pointExpiryDays,
  );
  const [minRedeemPoints, setMinRedeemPoints] = useState(
    program?.min_redeem_points ?? DEFAULTS.minRedeemPoints,
  );
  const [pointsValue, setPointsValue] = useState(
    program?.points_value ?? DEFAULTS.pointsValue,
  );
  const [tiers, setTiers] = useState<LoyaltyTier[]>(program?.tiers ?? []);

  const resetToProgram = () => {
    setIsActive(program?.is_active ?? DEFAULTS.isActive);
    setPointsPerUnit(program?.points_per_unit ?? DEFAULTS.pointsPerUnit);
    setPointsCurrency(program?.points_currency ?? DEFAULTS.pointsCurrency);
    setSignupBonus(program?.signup_bonus ?? DEFAULTS.signupBonus);
    setReferralBonus(program?.referral_bonus ?? DEFAULTS.referralBonus);
    setRefereeBonus(program?.referee_bonus ?? DEFAULTS.refereeBonus);
    setPointExpiryDays(program?.point_expiry_days ?? DEFAULTS.pointExpiryDays);
    setMinRedeemPoints(program?.min_redeem_points ?? DEFAULTS.minRedeemPoints);
    setPointsValue(program?.points_value ?? DEFAULTS.pointsValue);
    setTiers(program?.tiers ?? []);
  };

  const handleCancel = () => {
    resetToProgram();
    setIsEditing(false);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    startTransition(async () => {
      const result = await onSave({
        is_active: isActive,
        points_per_unit: pointsPerUnit,
        points_currency: pointsCurrency,
        signup_bonus: signupBonus,
        referral_bonus: referralBonus,
        referee_bonus: refereeBonus,
        point_expiry_days: pointExpiryDays === "" ? null : pointExpiryDays,
        min_redeem_points: minRedeemPoints,
        points_value: pointsValue,
        tiers,
      });
      if (result.ok) {
        toast.success("Loyalty program saved");
        setIsEditing(false);
      } else {
        toast.error("Couldn't save program", result.error);
      }
    });
  };

  // ── Empty state: no program configured yet ────────────────────────
  if (isEmpty && !isEditing) {
    if (!editable) {
      return (
        <div className="rounded-[6px] bg-background-elevated px-6 py-10 text-center">
          <p className="text-sm text-ink-500">
            No loyalty program configured yet.
          </p>
        </div>
      );
    }
    return (
      <div className="rounded-[6px] bg-background-elevated px-6 py-10 text-center">
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
          Set up your loyalty program
        </h2>
        <p className="mx-auto mt-2 max-w-md text-sm text-ink-600">
          Reward repeat customers with points they can redeem at checkout,
          plus signup and referral bonuses. Defaults give you a clean 1%
          back program — adjust any of them before saving.
        </p>
        <button
          type="button"
          onClick={() => setIsEditing(true)}
          className="mt-6 rounded-[6px] bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90"
        >
          Set up loyalty
        </button>
      </div>
    );
  }

  // ── Read mode ─────────────────────────────────────────────────────
  if (!isEditing) {
    return (
      <div className="space-y-6">
        <div className="rounded-[6px] bg-background-elevated px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-[color:var(--ink-900)]">
                Loyalty program
              </h2>
              <div className="mt-2 flex items-center gap-2">
                <StatusBadge active={isActive} />
              </div>
            </div>
            {editable && (
              <button
                type="button"
                onClick={() => setIsEditing(true)}
                className="rounded-[6px] border border-[color:var(--ink-900)]/15 px-4 py-2 text-xs font-medium text-[color:var(--ink-900)] transition-colors hover:bg-[color:var(--paper-200)]"
              >
                Edit
              </button>
            )}
          </div>
        </div>

        <Section title="Points configuration">
          <ReadGrid>
            <ReadField label={`Points per 1 ${storeCurrency} spent`}>
              {pointsPerUnit}
            </ReadField>
            <ReadField label="Points display name">{pointsCurrency}</ReadField>
            <ReadField label={`Value per point (in ${storeCurrency})`}>
              {pointsValue}
            </ReadField>
            <ReadField label="Min points to redeem">
              {minRedeemPoints.toLocaleString()}
            </ReadField>
            <ReadField label="Point expiry">
              {pointExpiryDays === "" ? "Never" : `${pointExpiryDays} days`}
            </ReadField>
          </ReadGrid>
        </Section>

        <Section title="Bonuses">
          <ReadGrid>
            <ReadField label="Signup bonus">
              {signupBonus.toLocaleString()} {pointsCurrency}
            </ReadField>
            <ReadField label="Referral bonus (referrer)">
              {referralBonus.toLocaleString()} {pointsCurrency}
            </ReadField>
            <ReadField label="Referral bonus (referee)">
              {refereeBonus.toLocaleString()} {pointsCurrency}
            </ReadField>
          </ReadGrid>
        </Section>

        <Section title="Tiers">
          {tiers.length === 0 ? (
            <p className="text-sm text-ink-500">No tiers configured.</p>
          ) : (
            <ul className="divide-y divide-ink-200">
              {tiers.map((t) => (
                <li
                  key={t.name}
                  className="flex items-center justify-between py-2.5 text-sm"
                >
                  <span className="font-medium text-[color:var(--ink-900)] capitalize">
                    {t.name}
                  </span>
                  <span className="text-ink-600">
                    {t.min_points.toLocaleString()} pts &middot; {t.multiplier}
                    x
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Section>
      </div>
    );
  }

  // ── Edit mode (form) ──────────────────────────────────────────────
  const inputClass =
    "w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-background-elevated px-3 py-2 text-sm text-[color:var(--ink-900)] focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]";

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Active toggle */}
      <div className="rounded-[6px] bg-background-elevated px-6 py-5">
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
              className="peer sr-only"
              aria-label="Toggle loyalty program"
            />
            <div className="h-6 w-11 rounded-md bg-[color:var(--ink-900)]/10 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-md after:bg-background-elevated after:transition-all peer-checked:bg-[color:var(--moss-700)] peer-checked:after:translate-x-full" />
          </label>
        </div>
      </div>

      {/* Points configuration */}
      <Section title="Points configuration">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Points per 1 {storeCurrency} spent
            </label>
            <input
              type="text"
              value={pointsPerUnit}
              onChange={(e) => setPointsPerUnit(e.target.value)}
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
              className={inputClass}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Value per point (in {storeCurrency})
            </label>
            <input
              type="text"
              value={pointsValue}
              onChange={(e) => setPointsValue(e.target.value)}
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
              placeholder="Never"
              className={`${inputClass} placeholder:text-[color:var(--ink-900)]/30`}
            />
          </div>
        </div>
      </Section>

      {/* Bonuses */}
      <Section title="Bonuses">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-ink-600">
              Signup bonus
            </label>
            <input
              type="number"
              value={signupBonus}
              onChange={(e) => setSignupBonus(parseInt(e.target.value) || 0)}
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
              className={inputClass}
            />
          </div>
        </div>
      </Section>

      {/* Tiers */}
      <div className="rounded-[6px] bg-background-elevated px-6 py-5">
        <TierBuilder value={tiers} onChange={setTiers} disabled={false} />
      </div>

      {/* Save / Cancel */}
      <div className="flex items-center justify-end gap-3">
        <button
          type="button"
          onClick={handleCancel}
          disabled={isPending}
          className="rounded-[6px] border border-[color:var(--ink-900)]/15 px-6 py-2.5 text-sm font-medium text-[color:var(--ink-900)] transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={isPending}
          className="rounded-[6px] bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
        >
          {isPending ? "Saving..." : "Save program"}
        </button>
      </div>
    </form>
  );
}

// ── Layout helpers ────────────────────────────────────────────────────

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded-[6px] bg-background-elevated px-6 py-5 space-y-4">
      <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
        {title}
      </h3>
      <hr className="border-t border-ink-200" />
      {children}
    </div>
  );
}

function ReadGrid({ children }: { children: ReactNode }) {
  return (
    <dl className="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
      {children}
    </dl>
  );
}

function ReadField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-xs font-medium text-ink-600">{label}</dt>
      <dd className="text-sm text-[color:var(--ink-900)]">{children}</dd>
    </div>
  );
}

function StatusBadge({ active }: { active: boolean }) {
  return (
    <span
      className={`inline-block rounded-md px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wider ${
        active
          ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
          : "bg-[color:var(--ink-900)]/5 text-ink-500"
      }`}
    >
      {active ? "Active" : "Inactive"}
    </span>
  );
}
