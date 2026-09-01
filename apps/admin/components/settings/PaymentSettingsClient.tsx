"use client";

// PaymentSettingsClient — renders one configuration card per payment provider
// the store's country supports (read from supported_countries via the
// supported-providers API). Unconfigured providers expose "Add credentials";
// configured providers expose Edit, Test connection, and Remove.
//
// Tax configuration is folded in as a section below the providers — it's
// country-driven, so it lives next to the other country-driven settings.

import { useRouter } from "next/navigation";

import type {
  PaymentConfig,
  SupportedProviders,
  TaxConfig,
} from "@/lib/api/settings-api";
import { ProviderCard } from "./ProviderCard";
import { PaymentConfigForm } from "./PaymentConfigForm";
import { paymentReadinessFor } from "@/lib/settings/payments-readiness";
import { TaxJarConfigForm } from "./TaxJarConfigForm";
import {
  removePaymentConfig,
  testPayment,
} from "@/app/(admin)/settings/payments/actions";

interface PaymentSettingsClientProps {
  supported: SupportedProviders;
  configs: PaymentConfig[];
  editable: boolean;
  taxConfig: TaxConfig | null;
}

export function PaymentSettingsClient({
  supported,
  configs,
  editable,
  taxConfig,
}: PaymentSettingsClientProps) {
  const router = useRouter();

  // Index existing configs by provider so each supported provider card
  // can look up its own state in O(1).
  const configByProvider = new Map(configs.map((c) => [c.provider, c]));

  return (
    <div className="space-y-10">
      {/* Payment providers — one card per country-supported provider */}
      <section className="space-y-3">
        {supported.payment_providers.length === 0 ? (
          <p className="border-t border-border-subtle py-10 text-sm text-foreground-tertiary">
            No payment providers are currently available for{" "}
            {supported.country_name} ({supported.country_code}).
          </p>
        ) : (
          supported.payment_providers.map((provider) => {
            const cfg = configByProvider.get(provider);
            const configured = Boolean(cfg);
            // Everything between this gateway and a recorded payment. Only
            // shown once credentials exist — "no gateway" is already plain
            // from the card's own Add-credentials state.
            const blockers = configured ? paymentReadinessFor(cfg) : [];

            return (
              <ProviderCard
                key={provider}
                providerName={provider}
                isActive={cfg?.is_active ?? false}
                maskedKey={cfg?.api_key ?? ""}
                mode={cfg?.mode ?? "test"}
                configured={configured}
                onRemove={
                  configured
                    ? async () => {
                        if (!editable) return;
                        await removePaymentConfig(provider);
                        router.refresh();
                      }
                    : undefined
                }
                onTest={
                  configured
                    ? async () => {
                        const result = await testPayment(provider);
                        if (!result.ok) {
                          return { success: false, error: result.message };
                        }
                        return {
                          success: result.success,
                          error: result.error,
                        };
                      }
                    : undefined
                }
              >
                {blockers.length > 0 && (
                  <div
                    role="status"
                    className="mb-5 space-y-2 rounded-md border border-[color:var(--signal,#B7410E)]/25 bg-[color:var(--signal,#B7410E)]/[0.04] px-4 py-3"
                  >
                    <p className="text-sm font-medium text-foreground">
                      {blockers.some((b) => b.severity === "silently_loses_orders")
                        ? "This gateway can take payments it will not record"
                        : "This gateway isn’t taking payments yet"}
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
                )}

                {editable ? (
                  <PaymentConfigForm
                    provider={provider}
                    initialMode={cfg?.mode}
                    initialIsActive={cfg?.is_active ?? false}
                    hasWebhookSecret={Boolean(cfg?.webhook_secret)}
                  />
                ) : (
                  <p className="text-sm text-foreground-tertiary">
                    You do not have permission to edit this configuration.
                  </p>
                )}
              </ProviderCard>
            );
          })
        )}
      </section>

      {/* Tax — country-driven, folded in from the old /settings/tax page */}
      <TaxSection
        countryCode={supported.country_code}
        taxConfig={taxConfig}
        editable={editable}
      />
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Tax section — strategy-aware (flat_rate / india_gst / taxjar)
// ─────────────────────────────────────────────────────────────────────────

interface TaxSectionProps {
  countryCode: string;
  taxConfig: TaxConfig | null;
  editable: boolean;
}

function TaxSection({ countryCode, taxConfig, editable }: TaxSectionProps) {
  return (
    <section className="space-y-3">
      <div className="space-y-2">
        <p className="eyebrow">Tax</p>
        <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
          Tax strategy
        </h2>
        <p className="text-sm text-foreground-tertiary">
          Tax calculation is determined by your store&apos;s country (
          {countryCode}) and cannot be changed here.
        </p>
      </div>

      {!taxConfig ? (
        <p className="border-t border-border-subtle py-5 text-sm text-foreground-tertiary">
          Unable to load tax configuration. Please try again later.
        </p>
      ) : (
        <div className="space-y-6">
          {/* Strategy summary — hairline row, not a bordered card */}
          <div className="flex items-start gap-4 border-t border-border-subtle pt-5">
            <div className="space-y-1.5">
              <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                Strategy
              </h3>
              <p className="text-sm text-foreground-secondary">
                {strategyLabel(taxConfig.strategy, countryCode)}
              </p>
            </div>
            <span className="ml-auto inline-block rounded-sm bg-[color:var(--moss-700)]/10 px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)]">
              {taxConfig.strategy.replace(/_/g, " ")}
            </span>
          </div>

          {/* Flat-rate countries */}
          {taxConfig.strategy === "flat_rate" && taxConfig.rate != null && (
            <div className="border-t border-border-subtle pt-5">
              <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary mb-3">
                Rate
              </h3>
              <p className="font-serif text-2xl tabular-nums text-foreground">
                {taxConfig.rate}%
              </p>
              <p className="mt-2 text-sm text-foreground-tertiary">
                Applied automatically to all orders from {countryCode}.
              </p>
            </div>
          )}

          {/* India GST */}
          {taxConfig.strategy === "india_gst" && taxConfig.india_gst && (
            <div className="space-y-3 border-t border-border-subtle pt-5">
              <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                India GST (automatic)
              </h3>
              <p className="max-w-prose text-sm leading-6 text-foreground-secondary">
                {taxConfig.india_gst.description}
              </p>
              <p className="text-sm text-foreground-tertiary">
                Set the HSN code and GST rate on each product to ensure correct
                tax at checkout.
              </p>
            </div>
          )}

          {/* US: TaxJar form */}
          {taxConfig.strategy === "taxjar" && (
            <div className="space-y-4 border-t border-border-subtle pt-5">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-foreground-tertiary">
                  TaxJar configuration
                </h3>
                {taxConfig.taxjar?.configured && (
                  <span
                    className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${
                      taxConfig.taxjar.is_active
                        ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
                        : "bg-[color:var(--ink-900)]/5 text-foreground-tertiary"
                    }`}
                  >
                    <span
                      className={`h-1.5 w-1.5 rounded-full ${
                        taxConfig.taxjar.is_active
                          ? "bg-[color:var(--moss-700)]"
                          : "bg-[color:var(--ink-900)]/30"
                      }`}
                      aria-hidden="true"
                    />
                    {taxConfig.taxjar.is_active ? "Active" : "Inactive"}
                  </span>
                )}
              </div>

              <p className="text-sm text-foreground-tertiary leading-6">
                US stores use TaxJar for sales tax calculation. Enter your
                TaxJar API key to enable automatic rates at checkout.
              </p>

              <hr className="border-t border-[color:var(--ink-900)]/6" />

              {editable ? (
                <TaxJarConfigForm
                  existingApiKey={taxConfig.taxjar?.api_key}
                  existingMode={taxConfig.taxjar?.mode}
                  isConfigured={taxConfig.taxjar?.configured ?? false}
                />
              ) : (
                <p className="text-sm text-foreground-tertiary">
                  You do not have permission to edit tax configuration.
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function strategyLabel(strategy: string, countryCode: string): string {
  switch (strategy) {
    case "flat_rate":
      return `A flat VAT/GST rate is applied to all orders from ${countryCode}.`;
    case "india_gst":
      return "India GST rates are applied per-product based on HSN codes.";
    case "taxjar":
      return "US sales tax is calculated via TaxJar based on customer location.";
    default:
      return `Tax strategy: ${strategy}`;
  }
}
