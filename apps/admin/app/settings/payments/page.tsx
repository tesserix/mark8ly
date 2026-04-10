import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listPaymentConfigs } from "@/lib/api/settings-api";
import { PaymentSettingsClient } from "@/components/settings/PaymentSettingsClient";

/**
 * /settings/payments — payment gateway configuration.
 *
 * Server component fetches configs from marketplace-api and renders
 * the interactive client component for inline editing.
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
            Payment settings
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Configure payment gateways for your store
            {currentStore ? ` (${currentStore.country_code})` : ""}.
            Each provider needs API credentials and must be activated before
            customers can check out.
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
  const configs = await listPaymentConfigs(storeId, { userId, tenantId });

  return <PaymentSettingsClient configs={configs} editable={editable} />;
}
