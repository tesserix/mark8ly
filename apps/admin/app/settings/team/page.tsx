import { AdminShell } from "@/components/shell/AdminShell";
import { TeamSettings } from "@/components/settings/TeamSettings";
import {
  canInviteMembers,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  listPendingInvitations,
  listTeamMembers,
} from "@/lib/api/platform-api";

/**
 * /settings/team — current members, pending invitations, and the
 * invite form. Owners and admins see the invite CTA; staff/viewer see
 * a read-only view. The "current members" list is the tenant owner
 * plus every accepted invitation — sourced from platform-api's
 * `/internal/tenants/{id}/members` endpoint which joins the tenants
 * row with accepted invitations rows.
 */
export default async function TeamSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    stores,
  } = await getServerSessionContext();
  const canInvite = canInviteMembers(role);

  // Fetch members + pending invitations in parallel. Both endpoints
  // fail-soft to an empty array so a transient platform-api hiccup
  // degrades the page instead of 500-ing it.
  const [members, invitations] = await Promise.all([
    listTeamMembers(tenantId).catch(() => []),
    listPendingInvitations(tenantId).catch(() => []),
  ]);
  // Current user email is passed so the members list can hide the
  // role-change dropdown on the row belonging to the signed-in user
  // (no self-demotion). Role is already in-scope.
  const currentUserEmail = email;

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
          members={members}
          invitations={invitations}
          canInvite={canInvite}
          currentRole={role}
          currentUserEmail={currentUserEmail}
          stores={stores}
        />
      </div>
    </AdminShell>
  );
}
