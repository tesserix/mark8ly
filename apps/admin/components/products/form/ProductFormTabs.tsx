"use client";

import type { KeyboardEvent } from "react";

export type ProductFormTabId = "general" | "media" | "options" | "variants";

export interface ProductFormTabsProps {
  active: ProductFormTabId;
  onChange: (next: ProductFormTabId) => void;
  badges?: Partial<Record<ProductFormTabId, number>>;
  disabled?: Partial<Record<ProductFormTabId, string>>;
}

const TABS: Array<{ id: ProductFormTabId; label: string }> = [
  { id: "general", label: "General" },
  { id: "media", label: "Media" },
  { id: "options", label: "Options" },
  { id: "variants", label: "Variants" },
];

export function ProductFormTabs({
  active,
  onChange,
  badges,
  disabled,
}: ProductFormTabsProps) {
  const handleKey = (e: KeyboardEvent<HTMLButtonElement>, idx: number): void => {
    if (e.key === "ArrowRight") {
      e.preventDefault();
      const next = TABS[(idx + 1) % TABS.length];
      if (next && !disabled?.[next.id]) onChange(next.id);
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      const prev = TABS[(idx - 1 + TABS.length) % TABS.length];
      if (prev && !disabled?.[prev.id]) onChange(prev.id);
    } else if (e.key === "Home") {
      e.preventDefault();
      const first = TABS[0];
      if (first) onChange(first.id);
    } else if (e.key === "End") {
      e.preventDefault();
      const last = TABS[TABS.length - 1];
      if (last) onChange(last.id);
    }
  };

  return (
    <div
      role="tablist"
      className="flex gap-6 border-b border-[color:var(--ink-900)] border-opacity-10"
    >
      {TABS.map((tab, idx) => {
        const isActive = active === tab.id;
        const isDisabled = !!disabled?.[tab.id];
        const badge = badges?.[tab.id];
        return (
          <button
            key={tab.id}
            role="tab"
            type="button"
            aria-selected={isActive}
            aria-disabled={isDisabled}
            tabIndex={isActive ? 0 : -1}
            disabled={isDisabled}
            title={disabled?.[tab.id]}
            onKeyDown={(e) => handleKey(e, idx)}
            onClick={() => !isDisabled && onChange(tab.id)}
            className={[
              "relative -mb-px pb-3 pt-2 text-sm transition-colors",
              isActive
                ? "text-[color:var(--ink-900)] border-b-2 border-[color:var(--moss-700)]"
                : "text-foreground-tertiary hover:opacity-100",
              isDisabled ? "opacity-40 cursor-not-allowed" : "cursor-pointer",
              "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]",
            ].join(" ")}
          >
            {tab.label}
            {typeof badge === "number" && badge > 0 && (
              <span className="ml-2 text-xs text-foreground-tertiary">
                {badge}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
