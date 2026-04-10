import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import {
  subscribeNotify,
  unsubscribeNotify,
} from "@/lib/storefront-api/notify";

interface UseNotifyMeOptions {
  productId: string;
  notifyType?: string;
}

export function useNotifyMe({ productId, notifyType = "back_in_stock" }: UseNotifyMeOptions) {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  const subscribe = useMutation({
    mutationFn: () => subscribeNotify(client, productId, notifyType),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["store", slug, "products", productId],
      });
    },
  });

  const unsubscribe = useMutation({
    mutationFn: () => unsubscribeNotify(client, productId, notifyType),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["store", slug, "products", productId],
      });
    },
  });

  return { subscribe, unsubscribe };
}
