"use client";

import { useState, useTransition } from "react";
import { useToast } from "@/components/feedback/Toaster";
import type { LoyaltyMember, LoyaltyTransaction } from "@/lib/api/loyalty-api";

interface MemberDetailPanelProps {
  member: LoyaltyMember;
  transactions: LoyaltyTransaction[];
  editable: boolean;
  onAdjust: (points: number, description: string) => Promise<boolean>;
}

export function MemberDetailPanel({
  member,
  transactions,
  editable,
  onAdjust,
}: MemberDetailPanelProps) {
  const { toast } = useToast();
  const [isPending, startTransition] = useTransition();
  const [adjustPoints, setAdjustPoints] = useState<string>("");
  const [adjustDescription, setAdjustDescription] = useState("");
  const [adjustError, setAdjustError] = useState<string | null>(null);

  const handleAdjust = (e: React.FormEvent) => {
    e.preventDefault();
    const points = parseInt(adjustPoints);
    if (isNaN(points) || points === 0) {
      setAdjustError("Enter a non-zero point amount.");
      return;
    }
    if (!adjustDescription.trim()) {
      setAdjustError("Enter a reason for the adjustment.");
      return;
    }
    setAdjustError(null);

    startTransition(async () => {
      const ok = await onAdjust(points, adjustDescription.trim());
      if (ok) {
        setAdjustPoints("");
        setAdjustDescription("");
        toast.success(
          `${points > 0 ? "+" : ""}${points} points ${points > 0 ? "credited" : "debited"}`,
        );
      } else {
        const msg = "Failed to adjust points. Please try again.";
        setAdjustError(msg);
        toast.error("Couldn't adjust points", msg);
      }
    });
  };

  return (
    <div className="space-y-6">
      {/* Member info */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <div className="flex items-start justify-between">
          <div>
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-xl font-medium text-[color:var(--ink-900)]">
              {member.customer_name ?? member.customer_email}
            </h2>
            {member.customer_name && (
              <p className="mt-0.5 text-sm text-ink-600">
                {member.customer_email}
              </p>
            )}
          </div>
          <span className="inline-block rounded-md bg-[color:var(--moss-700)]/10 px-3 py-1 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)]">
            {member.tier}
          </span>
        </div>

        <hr className="my-4 border-t border-ink-200" />

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
          <div className="sm:col-span-2">
            <p className="text-xs font-medium text-ink-500">
              Points balance
            </p>
            <p className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
              {member.points_balance.toLocaleString()}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium text-ink-500">
              Lifetime points
            </p>
            <p className="mt-1 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-ink-600">
              {member.lifetime_points.toLocaleString()}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium text-ink-500">
              Enrolled
            </p>
            <p className="mt-1 text-sm text-[color:var(--ink-900)]">
              {new Date(member.enrolled_at).toLocaleDateString()}
            </p>
          </div>
        </div>
      </div>

      {/* Adjust points form */}
      {editable && (
        <div className="rounded-[6px] bg-white px-6 py-5 space-y-4">
          <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
            Adjust points
          </h3>
          <hr className="border-t border-ink-200" />
          <form onSubmit={handleAdjust} className="space-y-3">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1">
                <label className="text-xs font-medium text-ink-600">
                  Points (positive to credit, negative to debit)
                </label>
                <input
                  type="number"
                  value={adjustPoints}
                  onChange={(e) => setAdjustPoints(e.target.value)}
                  placeholder="e.g. 100 or -50"
                  className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
                />
              </div>
              <div className="space-y-1">
                <label className="text-xs font-medium text-ink-600">
                  Reason
                </label>
                <input
                  type="text"
                  value={adjustDescription}
                  onChange={(e) => setAdjustDescription(e.target.value)}
                  placeholder="e.g. Customer support goodwill"
                  className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
                />
              </div>
            </div>
            {adjustError && (
              <p role="alert" aria-live="polite" className="text-xs text-[color:var(--danger,#8B2500)]">
                {adjustError}
              </p>
            )}
            <button
              type="submit"
              disabled={isPending}
              className="rounded-[6px] bg-[color:var(--ink-900)] px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
            >
              {isPending ? "Adjusting..." : "Adjust points"}
            </button>
          </form>
        </div>
      )}

      {/* Transaction history */}
      <div className="rounded-[6px] bg-white">
        <div className="px-6 py-4">
          <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-ink-500">
            Transaction history
          </h3>
        </div>
        {transactions.length === 0 ? (
          <div className="px-6 pb-6">
            <p className="text-sm text-ink-500">
              No transactions yet.
            </p>
          </div>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-y border-ink-200">
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  Type
                </th>
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  Points
                </th>
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  Balance
                </th>
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  Description
                </th>
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  Date
                </th>
              </tr>
            </thead>
            <tbody>
              {transactions.map((tx) => (
                <tr
                  key={tx.id}
                  className="border-b border-ink-200 last:border-0"
                >
                  <td className="px-4 py-3">
                    <span
                      className={`inline-block rounded-md px-2 py-0.5 text-xs font-medium capitalize ${
                        tx.type === "earn" || tx.type === "credit" || tx.type === "signup_bonus" || tx.type === "referral_bonus"
                          ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
                          : tx.type === "redeem" || tx.type === "debit" || tx.type === "expiry"
                            ? "bg-[color:var(--danger,#8B2500)]/10 text-[color:var(--danger,#8B2500)]"
                            : "bg-[color:var(--ink-900)]/5 text-ink-500"
                      }`}
                    >
                      {tx.type.replace(/_/g, " ")}
                    </span>
                  </td>
                  <td
                    className={`px-4 py-3 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] ${
                      tx.points > 0
                        ? "text-[color:var(--moss-700)]"
                        : "text-[color:var(--ink-900)]"
                    }`}
                  >
                    {tx.points > 0 ? "+" : ""}
                    {tx.points.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-ink-600">
                    {tx.balance_after.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-ink-600 text-xs">
                    {tx.description ?? "--"}
                  </td>
                  <td className="px-4 py-3 text-xs text-ink-500">
                    {new Date(tx.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
