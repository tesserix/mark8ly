import { AdminShell } from "@/components/shell/AdminShell";
import { TeamSettings } from "@/components/settings/TeamSettings";
import {
  canInviteMembers,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listPendingInvitations } from "@/lib/api/platform-api";

/**
 * /settings/team — owner row plus pending invitations. Owners and
 * admins see an "Invite teammate" CTA; staff/viewer see a read-only
 * list. There is no "current members" list yet — that's a separate
 * slice that needs FGA ListUsers or a parallel staff table.
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
      <div className="mx-auto w-full max-w-5xl space-y-12">
        <header className="space-y-3">
          <p className="eyebrow">Access control</p>
          <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
            Team
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Invite teammates, assign the right level of access, and keep store
            ownership clear as more people start helping run the business.
          </p>
          {!canInvite && (
            <p className="text-sm text-warning">
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
