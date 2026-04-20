"use client";

// ShippingSettingsClient — renders one configuration card per shipping
// carrier the store's country supports (read from supported_countries via
// the supported-providers API). Unconfigured carriers expose "Add
// credentials"; configured carriers expose Edit and Remove.

import { useRouter } from "next/navigation";

import type {
  ShippingConfig,
  SupportedProviders,
} from "@/lib/api/settings-api";
import { ProviderCard } from "./ProviderCard";
import { ShippingConfigForm } from "./ShippingConfigForm";
import { removeShippingConfig } from "@/app/(admin)/settings/shipping/actions";

interface ShippingSettingsClientProps {
  supported: SupportedProviders;
  configs: ShippingConfig[];
  editable: boolean;
}

export function ShippingSettingsClient({
  supported,
  configs,
  editable,
}: ShippingSettingsClientProps) {
  const router = useRouter();

  // Index existing configs by carrier so each supported carrier card can
  // look up its own state in O(1).
  const configByCarrier = new Map(configs.map((c) => [c.provider, c]));

  if (supported.shipping_carriers.length === 0) {
    return (
      <div className="rounded-md border border-border-subtle bg-[color:var(--background-elevated)] px-6 py-10 text-center">
        <p className="text-sm text-foreground-tertiary">
          No shipping carriers are currently available for{" "}
          {supported.country_name} ({supported.country_code}).
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {supported.shipping_carriers.map((carrier) => {
        const cfg = configByCarrier.get(carrier);
        const configured = Boolean(cfg);

        return (
          <ProviderCard
            key={carrier}
            providerName={carrier}
            isActive={cfg?.enabled ?? false}
            maskedKey={cfg?.api_key ?? ""}
            mode={cfg?.mode ?? "test"}
            configured={configured}
            onRemove={
              configured
                ? async () => {
                    if (!editable) return;
                    await removeShippingConfig(carrier);
                    router.refresh();
                  }
                : undefined
            }
          >
            {editable ? (
              <ShippingConfigForm provider={carrier} existing={cfg} />
            ) : (
              <p className="text-sm text-foreground-tertiary">
                You do not have permission to edit this configuration.
              </p>
            )}
          </ProviderCard>
        );
      })}
    </div>
  );
}
