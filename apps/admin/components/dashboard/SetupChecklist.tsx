"use client";

import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown, PartyPopper, Sparkles } from "lucide-react";
import Link from "next/link";

import type {
  SetupChecklist as SetupChecklistData,
  SetupProgress,
} from "@/lib/api/marketplace-api";
import { useSetupProgress } from "@/lib/hooks/useSetupProgress";

interface SetupChecklistProps {
  checklist: SetupChecklistData;
  storeId: string;
}

/**
 * "store_created" is a pseudo-step: it is complete by definition (the
 * merchant is looking at their store's dashboard). Together with the
 * prefilled store settings it gives every new merchant a 2/10 = 20%
 * head start — progress they can build on rather than a zero to climb.
 */
type StepKey = keyof SetupChecklistData | "store_created";

interface ChecklistItem {
  key: StepKey;
  label: string;
  href: string;
  phase: string;
}

const items: ChecklistItem[] = [
  { key: "store_created", label: "Create your store", href: "/dashboard", phase: "Store foundation" },
  { key: "has_store", label: "Configure store settings", href: "/settings/stores", phase: "Store foundation" },
  { key: "has_brand_assets", label: "Add store logo", href: "/settings/themes", phase: "Store foundation" },
  { key: "has_product", label: "Add your first product", href: "/products", phase: "Store foundation" },
  { key: "has_storefront_theme", label: "Customize your theme", href: "/settings/themes", phase: "Store foundation" },
  { key: "has_payment_provider", label: "Set up a payment gateway", href: "/settings/payments", phase: "Go live" },
  { key: "has_shipping_carrier", label: "Setup shipping integration", href: "/settings/shipping", phase: "Go live" },
  { key: "has_return_policy", label: "Write a return policy", href: "/settings/themes?tab=policies", phase: "Go live" },
  { key: "has_custom_domain", label: "Connect a custom domain", href: "/settings/domains", phase: "Go live" },
  { key: "has_first_order", label: "Make your first sale", href: "/orders", phase: "Go live" },
];

const phases = ["Store foundation", "Go live"] as const;

const TOTAL_STEPS = items.length;

function isDone(checklist: SetupChecklistData, key: StepKey): boolean {
  if (key === "store_created") return true;
  return checklist[key];
}

function completedCount(checklist: SetupChecklistData): number {
  return items.filter((item) => isDone(checklist, item.key)).length;
}

export function SetupChecklist({ checklist, storeId }: SetupChecklistProps) {
  const initial: SetupProgress = {
    checklist,
    completed_steps: completedCount(checklist),
    total_steps: TOTAL_STEPS,
    percent: Math.round((completedCount(checklist) / TOTAL_STEPS) * 100),
  };
  const progress = useSetupProgress(storeId, initial);
  const live = progress.checklist;

  const completed = completedCount(live);
  const percent = Math.round((completed / TOTAL_STEPS) * 100);
  const allComplete = completed === TOTAL_STEPS;

  // Stores that were already fully set up when the page rendered never
  // see the checklist; stores that FINISH during this session get the
  // celebration state instead of the section silently vanishing.
  const initiallyComplete = useRef(initial.completed_steps === TOTAL_STEPS);
  const [dismissed, setDismissed] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  // Track which step just flipped so it can pulse — the small "win"
  // moment when a step completes live while the merchant is watching.
  const prevChecklist = useRef(live);
  const [justCompleted, setJustCompleted] = useState<StepKey | null>(null);
  useEffect(() => {
    const prev = prevChecklist.current;
    prevChecklist.current = live;
    const flipped = items.find(
      (item) =>
        item.key !== "store_created" &&
        !isDone(prev, item.key) &&
        isDone(live, item.key),
    );
    if (!flipped) return;
    setJustCompleted(flipped.key);
    const timer = setTimeout(() => setJustCompleted(null), 2500);
    return () => clearTimeout(timer);
  }, [live]);

  if (initiallyComplete.current || dismissed) return null;

  if (allComplete) {
    return (
      <section className="border-b border-border-subtle pb-8">
        <div className="flex items-start justify-between gap-4 rounded-lg bg-[color:var(--moss-700)]/8 p-6">
          <div className="flex items-start gap-4">
            <PartyPopper
              className="mt-1 h-6 w-6 text-[color:var(--moss-700)]"
              aria-hidden="true"
            />
            <div className="space-y-1">
              <h2 className="font-serif text-xl font-medium text-foreground">
                Store setup complete — you&apos;re live!
              </h2>
              <p className="text-sm text-foreground-secondary">
                All {TOTAL_STEPS} steps done. Your store is fully configured
                and ready for customers.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={() => setDismissed(true)}
            className="shrink-0 text-sm text-foreground-tertiary hover:text-foreground"
          >
            Dismiss
          </button>
        </div>
      </section>
    );
  }

  return (
    <section className="border-b border-border-subtle pb-8">
      <button
        type="button"
        onClick={() => setCollapsed((prev) => !prev)}
        className="flex w-full items-center justify-between py-2"
        aria-expanded={!collapsed}
      >
        <div className="space-y-1 text-left">
          <h2 className="font-serif text-xl font-medium text-foreground">
            Store setup
          </h2>
          <p className="text-sm text-foreground-secondary">
            {completed} of {TOTAL_STEPS} complete
            <span className="mx-2 text-foreground-tertiary" aria-hidden="true">
              ·
            </span>
            <Sparkles
              className="mr-1 inline h-3.5 w-3.5 text-[color:var(--moss-700)]"
              aria-hidden="true"
            />
            We completed your first steps for you
          </p>
        </div>
        <div className="flex items-center gap-3">
          <span
            className="font-serif text-2xl font-medium text-[color:var(--moss-700)] tabular-nums"
            aria-hidden="true"
          >
            {percent}%
          </span>
          <ChevronDown
            className={`h-5 w-5 text-foreground-tertiary transition-transform ${
              collapsed ? "-rotate-90" : ""
            }`}
            aria-hidden="true"
          />
        </div>
      </button>

      <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-[color:var(--ink-900)]/10">
        <div
          className="h-full rounded-full bg-[color:var(--moss-700)] transition-[width] duration-700 ease-out"
          style={{ width: `${percent}%` }}
          role="progressbar"
          aria-valuenow={completed}
          aria-valuemin={0}
          aria-valuemax={TOTAL_STEPS}
          aria-label={`Setup progress: ${completed} of ${TOTAL_STEPS} complete`}
        />
      </div>

      {!collapsed && (
        <div className="mt-6 space-y-6">
          {phases.map((phase) => {
            const phaseItems = items.filter((item) => item.phase === phase);
            return (
              <div key={phase}>
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                  {phase}
                </p>
                <ul className="mt-3 space-y-3">
                  {phaseItems.map((item) => {
                    const done = isDone(live, item.key);
                    const pulsing = justCompleted === item.key;
                    return (
                      <li
                        key={item.key}
                        className={`flex items-center gap-3 rounded transition-colors duration-700 ${
                          pulsing ? "bg-[color:var(--moss-700)]/10" : ""
                        }`}
                      >
                        <span
                          className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full transition-colors duration-300 ${
                            done
                              ? "bg-[color:var(--moss-700)] text-white"
                              : "border border-[color:var(--ink-900)]/20"
                          }`}
                          aria-hidden="true"
                        >
                          {done && <Check className="h-3 w-3" />}
                        </span>
                        {done ? (
                          <span className="text-sm text-foreground-secondary line-through">
                            {item.label}
                          </span>
                        ) : (
                          <Link
                            href={item.href}
                            className="text-sm text-[color:var(--moss-700)] hover:underline"
                          >
                            {item.label}
                          </Link>
                        )}
                        {pulsing && (
                          <span className="text-xs font-medium text-[color:var(--moss-700)]">
                            Done!
                          </span>
                        )}
                      </li>
                    );
                  })}
                </ul>
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
