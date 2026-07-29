import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createGiftCardsApi, type IssueGiftCardBody } from "@repo/mobile-shared/api/gift-cards";
import type { GiftCardStatusTarget } from "@/lib/gift-card-status";
import { useApiClient } from "@/lib/api-client";

/** Issue a gift card. */
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

export interface SetGiftCardStatusVars {
  id: string;
  status: GiftCardStatusTarget;
}

/**
 * Enable or disable one gift card.
 *
 * Which cards MAY be toggled is `canSetGiftCardStatus` in
 * `lib/gift-card-status.ts` — a dependency-free module precisely so a caller
 * can ask that question without booting this one's api/auth chain.
 *
 * NO optimistic update and no optimistic hide. `["gift-cards"]` is a strict
 * prefix of BOTH of this feature's query keys — `["gift-cards", "list",
 * status]` and `["gift-cards", "detail", id]` — so react-query's default
 * prefix matching makes the list and that card's detail stale in one call,
 * and the refetch is authoritative about the badge. A second
 * `invalidateQueries` naming the detail key explicitly would be a no-op that
 * reads as a second, narrower rule.
 *
 * `["dashboard"]` is deliberately NOT invalidated: nothing in the dashboard
 * payload describes gift cards (see api/schemas/dashboard.ts), so it would
 * be a refetch for nothing.
 */
export function useSetGiftCardStatus() {
  const client = useApiClient();
  const api = createGiftCardsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, status }: SetGiftCardStatusVars) =>
      status === "active" ? api.enable(id) : api.disable(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gift-cards"] });
    },
  });
}
