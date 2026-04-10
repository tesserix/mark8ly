"use client";

import type { LoyaltyReferral } from "@/lib/api/loyalty-api";

interface ReferralsTableProps {
  referrals: LoyaltyReferral[];
  total: number;
}

export function ReferralsTable({ referrals, total }: ReferralsTableProps) {
  if (referrals.length === 0) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          No referrals yet.
        </p>
        <p className="mt-1 text-xs text-[color:var(--ink-900)]/30">
          Referrals will appear here when enrolled members share their codes.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-[6px] bg-white">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/6">
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Referrer
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Referee
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Status
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Bonuses
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Date
            </th>
          </tr>
        </thead>
        <tbody>
          {referrals.map((r) => (
            <tr
              key={r.id}
              className="border-b border-[color:var(--ink-900)]/6 last:border-0"
            >
              <td className="px-4 py-3 text-xs font-mono text-[color:var(--ink-900)]">
                {r.referrer_id.slice(0, 8)}...
              </td>
              <td className="px-4 py-3 text-xs font-mono text-[color:var(--ink-900)]">
                {r.referee_id.slice(0, 8)}...
              </td>
              <td className="px-4 py-3">
                <span
                  className={`inline-block rounded-[4px] px-2 py-0.5 text-xs font-medium capitalize ${
                    r.status === "completed"
                      ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
                      : "bg-[color:var(--ink-900)]/5 text-[color:var(--ink-900)]/50"
                  }`}
                >
                  {r.status}
                </span>
              </td>
              <td className="px-4 py-3 text-xs text-[color:var(--ink-900)]/60">
                +{r.referrer_bonus} / +{r.referee_bonus}
              </td>
              <td className="px-4 py-3 text-xs text-[color:var(--ink-900)]/50">
                {new Date(r.created_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="px-4 py-3 text-xs text-[color:var(--ink-900)]/40">
        {total} referral{total !== 1 ? "s" : ""} total
      </div>
    </div>
  );
}
