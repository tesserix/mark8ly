import { AdminShell } from "@/components/shell/AdminShell";
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
 *
 * Server component fetches account data and renders the interactive
 * client component for inline editing.
 */
export default async function AccountSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
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
            Account
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Manage your personal profile, security settings, and active sessions.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view account settings but cannot
              edit them.
            </p>
          )}
        </header>

        <AccountSettingsContent userId={userId} tenantId={tenantId} editable={editable} />
      </div>
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
