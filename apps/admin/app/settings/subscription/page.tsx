import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getSubscription } from "@/lib/api/settings-tier2-api";
import { SubscriptionSettingsClient } from "@/components/settings/SubscriptionSettingsClient";

/**
 * /settings/subscription — plan management and billing.
 *
 * Server component fetches the current subscription from marketplace-api
 * and renders the plan card + comparison grid via client component.
 */
export default async function SubscriptionSettingsPage() {
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
          <p className="eyebrow">Settings</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Subscription
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Manage your plan, view billing details, and compare available tiers.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view subscription details but
              cannot make changes.
            </p>
          )}
        </header>

        {currentStore ? (
          <SubscriptionSettingsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before managing subscriptions.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function SubscriptionSettingsContent({
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
  const subscription = await getSubscription(storeId, { userId, tenantId });

  return (
    <SubscriptionSettingsClient
      subscription={subscription}
      editable={editable}
    />
  );
}
