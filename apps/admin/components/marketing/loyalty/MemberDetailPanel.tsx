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
    <div className="space-y-10">
      {/* Member masthead — editorial, no boxed card */}
      <section className="space-y-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
              {member.customer_name ?? member.customer_email}
            </h2>
            {member.customer_name && (
              <p className="mt-1 text-sm text-foreground-secondary">
                {member.customer_email}
              </p>
            )}
          </div>
          <span className="inline-flex items-center gap-2 rounded-full bg-[color:var(--moss-700)]/10 px-3 py-1 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)]">
            <span
              aria-hidden="true"
              className="h-1.5 w-1.5 rounded-full bg-[color:var(--moss-700)]"
            />
            {member.tier}
          </span>
        </div>

        <div className="grid grid-cols-1 gap-6 border-t border-border-subtle pt-5 sm:grid-cols-3">
          <div className="sm:col-span-2">
            <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
              Points balance
            </p>
            <p className="mt-2 font-serif text-3xl font-medium tabular-nums text-foreground">
              {member.points_balance.toLocaleString()}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
              Lifetime points
            </p>
            <p className="mt-2 font-serif text-3xl font-medium tabular-nums text-foreground-secondary">
              {member.lifetime_points.toLocaleString()}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
              Enrolled
            </p>
            <p className="mt-2 text-sm text-foreground">
              {new Date(member.enrolled_at).toLocaleDateString()}
            </p>
          </div>
        </div>
      </section>

      {/* Adjust points — hairline section */}
      {editable && (
        <section className="space-y-4 border-t border-border-subtle pt-10">
          <div className="space-y-1">
            <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
              Adjust points
            </h3>
            <p className="text-sm text-foreground-secondary">
              Credit or debit this member&rsquo;s balance with a reason on record.
            </p>
          </div>

          <form onSubmit={handleAdjust} className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <label
                  htmlFor="loyalty-adjust-points"
                  className="text-xs font-medium text-foreground-secondary"
                >
                  Points (positive to credit, negative to debit)
                </label>
                <input
                  id="loyalty-adjust-points"
                  type="number"
                  value={adjustPoints}
                  onChange={(e) => setAdjustPoints(e.target.value)}
                  placeholder="e.g. 100 or -50"
                  aria-invalid={adjustError ? true : undefined}
                  className="w-full rounded-md border border-[color:var(--ink-900)]/10 bg-background-elevated px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
                />
              </div>
              <div className="space-y-1.5">
                <label
                  htmlFor="loyalty-adjust-reason"
                  className="text-xs font-medium text-foreground-secondary"
                >
                  Reason
                </label>
                <input
                  id="loyalty-adjust-reason"
                  type="text"
                  value={adjustDescription}
                  onChange={(e) => setAdjustDescription(e.target.value)}
                  placeholder="e.g. Customer support goodwill"
                  aria-invalid={adjustError ? true : undefined}
                  className="w-full rounded-md border border-[color:var(--ink-900)]/10 bg-background-elevated px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
                />
              </div>
            </div>
            {adjustError && (
              <p
                role="alert"
                aria-live="polite"
                className="text-xs text-[color:var(--danger)]"
              >
                {adjustError}
              </p>
            )}
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex items-center rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
            >
              {isPending ? "Adjusting…" : "Adjust points"}
            </button>
          </form>
        </section>
      )}

      {/* Transaction history — hairline section */}
      <section className="space-y-4 border-t border-border-subtle pt-10">
        <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          Transaction history
        </h3>

        {transactions.length === 0 ? (
          <p className="py-4 text-sm text-foreground-secondary">
            No transactions yet.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-y border-border-subtle text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3 text-right">Points</th>
                  <th className="px-4 py-3 text-right">Balance</th>
                  <th className="px-4 py-3">Description</th>
                  <th className="px-4 py-3">Date</th>
                </tr>
              </thead>
              <tbody>
                {transactions.map((tx) => (
                  <tr
                    key={tx.id}
                    className="border-b border-border-subtle last:border-0"
                  >
                    <td className="px-4 py-3">
                      <span
                        className={`inline-block rounded-md px-2 py-0.5 text-xs font-medium capitalize ${
                          tx.type === "earn" ||
                          tx.type === "credit" ||
                          tx.type === "signup_bonus" ||
                          tx.type === "referral_bonus"
                            ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
                            : tx.type === "redeem" ||
                                tx.type === "debit" ||
                                tx.type === "expiry"
                              ? "bg-[color:var(--danger)]/10 text-[color:var(--danger)]"
                              : "bg-[color:var(--ink-900)]/5 text-foreground-tertiary"
                        }`}
                      >
                        {tx.type.replace(/_/g, " ")}
                      </span>
                    </td>
                    <td
                      className={`px-4 py-3 text-right font-serif tabular-nums ${
                        tx.points > 0
                          ? "text-[color:var(--moss-700)]"
                          : "text-foreground"
                      }`}
                    >
                      {tx.points > 0 ? "+" : ""}
                      {tx.points.toLocaleString()}
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-foreground-secondary">
                      {tx.balance_after.toLocaleString()}
                    </td>
                    <td className="px-4 py-3 text-xs text-foreground-secondary">
                      {tx.description ?? "—"}
                    </td>
                    <td className="px-4 py-3 text-xs tabular-nums text-foreground-tertiary">
                      {new Date(tx.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
