"use client";

// PaymentSettingsClient — renders the list of payment provider cards
// with inline configure forms, test connection, and remove actions.
// Receives server-fetched data as props from the page server component.

import { useRouter } from "next/navigation";

import type { PaymentConfig } from "@/lib/api/settings-api";
import { ProviderCard } from "./ProviderCard";
import { PaymentConfigForm } from "./PaymentConfigForm";
import { removePaymentConfig, testPayment } from "@/app/settings/payments/actions";

interface PaymentSettingsClientProps {
  configs: PaymentConfig[];
  editable: boolean;
}

export function PaymentSettingsClient({
  configs,
  editable,
}: PaymentSettingsClientProps) {
  const router = useRouter();

  if (configs.length === 0) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          No payment providers configured yet. Configure a provider to start
          accepting payments.
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
          isActive={cfg.is_active}
          maskedKey={cfg.api_key}
          mode={cfg.mode}
          onConfigure={() => {
            // No-op — toggling is handled internally by ProviderCard
          }}
          onRemove={async () => {
            if (!editable) return;
            await removePaymentConfig(cfg.provider);
            router.refresh();
          }}
          onTest={async () => {
            const result = await testPayment(cfg.provider);
            if (!result.ok) {
              return { success: false, error: result.message };
            }
            return { success: result.success, error: result.error };
          }}
        >
          {editable ? (
            <PaymentConfigForm
              provider={cfg.provider}
              initialMode={cfg.mode}
              initialIsActive={cfg.is_active}
            />
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
