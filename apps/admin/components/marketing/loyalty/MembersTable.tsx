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
      <div className="flex flex-col items-start gap-2 border-t border-border-subtle py-12">
        <p className="font-serif text-xl font-medium text-foreground">
          No members enrolled yet
        </p>
        <p className="max-w-prose text-sm text-foreground-secondary">
          Members will appear here once customers enroll in the loyalty program.
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
              <th className="pb-3 pr-4">Email</th>
              <th className="pb-3 pr-4 text-right">Points</th>
              <th className="pb-3 pr-4 text-right">Lifetime</th>
              <th className="pb-3 pr-4">Tier</th>
              <th className="pb-3 pr-4">Enrolled</th>
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr
                key={m.id}
                className="group border-b border-[color:var(--ink-900)]/10 transition-colors hover:bg-[color:var(--ink-900)]/[0.03]"
              >
                <td className="py-3 pr-4">
                  <Link
                    href={`/marketing/loyalty/members/${m.id}`}
                    className="text-foreground underline-offset-4 transition-colors group-hover:text-[color:var(--moss-700)] group-hover:underline"
                  >
                    {m.customer_email}
                  </Link>
                </td>
                <td className="py-3 pr-4 text-right font-serif text-base tabular-nums text-foreground">
                  {m.points_balance.toLocaleString()}
                </td>
                <td className="py-3 pr-4 text-right tabular-nums text-foreground-secondary">
                  {m.lifetime_points.toLocaleString()}
                </td>
                <td className="py-3 pr-4">
                  <span className="inline-flex items-center gap-2 text-sm text-foreground">
                    <span
                      aria-hidden="true"
                      className="inline-block h-2 w-2 rounded-full bg-[color:var(--moss-700)]"
                    />
                    <span className="capitalize">{m.tier}</span>
                  </span>
                </td>
                <td className="py-3 pr-4 text-xs tabular-nums text-foreground-tertiary">
                  {new Date(m.enrolled_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs tabular-nums text-foreground-tertiary">
        {total} member{total !== 1 ? "s" : ""} total
      </p>
    </div>
  );
}
