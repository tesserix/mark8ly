"use client";

import { useState, useTransition } from "react";

import { Button, Input, Label } from "@tesserix/web";

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
// "51824753556" → "51 824 753 556", as on the ABN register.
const spaced = (abn: string) => abn.replace(/^(\d{2})(\d{3})(\d{3})(\d{3})$/, "$1 $2 $3 $4");

export function InStoreSettingsForm({ storeName, countryCode, initial }: {
  storeName: string;
  countryCode: string;
  initial: InStoreSettings;
}) {
  const [settings, setSettings] = useState(initial);
  const [abn, setAbn] = useState(spaced(initial.abn ?? ""));
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const change = (patch: Partial<InStoreSettings>) => {
    setError(null);
    startTransition(async () => {
      const res = await setInStoreSettingAction(patch);
      if (res.ok) {
        setSettings(res.settings);
        setAbn(spaced(res.settings.abn ?? ""));
      } else setError(res.message);
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
      {countryCode === "AU" ? (
        <form
          className="flex flex-col gap-2 py-5"
          onSubmit={(e) => {
            e.preventDefault();
            change({ abn });
          }}
        >
          <Label htmlFor="in-store-abn">ABN</Label>
          <span className="text-xs text-foreground-tertiary">
            Printed on in-store receipts, which then count as tax invoices. Leave empty if you have none.
          </span>
          <div className="flex gap-2">
            <Input
              id="in-store-abn"
              value={abn}
              onChange={(e) => setAbn(e.target.value)}
              inputMode="numeric"
              placeholder="51 824 753 556"
              className="max-w-xs"
              disabled={pending}
            />
            <Button type="submit" variant="outline" disabled={pending}>
              Save ABN
            </Button>
          </div>
        </form>
      ) : null}
      {error ? (
        <p role="alert" className="py-3 text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
