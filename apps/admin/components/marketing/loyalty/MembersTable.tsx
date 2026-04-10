"use client";

import Link from "next/link";
import type { LoyaltyMember } from "@/lib/api/loyalty-api";

interface MembersTableProps {
  members: LoyaltyMember[];
  total: number;
}

export function MembersTable({ members, total }: MembersTableProps) {
  if (members.length === 0) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          No members enrolled yet.
        </p>
        <p className="mt-1 text-xs text-[color:var(--ink-900)]/30">
          Members will appear here once customers enroll in the loyalty program.
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
              Email
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Points
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Lifetime
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Tier
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              Enrolled
            </th>
          </tr>
        </thead>
        <tbody>
          {members.map((m) => (
            <tr
              key={m.id}
              className="border-b border-[color:var(--ink-900)]/6 last:border-0 transition-colors hover:bg-[color:var(--paper-200)]/50"
            >
              <td className="px-4 py-3 text-[color:var(--ink-900)]">
                <Link
                  href={`/marketing/loyalty/members/${m.id}`}
                  className="text-[color:var(--moss-700)] hover:underline"
                >
                  {m.customer_email}
                </Link>
              </td>
              <td className="px-4 py-3 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-[color:var(--ink-900)]">
                {m.points_balance.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-[color:var(--ink-900)]/60">
                {m.lifetime_points.toLocaleString()}
              </td>
              <td className="px-4 py-3">
                <span className="inline-block rounded-[4px] bg-[color:var(--moss-700)]/10 px-2 py-0.5 text-xs font-medium capitalize text-[color:var(--moss-700)]">
                  {m.tier}
                </span>
              </td>
              <td className="px-4 py-3 text-xs text-[color:var(--ink-900)]/50">
                {new Date(m.enrolled_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="px-4 py-3 text-xs text-[color:var(--ink-900)]/40">
        {total} member{total !== 1 ? "s" : ""} total
      </div>
    </div>
  );
}
