import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createSegmentsApi,
  type CreateSegmentBody,
  type UpdateSegmentBody,
} from "@repo/mobile-shared/api/segments";
import { useApiClient } from "@/lib/api-client";

function useSegmentMutation<TVars>(
  run: (api: ReturnType<typeof createSegmentsApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const api = createSegmentsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(api, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["segments"] });
      // Campaigns reference segments as their audience — keep the campaign
      // audience picker fresh after a segment change.
      queryClient.invalidateQueries({ queryKey: ["campaigns"] });
    },
  });
}

export function useCreateSegment() {
  return useSegmentMutation<CreateSegmentBody>((api, body) => api.create(body));
}

export function useUpdateSegment() {
  return useSegmentMutation<{ id: string; body: UpdateSegmentBody }>((api, { id, body }) =>
    api.update(id, body),
  );
}

export function useDeleteSegment() {
  return useSegmentMutation<string>((api, id) => api.remove(id));
}
