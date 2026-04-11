import { AdminShell } from "@/components/shell/AdminShell";
import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getAccountProfile,
  listAccountSessions,
} from "@/lib/api/settings-tier2-api";
import { AccountSettingsClient } from "@/components/settings/AccountSettingsClient";

/**
 * /settings/account — account profile, MFA, sessions, and danger zone.
 */
export default async function AccountSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, userId } =
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
        title="Account"
        description="Manage your personal profile, security settings, and active sessions."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        <AccountSettingsContent
          userId={userId}
          tenantId={tenantId}
          editable={editable}
        />
      </AdminPage>
    </AdminShell>
  );
}

async function AccountSettingsContent({
  userId,
  tenantId,
  editable,
}: {
  userId: string;
  tenantId: string;
  editable: boolean;
}) {
  const session = { userId, tenantId };
  const [profile, sessions] = await Promise.all([
    getAccountProfile(session),
    listAccountSessions(session),
  ]);

  return (
    <AccountSettingsClient
      profile={profile}
      sessions={sessions}
      editable={editable}
    />
  );
}
