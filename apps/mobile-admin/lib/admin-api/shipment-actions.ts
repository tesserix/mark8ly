import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createShipmentsApi,
  type CreateShipmentBody,
  type SchedulePickupBody,
  type UpdateShipmentStatusBody,
} from "@repo/mobile-shared/api/shipments";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";
import { useApiClient } from "@/lib/api-client";

/**
 * Every shipment mutation invalidates the ["orders"] prefix so both the
 * shipment query and the parent order detail (its fulfillment status +
 * lifecycle move on a "delivered" transition) refetch together.
 */
function useShipmentMutation<TVars>(
  run: (api: ReturnType<typeof createShipmentsApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const shipmentsApi = createShipmentsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(shipmentsApi, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}

export function useCreateShipment() {
  return useShipmentMutation<{ orderId: string; body: CreateShipmentBody }>(
    (api, { orderId, body }) => api.create(orderId, body),
  );
}

export function useUpdateShipmentStatus() {
  return useShipmentMutation<{ orderId: string; shipmentId: string; body: UpdateShipmentStatusBody }>(
    (api, { orderId, shipmentId, body }) => api.updateStatus(orderId, shipmentId, body),
  );
}

export function useRefreshTracking() {
  return useShipmentMutation<{ orderId: string; shipmentId: string }>(
    (api, { orderId, shipmentId }) => api.refreshTracking(orderId, shipmentId),
  );
}

export function useSchedulePickup() {
  return useShipmentMutation<{ orderId: string; shipmentId: string; body?: SchedulePickupBody }>(
    (api, { orderId, shipmentId, body }) => api.schedulePickup(orderId, shipmentId, body),
  );
}

export function useDeleteShipment() {
  return useShipmentMutation<{ orderId: string; shipmentId: string }>(
    (api, { orderId, shipmentId }) => api.remove(orderId, shipmentId),
  );
}

export function useEmailLabel() {
  return useShipmentMutation<{ orderId: string; shipmentId: string; recipient: string }>(
    (api, { orderId, shipmentId, recipient }) => api.emailLabel(orderId, shipmentId, recipient),
  );
}

/**
 * Manual "Cancel / return shipment" — bypasses `useShipmentMutation`'s
 * generic `Promise<unknown>` signature so the mutation's `data` stays typed
 * as `CancelShipmentResult` (callers read `.status`/`.reason` from
 * `onSuccess`). A non-"succeeded" `status` is a normal response, not a
 * thrown `ApiError` — see `shipments.ts`'s `cancel()` doc comment.
 */
export function useCancelShipment() {
  const client = useApiClient();
  const shipmentsApi = createShipmentsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ orderId, shipmentId }: { orderId: string; shipmentId: string }) =>
      shipmentsApi.cancel(orderId, shipmentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}

/**
 * Invoice / receipt document resend. Not a shipment mutation — hits the
 * orders api — but shares the ["orders"] invalidation so any downstream
 * document-state read refreshes. No detail refetch is strictly needed; the
 * invalidation keeps the pattern uniform.
 */
function useDocumentEmail(kind: "invoice" | "receipt") {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ orderId, note }: { orderId: string; note?: string }) =>
      kind === "invoice"
        ? ordersApi.emailInvoice(orderId, note)
        : ordersApi.emailReceipt(orderId, note),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orders"] });
    },
  });
}

export function useEmailInvoice() {
  return useDocumentEmail("invoice");
}

export function useEmailReceipt() {
  return useDocumentEmail("receipt");
}
