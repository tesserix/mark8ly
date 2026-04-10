"use client";

import { useState, useTransition } from "react";
import { formatDistanceToNow, format } from "date-fns";
import { CreditCard, ArrowRight, AlertTriangle } from "lucide-react";

import type { StoreSubscription } from "@/lib/api/settings-tier2-api";
import { createCheckout, createPortal } from "@/app/settings/actions";

interface SubscriptionSettingsClientProps {
  subscription: StoreSubscription | null;
  editable: boolean;
}

const PLAN_LABELS: Record<string, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  enterprise: "Enterprise",
};

const STATUS_STYLES: Record<string, string> = {
  active: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  trialing: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  past_due: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  cancelled: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
  incomplete: "bg-[color:var(--warning)]/10 text-[color:var(--warning)]",
};

const PLAN_PRICES: Record<string, string> = {
  free: "$0",
  starter: "$9.99",
  pro: "$29.99",
  enterprise: "$99.99",
};

const PLAN_PRICES_ANNUAL: Record<string, string> = {
  free: "$0",
  starter: "$7.99",
  pro: "$24.99",
  enterprise: "$83.99",
};

type FeatureRow = {
  feature: string;
  free: string;
  starter: string;
  pro: string;
  enterprise: string;
};

const PLAN_FEATURES: FeatureRow[] = [
  { feature: "Products", free: "25", starter: "500", pro: "Unlimited", enterprise: "Unlimited" },
  { feature: "Categories", free: "5", starter: "25", pro: "Unlimited", enterprise: "Unlimited" },
  { feature: "Staff members", free: "1", starter: "3", pro: "10", enterprise: "Unlimited" },
  { feature: "Stores", free: "1", starter: "1", pro: "3", enterprise: "10" },
  { feature: "Orders / month", free: "50", starter: "500", pro: "Unlimited", enterprise: "Unlimited" },
  { feature: "Transaction fees", free: "0%", starter: "0%", pro: "0%", enterprise: "0%" },
  { feature: "Full color palette", free: "---", starter: "\u2713", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Announcement bar", free: "---", starter: "\u2713", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Remove powered-by", free: "---", starter: "---", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Custom CSS", free: "---", starter: "---", pro: "---", enterprise: "\u2713" },
  { feature: "Custom domain", free: "---", starter: "---", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Active coupons", free: "5", starter: "50", pro: "Unlimited", enterprise: "Unlimited" },
  { feature: "Gift cards", free: "---", starter: "\u2713", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Loyalty program", free: "---", starter: "---", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Campaigns / month", free: "---", starter: "5", pro: "50", enterprise: "Unlimited" },
  { feature: "Reviews", free: "---", starter: "\u2713", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Support tickets", free: "---", starter: "\u2713", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Priority support", free: "---", starter: "---", pro: "---", enterprise: "\u2713" },
  { feature: "Audit logs", free: "---", starter: "---", pro: "\u2713", enterprise: "\u2713" },
  { feature: "CSV import/export", free: "---", starter: "\u2713", pro: "\u2713", enterprise: "\u2713" },
  { feature: "Mobile app", free: "---", starter: "---", pro: "---", enterprise: "\u2713" },
];

export function SubscriptionSettingsClient({
  subscription,
  editable,
}: SubscriptionSettingsClientProps) {
  return (
    <div className="space-y-10">
      {subscription?.status === "past_due" && <PastDueWarning />}
      <CurrentPlanCard subscription={subscription} editable={editable} />
      <hr className="border-border" />
      <PlanComparison currentPlan={subscription?.plan ?? "free"} editable={editable} />
    </div>
  );
}

// ─── Past Due Warning ─────────────────────────────────────────────────

function PastDueWarning() {
  return (
    <div className="flex items-start gap-3 rounded-[6px] border border-[color:var(--signal)]/30 bg-[color:var(--signal)]/5 p-4">
      <AlertTriangle className="mt-0.5 h-5 w-5 text-[color:var(--signal)]" aria-hidden="true" />
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">Payment past due</p>
        <p className="text-sm text-foreground-secondary">
          Your last payment failed. Please update your billing details to avoid service interruption.
        </p>
      </div>
    </div>
  );
}

// ─── Current Plan Card ────────────────────────────────────────────────

function CurrentPlanCard({
  subscription,
  editable,
}: {
  subscription: StoreSubscription | null;
  editable: boolean;
}) {
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const plan = subscription?.plan ?? "free";
  const status = subscription?.status ?? "active";

  function handleManageBilling() {
    setError(null);
    startTransition(async () => {
      const result = await createPortal();
      if (!result.ok) {
        setError(result.message);
      } else {
        window.location.href = result.data.portal_url;
      }
    });
  }

  return (
    <section className="space-y-6">
      <div className="flex items-start gap-3">
        <CreditCard className="mt-0.5 h-5 w-5 text-foreground-secondary" aria-hidden="true" />
        <div className="space-y-1">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
            Current plan
          </h2>
          <p className="text-sm text-foreground-secondary">
            Your current subscription and billing period.
          </p>
        </div>
      </div>
      <div className="space-y-4">
        <div className="flex items-baseline gap-3">
          <span className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-foreground">
            {PLAN_LABELS[plan] ?? plan}
          </span>
          <span
            className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_STYLES[status] ?? STATUS_STYLES.active}`}
          >
            {status.replace("_", " ")}
          </span>
        </div>
        {subscription?.current_period_start && subscription?.current_period_end && (
          <p className="text-sm text-foreground-secondary">
            {format(new Date(subscription.current_period_start), "MMM d, yyyy")}
            {" -- "}
            {format(new Date(subscription.current_period_end), "MMM d, yyyy")}
          </p>
        )}
        {subscription?.cancel_at_period_end && (
          <p className="text-sm text-[color:var(--warning)]">
            Cancels at end of current period
          </p>
        )}
        {error && <p role="alert" className="text-sm text-[color:var(--signal)]">{error}</p>}
        {editable && plan !== "free" && (
          <button
            type="button"
            onClick={handleManageBilling}
            disabled={isPending}
            className="h-10 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-5 text-sm font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
          >
            {isPending ? "Redirecting..." : "Manage billing"}
          </button>
        )}
      </div>
    </section>
  );
}

// ─── Plan Comparison Grid ─────────────────────────────────────────────

function PlanComparison({
  currentPlan,
  editable,
}: {
  currentPlan: string;
  editable: boolean;
}) {
  const plans = ["free", "starter", "pro", "enterprise"] as const;
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  function handleChangePlan(plan: string) {
    setError(null);
    startTransition(async () => {
      const result = await createCheckout(plan);
      if (!result.ok) {
        setError(result.message);
      } else {
        window.location.href = result.data.checkout_url;
      }
    });
  }

  return (
    <section className="space-y-6">
      <div className="space-y-1">
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
          Compare plans
        </h2>
        <p className="text-sm text-foreground-secondary">
          0% transaction fees on every plan. Your revenue is yours.
        </p>
      </div>
      {error && <p role="alert" className="text-sm text-[color:var(--signal)]">{error}</p>}
      <div className="hidden sm:block overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="pb-3 pr-4 text-left font-medium text-foreground-secondary" />
              {plans.map((p) => (
                <th
                  key={p}
                  className={`pb-3 px-4 text-center ${
                    p === currentPlan ? "text-[color:var(--moss-700)]" : "text-foreground-secondary"
                  }`}
                >
                  <div className="space-y-1">
                    <span className="text-sm font-medium">
                      {PLAN_LABELS[p]}
                      {p === currentPlan && (
                        <span className="ml-1 text-xs font-normal">(current)</span>
                      )}
                    </span>
                    <div className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-xl font-medium text-foreground">
                      {PLAN_PRICES[p]}
                      <span className="text-xs font-normal text-foreground-secondary">/mo</span>
                    </div>
                    <div className="text-xs text-foreground-tertiary">
                      {PLAN_PRICES_ANNUAL[p]}/mo billed annually
                    </div>
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {PLAN_FEATURES.map((row) => (
              <tr key={row.feature} className="border-b border-border-subtle">
                <td className="py-2.5 pr-4 text-foreground">{row.feature}</td>
                {plans.map((p) => {
                  const val = row[p];
                  const isCheck = val === "\u2713";
                  const isDash = val === "---";
                  return (
                    <td key={p} className="py-2.5 px-4 text-center">
                      {isCheck ? (
                        <span className="text-[color:var(--moss-700)]" aria-label="Included">{val}</span>
                      ) : isDash ? (
                        <span className="text-[color:var(--ink-900)]/20" aria-label="Not included">{val}</span>
                      ) : (
                        <span className="text-foreground tabular-nums">{val}</span>
                      )}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
          {editable && (
            <tfoot>
              <tr>
                <td className="pt-5" />
                {plans.map((p) => (
                  <td key={p} className="pt-5 px-4 text-center">
                    {p === currentPlan ? (
                      <span className="text-xs text-foreground-secondary">Current plan</span>
                    ) : p === "free" ? null : (
                      <button
                        type="button"
                        onClick={() => handleChangePlan(p)}
                        disabled={isPending}
                        className="inline-flex items-center gap-1 rounded-[6px] bg-[color:var(--ink-900)] h-10 px-4 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
                      >
                        {isPending ? "..." : "Choose"}
                        <ArrowRight className="h-3 w-3" aria-hidden="true" />
                      </button>
                    )}
                  </td>
                ))}
              </tr>
            </tfoot>
          )}
        </table>
      </div>
      {/* Mobile stacked layout */}
      <div className="sm:hidden space-y-8">
        {plans.map((p) => (
          <div key={p} className="space-y-3">
            <div className="flex items-baseline justify-between">
              <h3
                className={`text-sm font-medium ${
                  p === currentPlan ? "text-[color:var(--moss-700)]" : "text-foreground"
                }`}
              >
                {PLAN_LABELS[p]}
                {p === currentPlan && (
                  <span className="ml-1 text-xs font-normal">(current)</span>
                )}
              </h3>
              <span className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-foreground">
                {PLAN_PRICES[p]}<span className="text-xs font-normal text-foreground-secondary">/mo</span>
              </span>
            </div>
            <ul className="space-y-1.5">
              {PLAN_FEATURES.map((row) => {
                const val = row[p];
                const isDash = val === "---";
                return (
                  <li key={row.feature} className="flex items-center justify-between text-sm">
                    <span className={isDash ? "text-foreground-tertiary" : "text-foreground-secondary"}>{row.feature}</span>
                    {isDash ? (
                      <span className="text-[color:var(--ink-900)]/20">---</span>
                    ) : val === "\u2713" ? (
                      <span className="text-[color:var(--moss-700)]">{val}</span>
                    ) : (
                      <span className="tabular-nums text-foreground">{val}</span>
                    )}
                  </li>
                );
              })}
            </ul>
            {editable && p !== currentPlan && p !== "free" && (
              <button
                type="button"
                onClick={() => handleChangePlan(p)}
                disabled={isPending}
                className="inline-flex items-center gap-1 rounded-[6px] bg-[color:var(--ink-900)] h-10 px-4 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
              >
                {isPending ? "..." : "Choose"}
                <ArrowRight className="h-3 w-3" aria-hidden="true" />
              </button>
            )}
            {p === currentPlan && (
              <span className="text-xs text-foreground-secondary">Current plan</span>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
