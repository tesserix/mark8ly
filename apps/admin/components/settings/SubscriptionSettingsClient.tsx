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

const PLAN_FEATURES: { feature: string; free: boolean; starter: boolean; pro: boolean; enterprise: boolean }[] = [
  { feature: "Products", free: true, starter: true, pro: true, enterprise: true },
  { feature: "Staff accounts", free: false, starter: true, pro: true, enterprise: true },
  { feature: "Custom domain", free: false, starter: true, pro: true, enterprise: true },
  { feature: "Gift cards", free: false, starter: false, pro: true, enterprise: true },
  { feature: "Coupons & discounts", free: false, starter: true, pro: true, enterprise: true },
  { feature: "Analytics", free: false, starter: false, pro: true, enterprise: true },
  { feature: "Audit logs", free: false, starter: false, pro: true, enterprise: true },
  { feature: "Loyalty program", free: false, starter: false, pro: false, enterprise: true },
  { feature: "Priority support", free: false, starter: false, pro: false, enterprise: true },
  { feature: "API access", free: false, starter: false, pro: true, enterprise: true },
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
      <div className="rounded-[6px] bg-white p-6 space-y-4">
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
        {error && <p className="text-sm text-[color:var(--signal)]">{error}</p>}
        {editable && plan !== "free" && (
          <button
            type="button"
            onClick={handleManageBilling}
            disabled={isPending}
            className="h-10 rounded-[6px] border border-border bg-white px-5 text-sm font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
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
          Choose the plan that fits your business.
        </p>
      </div>
      {error && <p className="text-sm text-[color:var(--signal)]">{error}</p>}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="pb-3 pr-4 text-left font-medium text-foreground-secondary">Feature</th>
              {plans.map((p) => (
                <th
                  key={p}
                  className={`pb-3 px-4 text-center font-medium ${
                    p === currentPlan ? "text-[color:var(--moss-700)]" : "text-foreground-secondary"
                  }`}
                >
                  {PLAN_LABELS[p]}
                  {p === currentPlan && (
                    <span className="ml-1 text-xs font-normal">(current)</span>
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {PLAN_FEATURES.map((row) => (
              <tr key={row.feature} className="border-b border-border">
                <td className="py-3 pr-4 text-foreground">{row.feature}</td>
                {plans.map((p) => (
                  <td key={p} className="py-3 px-4 text-center">
                    {row[p] ? (
                      <span className="text-[color:var(--moss-700)]" aria-label="Included">
                        &#10003;
                      </span>
                    ) : (
                      <span className="text-[color:var(--ink-900)]/20" aria-label="Not included">
                        ---
                      </span>
                    )}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
          {editable && (
            <tfoot>
              <tr>
                <td className="pt-4" />
                {plans.map((p) => (
                  <td key={p} className="pt-4 px-4 text-center">
                    {p === currentPlan ? (
                      <span className="text-xs text-foreground-secondary">Current plan</span>
                    ) : p === "free" ? null : (
                      <button
                        type="button"
                        onClick={() => handleChangePlan(p)}
                        disabled={isPending}
                        className="inline-flex items-center gap-1 rounded-[6px] bg-[color:var(--ink-900)] px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
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
    </section>
  );
}
