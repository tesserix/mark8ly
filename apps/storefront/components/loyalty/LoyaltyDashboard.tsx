"use client";

import type {
  LoyaltyProgramPublic,
  CustomerLoyalty,
} from "@/lib/api/loyalty";

interface LoyaltyDashboardProps {
  program: LoyaltyProgramPublic;
  customer: CustomerLoyalty | null;
  onEnroll?: () => void;
}

export function LoyaltyDashboard({
  program,
  customer,
  onEnroll,
}: LoyaltyDashboardProps) {
  if (!customer) {
    return (
      <div className="space-y-6">
        <div className="rounded-[6px] bg-white px-6 py-8 text-left">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
            Join our loyalty program
          </h2>
          <p className="mt-2 text-sm text-ink-600">
            Earn {program.points_currency} on every purchase and unlock
            exclusive rewards.
          </p>
          {program.signup_bonus > 0 && (
            <p className="mt-1 text-sm font-medium text-[color:var(--moss-700)]">
              Get {program.signup_bonus} {program.points_currency} just for
              joining!
            </p>
          )}
          {onEnroll && (
            <button
              onClick={onEnroll}
              className="mt-4 rounded-[6px] bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90"
            >
              Join now
            </button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Balance card */}
      <div className="rounded-[6px] bg-white px-6 py-6">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
              Your {program.points_currency}
            </p>
            <p className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium text-[color:var(--ink-900)]">
              {customer.points_balance.toLocaleString()}
            </p>
            <p className="mt-0.5 text-xs text-ink-500">
              Lifetime: {customer.lifetime_points.toLocaleString()}
            </p>
          </div>
          <span className="inline-block rounded-md bg-[color:var(--moss-700)]/10 px-3 py-1 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)]">
            {customer.tier}
          </span>
        </div>
      </div>

      {/* Referral card */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
          Your referral code
        </h3>
        <div className="mt-2 flex items-center gap-3">
          <code className="rounded-md bg-[color:var(--paper-200)] px-3 py-1.5 font-mono text-sm font-medium text-[color:var(--ink-900)]">
            {customer.referral_code}
          </code>
          <button
            onClick={() => navigator.clipboard.writeText(customer.referral_code)}
            className="rounded-[6px] px-3 py-1.5 text-xs font-medium text-[color:var(--moss-700)] transition-colors hover:bg-[color:var(--moss-700)]/5"
          >
            Copy
          </button>
        </div>
        {program.referral_bonus > 0 && (
          <p className="mt-2 text-xs text-ink-500">
            Share this code and earn {program.referral_bonus}{" "}
            {program.points_currency} for each friend who joins.
          </p>
        )}
      </div>

      {/* Tiers */}
      {program.tiers.length > 0 && (
        <div className="rounded-[6px] bg-white px-6 py-5">
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
            Tiers
          </h3>
          <div className="space-y-2">
            {program.tiers.map((tier) => (
              <div
                key={tier.name}
                className={`flex items-center justify-between rounded-[6px] px-4 py-2.5 ${
                  customer.tier === tier.name.toLowerCase()
                    ? "border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5"
                    : "bg-[color:var(--paper-200)]"
                }`}
              >
                <span className="text-sm font-medium text-[color:var(--ink-900)]">
                  {tier.name}
                </span>
                <span className="text-xs text-ink-500">
                  {tier.min_points.toLocaleString()} pts &middot;{" "}
                  {tier.multiplier}x
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
