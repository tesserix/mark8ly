import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createReviewsApi } from "@repo/mobile-shared/api/reviews";
import { useApiClient } from "@/lib/api-client";

/**
 * Invalidate the list + the single review so both reflect the mutation, and
 * ["dashboard"] with them: the dashboard payload carries its own
 * `stats.pending_reviews`, which is what drives the queue's "See all N
 * pending reviews" row. Approving a review left that count one too high until
 * something else happened to refetch the dashboard.
 */
function useReviewMutation<TVars>(
  run: (api: ReturnType<typeof createReviewsApi>, vars: TVars) => Promise<unknown>,
  idOf: (vars: TVars) => string,
) {
  const client = useApiClient();
  const reviewsApi = createReviewsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(reviewsApi, vars),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ["reviews"] });
      queryClient.invalidateQueries({ queryKey: ["review", idOf(vars)] });
      queryClient.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });
}

export function useApproveReview() {
  return useReviewMutation<string>((api, id) => api.approve(id), (id) => id);
}

export function useRejectReview() {
  return useReviewMutation<string>((api, id) => api.reject(id), (id) => id);
}

export function useToggleReviewFeatured() {
  return useReviewMutation<{ id: string; featured: boolean }>(
    (api, { id, featured }) => api.setFeatured(id, featured),
    ({ id }) => id,
  );
}

export function useReplyToReview() {
  return useReviewMutation<{ id: string; content: string }>(
    (api, { id, content }) => api.reply(id, content),
    ({ id }) => id,
  );
}
