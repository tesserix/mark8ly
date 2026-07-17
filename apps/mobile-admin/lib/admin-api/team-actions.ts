import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createTeamApi } from "@repo/mobile-shared/api/team";
import { useApiClient } from "@/lib/api-client";

function useTeamMutation<TVars>(
  run: (api: ReturnType<typeof createTeamApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const api = createTeamApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(api, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["team"] });
    },
  });
}

export function useInviteTeamMember() {
  return useTeamMutation<{ email: string; role: string }>((api, { email, role }) =>
    api.invite(email, role),
  );
}

export function useRevokeInvitation() {
  return useTeamMutation<string>((api, invitationId) => api.revoke(invitationId));
}

export function useUpdateMemberRole() {
  return useTeamMutation<{ email: string; newRole: string }>((api, { email, newRole }) =>
    api.updateRole(email, newRole),
  );
}
