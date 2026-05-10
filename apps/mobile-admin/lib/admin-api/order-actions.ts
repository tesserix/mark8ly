import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";
import { useApiClient } from "@/lib/api-client";

export function useConfirmOrder() {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => ordersApi.confirm(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}

export function useFulfillOrder() {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, trackingNumber }: { id: string; trackingNumber: string }) =>
      ordersApi.fulfill(id, trackingNumber),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}

export function useCancelOrder() {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason?: string }) =>
      ordersApi.cancel(id, reason),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}

export function useRefundOrder() {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, amount }: { id: string; amount: number }) =>
      ordersApi.refund(id, amount),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}
