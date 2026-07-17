import type { createApiClient } from "./client";
import {
  teamMemberListSchema,
  teamInvitationListSchema,
  type TeamMemberListResponse,
  type TeamInvitationListResponse,
} from "./schemas/team";

/**
 * Team management. These routes are mounted on marketplace-api's mobile group
 * (under the store prefix so the client resolves them) but are TENANT-scoped —
 * marketplace-api proxies them to platform-api using the bearer's tenant + UID.
 * List endpoints return `{data: [...]}`; mutations return `{data: ...}` which
 * the app doesn't need to read, so they go unschema'd.
 */
export function createTeamApi(client: ReturnType<typeof createApiClient>) {
  return {
    listMembers: () =>
      client.get<TeamMemberListResponse>("/team/members", undefined, teamMemberListSchema),
    updateRole: (email: string, newRole: string) =>
      client.patch("/team/members/role", { email, new_role: newRole }),
    listInvitations: () =>
      client.get<TeamInvitationListResponse>(
        "/team/invitations",
        undefined,
        teamInvitationListSchema,
      ),
    invite: (email: string, role: string) => client.post("/team/invitations", { email, role }),
    revoke: (invitationId: string) => client.delete(`/team/invitations/${invitationId}`),
  };
}

export type { TeamMemberListResponse, TeamInvitationListResponse };
