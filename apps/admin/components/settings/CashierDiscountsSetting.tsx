"use client";

import { useState, useTransition } from "react";

import { setCashierDiscountsAction } from "@/app/(admin)/settings/team/actions";
import { ToggleSwitch } from "@/components/settings/BrandingSettingsClient";

// Store setting (#1104): whether cashiers may give discounts and ring up
// custom prices at the counter. Off by default.
export function CashierDiscountsSetting({ storeName, initial }: { storeName: string; initial: boolean }) {
  const [enabled, setEnabled] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const change = (next: boolean) => {
    setError(null);
    startTransition(async () => {
      const res = await setCashierDiscountsAction(next);
      if (res.ok) setEnabled(res.enabled);
      else setError(res.message);
    });
  };

  return (
    <section className="flex flex-col gap-2">
      <label className="flex items-center justify-between gap-6">
        <span className="flex flex-col gap-1">
          <span className="text-sm font-medium text-foreground">Cashiers can give discounts and custom prices</span>
          <span className="text-xs text-foreground-tertiary">
            At {storeName}. Off: only you, admins and staff can.
          </span>
        </span>
        <ToggleSwitch checked={enabled} onChange={change} disabled={pending} />
      </label>
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </section>
  );
}
