import { AdminShell } from "@/components/shell/AdminShell";
import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getNotificationPreferences } from "@/lib/api/settings-tier2-api";
import { NotificationSettingsClient } from "@/components/settings/NotificationSettingsClient";

/**
 * /settings/notifications — notification preference toggles.
 */
export default async function NotificationSettingsPage() {
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
        title="Notifications"
        description="Choose which events you want to be notified about. Toggle each notification type on or off."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        {currentStore ? (
          <NotificationsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store to manage notifications.
          </p>
        )}
      </AdminPage>
    </AdminShell>
  );
}

async function NotificationsContent({
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
  const preferences = await getNotificationPreferences(storeId, {
    userId,
    tenantId,
  });

  return (
    <NotificationSettingsClient preferences={preferences} editable={editable} />
  );
}
