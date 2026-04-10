"use client";

// ShippingSettingsClient — renders the list of shipping carrier cards
// with inline configure forms and remove actions.

import { useRouter } from "next/navigation";

import type { ShippingConfig } from "@/lib/api/settings-api";
import { ProviderCard } from "./ProviderCard";
import { ShippingConfigForm } from "./ShippingConfigForm";
import { removeShippingConfig } from "@/app/settings/shipping/actions";

interface ShippingSettingsClientProps {
  configs: ShippingConfig[];
  editable: boolean;
}

export function ShippingSettingsClient({
  configs,
  editable,
}: ShippingSettingsClientProps) {
  const router = useRouter();

  if (configs.length === 0) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          No shipping carriers configured yet. Configure a carrier to enable
          shipping rate calculation.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {configs.map((cfg) => (
        <ProviderCard
          key={cfg.id}
          providerName={cfg.provider}
          isActive={cfg.enabled}
          maskedKey={cfg.api_key}
          mode={cfg.mode}
          onConfigure={() => {
            // Toggle handled internally by ProviderCard
          }}
          onRemove={async () => {
            if (!editable) return;
            await removeShippingConfig(cfg.provider);
            router.refresh();
          }}
        >
          {editable ? (
            <ShippingConfigForm provider={cfg.provider} existing={cfg} />
          ) : (
            <p className="text-sm text-[color:var(--ink-900)]/50">
              You do not have permission to edit this configuration.
            </p>
          )}
        </ProviderCard>
      ))}
    </div>
  );
}
