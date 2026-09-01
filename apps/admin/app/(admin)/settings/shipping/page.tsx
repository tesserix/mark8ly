import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getSupportedProviders,
  listShippingConfigs,
} from "@/lib/api/settings-api";
import { listWarehouses } from "@/lib/api/warehouses-api";
import { ShippingSettingsClient } from "@/components/settings/ShippingSettingsClient";

/**
 * /settings/shipping — shipping carrier configuration. Each carrier card
 * includes credentials, warehouse address, and fee settings.
 */
export default async function ShippingSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, userId, currentStore } =
    await getServerSessionContext();

  const editable = canEditSettings(role);
  const country = currentStore?.country_code;

  return (
          <AdminPage
        eyebrow="Selling"
        title="Shipping"
        description={
          <>
            Configure shipping carriers for your store
            {country ? ` (${country})` : ""}. Each carrier needs API
            credentials and a warehouse origin address for rate calculation.
          </>
        }
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        {currentStore ? (
          <ShippingSettingsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
            storeCountry={(currentStore.country_code ?? "").toUpperCase()}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring shipping.
          </p>
        )}
      </AdminPage>
  );
}

async function ShippingSettingsContent({
  storeId,
  userId,
  tenantId,
  editable,
  storeCountry,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
  storeCountry: string;
}) {
  const session = { userId, tenantId };
  const [supported, configs, warehouses] = await Promise.all([
    getSupportedProviders(storeId, session),
    listShippingConfigs(storeId, session),
    listWarehouses(storeId, session),
  ]);

  if (!supported) {
    return (
      <p className="border-t border-border-subtle py-10 text-sm text-foreground-tertiary">
        Unable to load supported carriers for this store. Please try again
        later.
      </p>
    );
  }

  return (
    <ShippingSettingsClient
      supported={supported}
      configs={configs}
      warehouses={warehouses}
      editable={editable}
      storeCountry={storeCountry}
    />
  );
}
