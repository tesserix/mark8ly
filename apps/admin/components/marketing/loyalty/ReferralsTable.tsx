"use client";

import type { LoyaltyReferral } from "@/lib/api/loyalty-api";

interface ReferralsTableProps {
  referrals: LoyaltyReferral[];
  total: number;
}

export function ReferralsTable({ referrals, total }: ReferralsTableProps) {
  if (referrals.length === 0) {
    return (
      <div className="flex flex-col items-start gap-2 border-t border-border-subtle py-12">
        <p className="font-serif text-xl font-medium text-foreground">
          No referrals yet
        </p>
        <p className="max-w-prose text-sm text-foreground-secondary">
          Referrals will appear here when enrolled members share their codes.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-[color:var(--ink-900)]/15 text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
              <th className="pb-3 pr-4">Referrer</th>
              <th className="pb-3 pr-4">Referee</th>
              <th className="pb-3 pr-4">Status</th>
              <th className="pb-3 pr-4">Bonuses</th>
              <th className="pb-3 pr-4">Date</th>
            </tr>
          </thead>
          <tbody>
            {referrals.map((r) => (
              <tr
                key={r.id}
                className="border-b border-[color:var(--ink-900)]/10 transition-colors hover:bg-[color:var(--ink-900)]/[0.03]"
              >
                <td className="py-3 pr-4 font-mono text-xs text-foreground">
                  {r.referrer_id.slice(0, 8)}…
                </td>
                <td className="py-3 pr-4 font-mono text-xs text-foreground">
                  {r.referee_id.slice(0, 8)}…
                </td>
                <td className="py-3 pr-4">
                  <span className="inline-flex items-center gap-2 text-sm text-foreground">
                    <span
                      aria-hidden="true"
                      className={
                        "inline-block h-2 w-2 rounded-full " +
                        (r.status === "completed"
                          ? "bg-[color:var(--moss-700)]"
                          : "border border-[color:var(--ink-900)]/30 bg-transparent")
                      }
                    />
                    <span className="capitalize">{r.status}</span>
                  </span>
                </td>
                <td className="py-3 pr-4 tabular-nums text-foreground-secondary">
                  +{r.referrer_bonus} / +{r.referee_bonus}
                </td>
                <td className="py-3 pr-4 text-xs tabular-nums text-foreground-tertiary">
                  {new Date(r.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs tabular-nums text-foreground-tertiary">
        {total} referral{total !== 1 ? "s" : ""} total
      </p>
    </div>
  );
}
