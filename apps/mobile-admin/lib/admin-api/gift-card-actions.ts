import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createGiftCardsApi, type IssueGiftCardBody } from "@repo/mobile-shared/api/gift-cards";
import { useApiClient } from "@/lib/api-client";

/** Issue a gift card. The backend exposes no update/delete — issue + read only. */
export function useIssueGiftCard() {
  const client = useApiClient();
  const api = createGiftCardsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: IssueGiftCardBody) => api.issue(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gift-cards"] });
    },
  });
}
