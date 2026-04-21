"use client";

import { useEffect, useRef, useState } from "react";
import type {
  LoyaltyProgramPublic,
  CustomerLoyalty,
} from "@/lib/api/loyalty";

interface LoyaltyDashboardProps {
  program: LoyaltyProgramPublic;
  customer: CustomerLoyalty | null;
  storeHost?: string;
}

export function LoyaltyDashboard({
  program,
  customer,
  storeHost,
}: LoyaltyDashboardProps) {
  if (!customer) {
    // Auto-enrollment runs on sign-in and on lazy page visit, so reaching
    // this branch means the program is active but enrollment itself failed
    // (network blip, transient backend error). Surface that instead of a
    // broken "Join" button — the next page load will likely recover.
    return (
      <div className="rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] px-6 py-6">
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))]/70">
          We couldn&apos;t fetch your loyalty balance right now. Refresh in
          a moment and it should appear.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Balance card */}
      <div className="rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] px-6 py-6">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))]/60">
              Your {program.points_currency}
            </p>
            <p className="mt-1 font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-4xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
              {customer.points_balance.toLocaleString()}
            </p>
            <p className="mt-0.5 text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
              Lifetime: {customer.lifetime_points.toLocaleString()}
            </p>
          </div>
          <span className="inline-block rounded-md bg-[color:var(--storefront-accent,var(--moss-700))]/10 px-3 py-1 text-xs font-semibold uppercase tracking-wider text-[color:var(--storefront-accent,var(--moss-700))]">
            {customer.tier}
          </span>
        </div>
      </div>

      {/* Referral card */}
      <ReferralCard
        code={customer.referral_code}
        storeHost={storeHost}
        referralBonus={program.referral_bonus}
        pointsCurrency={program.points_currency}
      />

      {/* Tiers */}
      {program.tiers.length > 0 && (
        <div className="rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] px-6 py-5">
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))]/60">
            Tiers
          </h3>
          <div className="space-y-2">
            {program.tiers.map((tier) => (
              <div
                key={tier.name}
                className={`flex items-center justify-between rounded-[6px] px-4 py-2.5 ${
                  customer.tier === tier.name.toLowerCase()
                    ? "border border-[color:var(--storefront-accent,var(--moss-700))]/20 bg-[color:var(--storefront-accent,var(--moss-700))]/5"
                    : "bg-[color:var(--storefront-background,var(--paper-200))]"
                }`}
              >
                <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                  {tier.name}
                </span>
                <span className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
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

function ReferralCard({
  code,
  storeHost,
  referralBonus,
  pointsCurrency,
}: {
  code: string;
  storeHost?: string;
  referralBonus: number;
  pointsCurrency: string;
}) {
  const [copied, setCopied] = useState<"code" | "link" | null>(null);
  // Track the "Copied" revert timer so rapid consecutive copies don't
  // leak timers and unmount-mid-feedback doesn't leave a stray callback
  // trying to setState on a dead component.
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copiedTimerRef.current !== null) {
        clearTimeout(copiedTimerRef.current);
      }
    };
  }, []);

  // Server passes the live host so the share link is always correct for
  // the tenant the customer is viewing. Fallback to window.location at
  // copy time if the prop is missing.
  const shareLink = storeHost
    ? `https://${storeHost}/?ref=${encodeURIComponent(code)}`
    : "";

  const copy = async (value: string, kind: "code" | "link") => {
    const fallback =
      !value && typeof window !== "undefined"
        ? `${window.location.origin}/?ref=${encodeURIComponent(code)}`
        : value;
    try {
      await navigator.clipboard.writeText(fallback);
      setCopied(kind);
      if (copiedTimerRef.current !== null) {
        clearTimeout(copiedTimerRef.current);
      }
      copiedTimerRef.current = setTimeout(() => {
        setCopied(null);
        copiedTimerRef.current = null;
      }, 1500);
    } catch {
      // Ignore — surface area for toast wiring later.
    }
  };

  return (
    <div className="rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] px-6 py-5 space-y-3">
      <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))]/60">
        Invite friends
      </h3>

      <div className="flex items-center gap-3">
        <code className="rounded-md bg-[color:var(--storefront-background,var(--paper-200))] px-3 py-1.5 font-mono text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          {code}
        </code>
        <button
          type="button"
          onClick={() => copy(code, "code")}
          className="rounded-[6px] px-3 py-1.5 text-xs font-medium text-[color:var(--storefront-accent,var(--moss-700))] transition-colors hover:bg-[color:var(--storefront-accent,var(--moss-700))]/5"
        >
          {copied === "code" ? "Copied" : "Copy code"}
        </button>
      </div>

      {shareLink && (
        <div className="flex items-center gap-3">
          <span className="flex-1 min-w-0 truncate rounded-md bg-[color:var(--storefront-background,var(--paper-200))] px-3 py-1.5 font-mono text-xs text-[color:var(--storefront-text,var(--ink-900))]/70">
            {shareLink}
          </span>
          <button
            type="button"
            onClick={() => copy(shareLink, "link")}
            className="rounded-[6px] px-3 py-1.5 text-xs font-medium text-[color:var(--storefront-accent,var(--moss-700))] transition-colors hover:bg-[color:var(--storefront-accent,var(--moss-700))]/5"
          >
            {copied === "link" ? "Copied" : "Copy link"}
          </button>
        </div>
      )}

      {referralBonus > 0 && (
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
          Share your link and earn {referralBonus} {pointsCurrency} for each
          friend who joins.
        </p>
      )}
    </div>
  );
}
