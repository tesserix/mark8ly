import { AdminShell } from "@/components/shell/AdminShell";
import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getSupportedProviders,
  listShippingConfigs,
} from "@/lib/api/settings-api";
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
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
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
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring shipping.
          </p>
        )}
      </AdminPage>
    </AdminShell>
  );
}

async function ShippingSettingsContent({
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
  const [supported, configs] = await Promise.all([
    getSupportedProviders(storeId, session),
    listShippingConfigs(storeId, session),
  ]);

  if (!supported) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          Unable to load supported carriers for this store. Please try again
          later.
        </p>
      </div>
    );
  }

  return (
    <ShippingSettingsClient
      supported={supported}
      configs={configs}
      editable={editable}
    />
  );
}
