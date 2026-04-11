"use client";

import Link from "next/link";
import { AlertTriangle } from "lucide-react";

import type { LowStockItem } from "@/lib/api/marketplace-api";

interface LowStockAlertsProps {
  items: LowStockItem[];
}

export function LowStockAlerts({ items }: LowStockAlertsProps) {
  if (items.length === 0) return null;

  return (
    <section>
      <div className="flex items-center gap-2">
        <AlertTriangle
          className="h-4 w-4 text-[color:var(--signal)]"
          aria-hidden="true"
        />
        <h3 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-foreground">
          Low stock alerts
        </h3>
      </div>
      <div className="overflow-x-auto">
      <ul className="mt-4 space-y-0 divide-y divide-border-subtle" role="list">
        {items.map((item) => (
          <li
            key={item.id}
            className="flex min-h-[44px] items-center justify-between gap-4 border-l-2 border-l-[color:var(--signal)] py-3 pl-4"
          >
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-foreground">
                {item.title}
              </p>
              {item.variant_title && (
                <p className="text-xs text-foreground-secondary">
                  {item.variant_title}
                </p>
              )}
            </div>
            <div className="flex items-center gap-4">
              <div className="text-right">
                <p
                  className="text-sm font-medium text-[color:var(--signal)]"
                  style={{ fontFeatureSettings: '"tnum" 1' }}
                >
                  {item.quantity} left
                </p>
                <p className="text-xs text-foreground-tertiary">
                  threshold: {item.low_stock_threshold}
                </p>
              </div>
              <Link
                href="/products"
                className="inline-flex min-h-[44px] items-center text-sm text-[color:var(--moss-700)] hover:underline"
              >
                View
              </Link>
            </div>
          </li>
        ))}
      </ul>
      </div>
    </section>
  );
}
