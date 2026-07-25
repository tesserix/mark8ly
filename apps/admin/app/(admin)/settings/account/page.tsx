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
  const { email, role, tenantId, userId } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
          <AdminPage
        eyebrow="Account"
        title="Account"
        description="Manage your personal profile, security settings, and active sessions."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        <AccountSettingsContent
          userId={userId}
          tenantId={tenantId}
          sessionEmail={email}
          editable={editable}
          isOwner={role === "owner"}
        />
      </AdminPage>
  );
}

async function AccountSettingsContent({
  userId,
  tenantId,
  sessionEmail,
  editable,
  isOwner,
}: {
  userId: string;
  tenantId: string;
  sessionEmail: string;
  editable: boolean;
  isOwner: boolean;
}) {
  const session = { userId, tenantId };
  const [profile, sessions] = await Promise.all([
    getAccountProfile(session),
    listAccountSessions(session),
  ]);

  return (
    <AccountSettingsClient
      profile={profile}
      sessionEmail={sessionEmail}
      sessions={sessions}
      editable={editable}
      isOwner={isOwner}
    />
  );
}
