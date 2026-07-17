import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createLoyaltyApi,
  type UpdateLoyaltyProgramBody,
  type AdjustPointsBody,
} from "@repo/mobile-shared/api/loyalty";
import { useApiClient } from "@/lib/api-client";

export function useUpdateLoyaltyProgram() {
  const client = useApiClient();
  const api = createLoyaltyApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: UpdateLoyaltyProgramBody) => api.updateProgram(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["loyalty"] });
    },
  });
}

export function useAdjustPoints() {
  const client = useApiClient();
  const api = createLoyaltyApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: AdjustPointsBody }) => api.adjustPoints(id, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["loyalty"] });
    },
  });
}
