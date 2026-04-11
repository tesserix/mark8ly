import { AdminShell } from "@/components/shell/AdminShell";
import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getSubscription } from "@/lib/api/settings-tier2-api";
import { SubscriptionSettingsClient } from "@/components/settings/SubscriptionSettingsClient";

/**
 * /settings/subscription — plan management and billing.
 */
export default async function SubscriptionSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, userId, currentStore } =
    await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <AdminPage
        eyebrow="Account"
        title="Subscription"
        description="Manage your plan, view billing details, and compare available tiers."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
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
      </AdminPage>
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
