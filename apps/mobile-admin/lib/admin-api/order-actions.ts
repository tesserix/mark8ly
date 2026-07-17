import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createOrdersApi, type ConfirmOrderBody, type RefundOrderBody } from "@repo/mobile-shared/api/orders";
import { useApiClient } from "@/lib/api-client";

/** Every order mutation invalidates the ["orders"] prefix (list + detail). */
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
