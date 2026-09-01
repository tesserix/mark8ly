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
import { readinessFor } from "@/lib/settings/shipping-readiness";
import { removeShippingConfig } from "@/app/(admin)/settings/shipping/actions";

interface ShippingSettingsClientProps {
  supported: SupportedProviders;
  configs: ShippingConfig[];
  editable: boolean;
  /**
   * ISO 3166 alpha-2 country code from the active store's session
   * context. Used to pre-select the warehouse country dropdown. We
   * trust this over `supported.country_code` because the session
   * context is the canonical source.
   */
  storeCountry: string;
}

export function ShippingSettingsClient({
  supported,
  configs,
  editable,
  storeCountry,
}: ShippingSettingsClientProps) {
  const router = useRouter();

  // Index existing configs by carrier so each supported carrier card can
  // look up its own state in O(1).
  const configByCarrier = new Map(configs.map((c) => [c.provider, c]));

  if (supported.shipping_carriers.length === 0) {
    return (
      <p className="border-t border-border-subtle py-10 text-sm text-foreground-tertiary">
        No shipping carriers are currently available for{" "}
        {supported.country_name} ({supported.country_code}).
      </p>
    );
  }

  return (
    <div className="space-y-3">
      {supported.shipping_carriers.map((carrier) => {
        const cfg = configByCarrier.get(carrier);
        const configured = Boolean(cfg);
        // Everything standing between this carrier and a live rate. Only
        // shown once credentials exist — "no carrier" is already obvious
        // from the card's own Add-credentials state.
        const blockers = configured ? readinessFor(cfg) : [];

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
                    // Surface the failure rather than refreshing over it:
                    // this DELETE was 401 for months and looked like a no-op.
                    const result = await removeShippingConfig(carrier);
                    if (!result.ok) {
                      return { ok: false, message: result.message };
                    }
                    router.refresh();
                    return { ok: true };
                  }
                : undefined
            }
            banner={
              blockers.length > 0 ? (
                <div
                  role="status"
                  className="mb-5 space-y-2 rounded-md border border-[color:var(--signal,#B7410E)]/25 bg-[color:var(--signal,#B7410E)]/[0.04] px-4 py-3"
                >
                  <p className="text-sm font-medium text-foreground">
                    This carrier isn&rsquo;t quoting rates yet
                  </p>
                  <ul className="space-y-1 text-sm text-foreground-secondary">
                    {blockers.map((b) => (
                      <li key={b.code} className="flex gap-2">
                        <span aria-hidden="true">&middot;</span>
                        <span>{b.message}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null
            }
          >
            {editable ? (
              <ShippingConfigForm
                provider={carrier}
                existing={cfg}
                defaultCountryCode={storeCountry || supported.country_code}
              />
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
