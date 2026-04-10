"use client";

import { useState } from "react";
import { Check, ChevronDown } from "lucide-react";
import Link from "next/link";

import type { SetupChecklist as SetupChecklistData } from "@/lib/api/marketplace-api";

interface SetupChecklistProps {
  checklist: SetupChecklistData;
}

interface ChecklistItem {
  key: keyof SetupChecklistData;
  label: string;
  href: string;
  phase: string;
}

const items: ChecklistItem[] = [
  { key: "has_store_settings", label: "Configure store settings", href: "/settings/general", phase: "Store Setup" },
  { key: "has_product", label: "Add your first product", href: "/products", phase: "Store Setup" },
  { key: "has_storefront", label: "Customize your storefront", href: "/settings/storefront", phase: "Store Setup" },
  { key: "has_payment", label: "Set up a payment gateway", href: "/settings/payments", phase: "Payments & Shipping" },
  { key: "has_shipping", label: "Configure shipping rates", href: "/settings/shipping", phase: "Payments & Shipping" },
  { key: "has_tax", label: "Set up tax rules", href: "/settings/tax", phase: "Payments & Shipping" },
  { key: "has_domain", label: "Connect a custom domain", href: "/settings/domains", phase: "Launch" },
  { key: "has_test_order", label: "Place a test order", href: "/orders", phase: "Launch" },
];

const phases = ["Store Setup", "Payments & Shipping", "Launch"] as const;

export function SetupChecklist({ checklist }: SetupChecklistProps) {
  const completedCount = items.filter((item) => checklist[item.key]).length;
  const allComplete = completedCount === 8;
  const [collapsed, setCollapsed] = useState(false);

  if (allComplete) return null;

  const progressPct = (completedCount / 8) * 100;

  return (
    <section className="border-b border-border-subtle pb-8">
      <button
        type="button"
        onClick={() => setCollapsed((prev) => !prev)}
        className="flex w-full items-center justify-between py-2"
        aria-expanded={!collapsed}
      >
        <div className="space-y-1 text-left">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-xl font-medium text-foreground">
            Store setup
          </h2>
          <p className="text-sm text-foreground-secondary">
            {completedCount} of 8 complete
          </p>
        </div>
        <ChevronDown
          className={`h-5 w-5 text-foreground-tertiary transition-transform ${
            collapsed ? "-rotate-90" : ""
          }`}
          aria-hidden="true"
        />
      </button>

      <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-[color:var(--ink-900)]/10">
        <div
          className="h-full rounded-full bg-[color:var(--moss-700)] transition-all duration-500"
          style={{ width: `${progressPct}%` }}
          role="progressbar"
          aria-valuenow={completedCount}
          aria-valuemin={0}
          aria-valuemax={8}
          aria-label={`Setup progress: ${completedCount} of 8 complete`}
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
                    const done = checklist[item.key];
                    return (
                      <li key={item.key} className="flex items-center gap-3">
                        <span
                          className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full ${
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
