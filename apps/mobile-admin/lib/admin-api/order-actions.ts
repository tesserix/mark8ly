import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createOrdersApi, type ConfirmOrderBody, type RefundOrderBody } from "@repo/mobile-shared/api/orders";
import { useApiClient } from "@/lib/api-client";

/**
 * Every order mutation invalidates the ["orders"] prefix (list + detail) AND
 * ["dashboard"].
 *
 * The dashboard is a SEPARATE query key that carries its own copy of the same
 * facts — `recent_orders`, `stats.orders_pending`/`_fulfilled`/`_cancelled`
 * and today's revenue all move when an order is confirmed or cancelled.
 * Without this second invalidation the Dashboard's action queue was never
 * refetched after an approve at all: it kept serving the pre-confirm payload
 * for the life of the screen, with the just-approved order still labelled
 * Pending. `onSuccess` deliberately does NOT return these promises — see
 * `useExpireHidesOnFreshAnswer` in app/(tabs)/index.tsx, which needs the
 * refetch to land AFTER `isPending` goes false.
 */
function useOrderMutation<TVars>(
  run: (api: ReturnType<typeof createOrdersApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(ordersApi, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });
}

export function useConfirmOrder() {
  return useOrderMutation<{ id: string; body?: ConfirmOrderBody }>((api, { id, body }) =>
    api.confirm(id, body),
  );
}

export function useFulfillOrder() {
  return useOrderMutation<string>((api, id) => api.fulfill(id));
}

export function useCancelOrder() {
  return useOrderMutation<{ id: string; reason: string }>((api, { id, reason }) =>
    api.cancel(id, reason),
  );
}

export function useRefundOrder() {
  return useOrderMutation<{ id: string; body: RefundOrderBody }>((api, { id, body }) =>
    api.refund(id, body),
  );
}
