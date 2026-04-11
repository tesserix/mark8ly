import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getSupportedProviders,
  getTaxConfig,
  listPaymentConfigs,
} from "@/lib/api/settings-api";
import { PaymentSettingsClient } from "@/components/settings/PaymentSettingsClient";

/**
 * /settings/payments — payment gateway + tax configuration.
 *
 * Fetches three things in parallel:
 *   1. supported-providers — the country-allowed payment catalogue, so the
 *      UI can render one card per provider instead of a dead-end empty state.
 *   2. existing configs — to hydrate the cards that are already saved.
 *   3. tax config — folded into this page as a section below the providers.
 */
export default async function PaymentSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Store setup</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Payments &amp; tax
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Configure payment gateways for your store
            {currentStore ? ` (${currentStore.country_code})` : ""}.
            Providers are determined by your store&apos;s country. Each one
            needs API credentials and must be activated before customers can
            check out.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view settings but cannot edit
              them.
            </p>
          )}
        </header>

        {currentStore ? (
          <PaymentSettingsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring payments.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function PaymentSettingsContent({
  storeId,
  userId,
  tenantId,
  editable,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
}) {
  const session = { userId, tenantId };
  const [supported, configs, taxConfig] = await Promise.all([
    getSupportedProviders(storeId, session),
    listPaymentConfigs(storeId, session),
    getTaxConfig(storeId, session),
  ]);

  if (!supported) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          Unable to load supported providers for this store. Please try again
          later.
        </p>
      </div>
    );
  }

  return (
    <PaymentSettingsClient
      supported={supported}
      configs={configs}
      editable={editable}
      taxConfig={taxConfig}
    />
  );
}
