import { AdminShell } from "@/components/shell/AdminShell";
import { TeamSettings } from "@/components/settings/TeamSettings";
import {
  canInviteMembers,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listPendingInvitations } from "@/lib/api/platform-api";

/**
 * Admin /settings/team — Phase P.
 *
 * Lists the tenant owner (hardcoded first row from tenant.owner_email)
 * and any pending invitations. Owners and admins see an "Invite
 * teammate" CTA; staff/viewer see a read-only list. There is no
 * "current members" list yet — FGA ListUsers or a parallel Postgres
 * staff table is a separate slice.
 */
export default async function TeamSettingsPage() {
  const {
    tenantName,
    email,
    tenant,
    role,
    memberships,
    tenantId,
    stores,
  } = await getServerSessionContext();
  const canInvite = canInviteMembers(role);

  const invitations = await listPendingInvitations(tenantId).catch(() => []);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-6">
        <header className="admin-surface rounded-[2rem] px-6 py-7 sm:px-8">
          <p className="admin-eyebrow">Access control</p>
          <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground">
            Team
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
            Invite teammates, assign the right level of access, and keep
            store ownership clear as more people start helping run the
            business.
          </p>
          {!canInvite && (
            <p className="mt-4 inline-flex rounded-full border border-amber-300 bg-amber-50 px-3 py-1 text-xs text-amber-900">
              Read-only: your role ({role}) can view the team but cannot
              invite or revoke teammates.
            </p>
          )}
        </header>

        <TeamSettings
          ownerEmail={tenant?.owner_email ?? ""}
          invitations={invitations}
          canInvite={canInvite}
          stores={stores}
        />
      </div>
    </AdminShell>
  );
}
