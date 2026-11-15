"use client";

import { useState, useTransition } from "react";

import { setInStoreSettingAction } from "@/app/(admin)/settings/in-store/actions";
import { ToggleSwitch } from "@/components/settings/BrandingSettingsClient";
import type { InStoreSettings } from "@/lib/api/instore-api";

function Row({ label, hint, checked, disabled, onChange }: {
  label: string;
  hint: string;
  checked: boolean;
  disabled: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between gap-6 py-5">
      <span className="flex flex-col gap-1">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <span className="text-xs text-foreground-tertiary">{hint}</span>
      </span>
      <ToggleSwitch checked={checked} onChange={onChange} disabled={disabled} />
    </label>
  );
}

// The store's in-store sales switches: selling at a counter or stall at all,
// and whether cashiers may give discounts and custom prices.
export function InStoreSettingsForm({ storeName, initial }: { storeName: string; initial: InStoreSettings }) {
  const [settings, setSettings] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const change = (patch: Partial<InStoreSettings>) => {
    setError(null);
    startTransition(async () => {
      const res = await setInStoreSettingAction(patch);
      if (res.ok) setSettings(res.settings);
      else setError(res.message);
    });
  };

  return (
    <div className="flex flex-col divide-y divide-border-subtle">
      <Row
        label="Sell in store"
        hint={`Ring up sales at ${storeName}'s counter or stall from Orders → In-store sale. Turning this off hides it; past in-store orders stay.`}
        checked={settings.in_store_enabled}
        disabled={pending}
        onChange={(v) => change({ in_store_enabled: v })}
      />
      {settings.in_store_enabled ? (
        <Row
          label="Cashiers can give discounts and custom prices"
          hint="Off: only you, admins and staff can."
          checked={settings.cashier_discounts_enabled}
          disabled={pending}
          onChange={(v) => change({ cashier_discounts_enabled: v })}
        />
      ) : null}
      {error ? (
        <p role="alert" className="py-3 text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
