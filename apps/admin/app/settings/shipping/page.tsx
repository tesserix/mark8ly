import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listShippingConfigs } from "@/lib/api/settings-api";
import { ShippingSettingsClient } from "@/components/settings/ShippingSettingsClient";

/**
 * /settings/shipping — shipping carrier configuration.
 *
 * Server component fetches configs from marketplace-api and renders
 * the interactive client component for inline editing. Each carrier
 * card includes credentials, warehouse address, and fee settings.
 */
export default async function ShippingSettingsPage() {
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
            Shipping settings
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Configure shipping carriers for your store
            {currentStore ? ` (${currentStore.country_code})` : ""}.
            Each carrier needs API credentials and a warehouse origin address
            for rate calculation.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view settings but cannot edit
              them.
            </p>
          )}
        </header>

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
      </div>
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
  const configs = await listShippingConfigs(storeId, { userId, tenantId });

  return <ShippingSettingsClient configs={configs} editable={editable} />;
}
