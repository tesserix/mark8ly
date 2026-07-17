import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createCampaignsApi,
  type CreateCampaignBody,
  type UpdateCampaignBody,
} from "@repo/mobile-shared/api/campaigns";
import { useApiClient } from "@/lib/api-client";

function useCampaignMutation<TVars>(
  run: (api: ReturnType<typeof createCampaignsApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const api = createCampaignsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(api, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["campaigns"] });
    },
  });
}

export function useCreateCampaign() {
  return useCampaignMutation<CreateCampaignBody>((api, body) => api.create(body));
}

export function useUpdateCampaign() {
  return useCampaignMutation<{ id: string; body: UpdateCampaignBody }>((api, { id, body }) =>
    api.patch(id, body),
  );
}

export function useDeleteCampaign() {
  return useCampaignMutation<string>((api, id) => api.remove(id));
}

export function useSendCampaign() {
  return useCampaignMutation<string>((api, id) => api.send(id));
}

export function usePauseCampaign() {
  return useCampaignMutation<string>((api, id) => api.pause(id));
}

export function useResumeCampaign() {
  return useCampaignMutation<string>((api, id) => api.resume(id));
}

export function useScheduleCampaign() {
  return useCampaignMutation<{ id: string; scheduledAt: string }>((api, { id, scheduledAt }) =>
    api.schedule(id, scheduledAt),
  );
}
