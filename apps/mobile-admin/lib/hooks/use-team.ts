import { useQuery } from "@tanstack/react-query";
import { createTeamApi } from "@repo/mobile-shared/api/team";
import type {
  TeamMemberListResponse,
  TeamInvitationListResponse,
} from "@repo/mobile-shared/api/schemas/team";
import { useApiClient } from "@/lib/api-client";

export function useTeamMembers() {
  const client = useApiClient();
  const api = createTeamApi(client);

  return useQuery<TeamMemberListResponse>({
    queryKey: ["team", "members"],
    queryFn: () => api.listMembers(),
    refetchOnWindowFocus: true,
  });
}

export function useTeamInvitations() {
  const client = useApiClient();
  const api = createTeamApi(client);

  return useQuery<TeamInvitationListResponse>({
    queryKey: ["team", "invitations"],
    queryFn: () => api.listInvitations(),
    refetchOnWindowFocus: true,
  });
}
