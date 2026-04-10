"use client";

import * as React from "react";

type BulkField = "price" | "stock" | "weight";

type BulkPatch =
  | { field: "price"; value: string }
  | { field: "stock"; value: number }
  | { field: "weight"; value: number };

export interface VariantBulkBarProps {
  variantCount: number;
  currencyCode: string;
  onBulkPatch: (patch: BulkPatch) => void;
}

const FIELD_LABELS: Record<BulkField, string> = {
  price: "Price",
  stock: "Stock",
  weight: "Weight",
};

export function VariantBulkBar({
  variantCount,
  onBulkPatch,
}: VariantBulkBarProps): React.ReactElement {
  const [active, setActive] = React.useState<BulkField | null>(null);
  const [draft, setDraft] = React.useState("");

  const start = (field: BulkField): void => {
    setActive(field);
    setDraft("");
  };

  const cancel = (): void => {
    setActive(null);
    setDraft("");
  };

  const apply = (): void => {
    if (active === null) return;
    if (active === "price") {
      onBulkPatch({ field: "price", value: draft });
    } else if (active === "stock") {
      const n = Number.parseInt(draft, 10);
      if (Number.isNaN(n)) return;
      onBulkPatch({ field: "stock", value: n });
    } else {
      const n = Number.parseFloat(draft);
      if (Number.isNaN(n)) return;
      onBulkPatch({ field: "weight", value: n });
    }
    setActive(null);
    setDraft("");
  };

  return (
    <div className="flex items-center justify-between gap-4 border-t border-[var(--ink-100)] bg-[var(--paper-200)] px-4 py-3">
      <span className="font-[family-name:var(--font-serif)] text-sm text-[var(--ink-900)]">
        {variantCount} variants
      </span>
      {active === null ? (
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => start("price")}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
          >
            Set all prices…
          </button>
          <button
            type="button"
            onClick={() => start("stock")}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
          >
            Set all stock…
          </button>
          <button
            type="button"
            onClick={() => start("weight")}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
          >
            Set all weights…
          </button>
        </div>
      ) : (
        <div className="flex items-center gap-2">
          <label
            htmlFor={`bulk-${active}`}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)]"
          >
            {FIELD_LABELS[active]}
          </label>
          <input
            id={`bulk-${active}`}
            type={active === "price" ? "text" : "number"}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            className="w-28 bg-[var(--background-elevated)] px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          />
          <button
            type="button"
            onClick={apply}
            className="text-xs uppercase tracking-widest text-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          >
            Apply
          </button>
          <button
            type="button"
            onClick={cancel}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          >
            Cancel
          </button>
        </div>
      )}
    </div>
  );
}
