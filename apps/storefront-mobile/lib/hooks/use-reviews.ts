import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import { listMyReviews, submitReview } from "@/lib/storefront-api/reviews";
import type { SubmitReviewBody } from "@/lib/storefront-api/reviews";

export function useMyReviews() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "my-reviews"],
    queryFn: () => listMyReviews(client),
    select: (res) => res.data,
  });
}

export function useSubmitReview() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ handle, body }: { handle: string; body: SubmitReviewBody }) =>
      submitReview(client, handle, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "my-reviews"] });
    },
  });
}
