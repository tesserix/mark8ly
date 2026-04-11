import { AdminPage, ReadOnlyNotice } from "@/components/layout";
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
 * /settings/team — current members, pending invitations, and the invite
 * form. Owners and admins see the invite CTA; staff/viewer see a read-only
 * view. The "current members" list is the tenant owner plus every accepted
 * invitation — sourced from platform-api's `/internal/tenants/{id}/members`
 * endpoint which joins the tenants row with accepted invitations rows.
 */
export default async function TeamSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, stores } =
    await getServerSessionContext();
  const canInvite = canInviteMembers(role);

  // Fetch members + pending invitations in parallel. Both endpoints
  // fail-soft to an empty array so a transient platform-api hiccup
  // degrades the page instead of 500-ing it.
  const [members, invitations] = await Promise.all([
    listTeamMembers(tenantId).catch(() => []),
    listPendingInvitations(tenantId).catch(() => []),
  ]);

  return (
          <AdminPage
        eyebrow="Team & access"
        title="Team"
        description="Invite teammates, assign the right level of access, and keep store ownership clear as more people start helping run the business."
        readOnlyNotice={
          !canInvite && role ? <ReadOnlyNotice role={role} /> : undefined
        }
      >
        <TeamSettings
          members={members}
          invitations={invitations}
          canInvite={canInvite}
          currentRole={role}
          currentUserEmail={email}
          stores={stores}
        />
      </AdminPage>
  );
}
