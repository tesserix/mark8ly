import { useQuery } from "@tanstack/react-query";
import { createShipmentsApi } from "@repo/mobile-shared/api/shipments";
import type { Shipment } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

/**
 * The order's shipment (or null when none exists yet). Keyed under the
 * ["orders"] prefix so an order mutation's invalidateQueries(["orders"])
 * refetches it too — a "delivered" transition changes the order's
 * fulfillment status, and vice-versa. `enabled` lets the caller skip the
 * fetch for terminal order statuses where shipping is not actionable.
 */
export function useShipment(orderId: string, enabled = true) {
  const client = useApiClient();
  const shipmentsApi = createShipmentsApi(client);

  return useQuery<Shipment | null>({
    queryKey: ["orders", "shipment", orderId],
    queryFn: () => shipmentsApi.get(orderId),
    enabled: !!orderId && enabled,
  });
}
