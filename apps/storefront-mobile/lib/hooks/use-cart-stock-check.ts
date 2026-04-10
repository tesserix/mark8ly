import { useQueries } from "@tanstack/react-query";
import { useMemo } from "react";
import { useApiClient, useStoreSlug } from "./use-api-client";
import { getProductByHandle } from "@/lib/storefront-api/products";
import type { CartItem } from "@/stores/cart-store";

interface StockInfo {
  available: number;
  status: string;
}

export function useCartStockCheck(cartItems: readonly CartItem[]) {
  const client = useApiClient();
  const slug = useStoreSlug();

  const queries = useQueries({
    queries: cartItems.map((item) => ({
      queryKey: ["store", slug, "products", item.handle, "stock-check"],
      queryFn: () => getProductByHandle(client, item.handle),
      staleTime: 30_000,
    })),
  });

  const isLoading = queries.some((q) => q.isLoading);
  const isRefetching = queries.some((q) => q.isRefetching);

  const stockMap = useMemo(() => {
    const map = new Map<string, StockInfo>();

    for (let i = 0; i < cartItems.length; i++) {
      const item = cartItems[i];
      const query = queries[i];
      if (!query?.data) continue;

      const product = query.data.data;
      const variant = product.variants.find((v) => v.id === item.variantId);

      if (variant) {
        map.set(item.variantId, {
          available: variant.stock_quantity,
          status: variant.stock_status,
        });
      } else {
        map.set(item.variantId, {
          available: product.stock_quantity,
          status: product.stock_status,
        });
      }
    }

    return map;
  }, [cartItems, queries]);

  const hasOutOfStock = useMemo(() => {
    for (const item of cartItems) {
      const info = stockMap.get(item.variantId);
      if (info && (info.status === "out_of_stock" || info.available < item.qty)) {
        return true;
      }
    }
    return false;
  }, [cartItems, stockMap]);

  const refetchAll = () => {
    for (const q of queries) {
      q.refetch();
    }
  };

  return { stockMap, isLoading, isRefetching, hasOutOfStock, refetchAll };
}
