import { AdminPage, ReadOnlyNotice } from "@/components/layout";
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
  const { tenantName, email, role, memberships, tenantId, userId, currentStore } =
    await getServerSessionContext();

  const editable = canEditSettings(role);
  const country = currentStore?.country_code;

  return (
          <AdminPage
        eyebrow="Selling"
        title="Payments & tax"
        description={
          <>
            Configure payment gateways for your store
            {country ? ` (${country})` : ""}. Providers are determined by your
            store&apos;s country. Each one needs API credentials and must be
            activated before customers can check out.
          </>
        }
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
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
      </AdminPage>
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
      <p className="border-t border-border-subtle py-10 text-sm text-foreground-tertiary">
        Unable to load supported providers for this store. Please try again
        later.
      </p>
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
