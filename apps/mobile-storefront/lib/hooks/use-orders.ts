import { useQuery } from "@tanstack/react-query";
import type {
  StorefrontOrderDetail,
  StorefrontOrderSummary,
} from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";
import { useAuth } from "@repo/mobile-shared/auth/provider";

interface OrdersList {
  items: StorefrontOrderSummary[];
  total: number;
  next_cursor: string | null;
  has_more: boolean;
}

export function useOrders() {
  const api = useStorefrontApi();
  const { user } = useAuth();
  return useQuery<OrdersList>({
    queryKey: ["orders"],
    queryFn: () => api.get<OrdersList>("/orders"),
    enabled: !!user,
  });
}

export function useOrder(id: string) {
  const api = useStorefrontApi();
  const { user } = useAuth();
  return useQuery<StorefrontOrderDetail>({
    queryKey: ["order", id],
    queryFn: () => api.get<StorefrontOrderDetail>(`/orders/${id}`),
    enabled: !!user && !!id,
  });
}
