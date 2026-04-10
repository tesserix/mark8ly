import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getNotificationPreferences } from "@/lib/api/settings-tier2-api";
import { NotificationSettingsClient } from "@/components/settings/NotificationSettingsClient";

/**
 * /settings/notifications — notification preference toggles.
 *
 * Server component fetches current preferences and renders the
 * toggle grid via client component.
 */
export default async function NotificationSettingsPage() {
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
            Notifications
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Choose which events you want to be notified about. Toggle each
            notification type on or off.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view notification preferences but
              cannot edit them.
            </p>
          )}
        </header>

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
      </div>
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
    <NotificationSettingsClient
      preferences={preferences}
      editable={editable}
    />
  );
}
