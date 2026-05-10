import { useQuery } from "@tanstack/react-query";
import type {
  StorefrontCategory,
  StorefrontProduct,
  StorefrontProductDetail,
} from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";

interface PagedProducts {
  items: StorefrontProduct[];
  total: number;
  next_cursor: string | null;
  has_more: boolean;
}

/** Lists products for the merchant's storefront, optionally filtered by category. */
export function useProducts(params?: { category?: string; search?: string }) {
  const api = useStorefrontApi();
  return useQuery<PagedProducts>({
    queryKey: ["products", params?.category ?? null, params?.search ?? null],
    queryFn: () => {
      const q: Record<string, string> = {};
      if (params?.category) q.category = params.category;
      if (params?.search) q.q = params.search;
      return api.get<PagedProducts>("/products", q);
    },
  });
}

export function useProduct(handle: string) {
  const api = useStorefrontApi();
  return useQuery<StorefrontProductDetail>({
    queryKey: ["product", handle],
    queryFn: () => api.get<StorefrontProductDetail>(`/products/${handle}`),
    enabled: !!handle,
  });
}

export function useCategories() {
  const api = useStorefrontApi();
  return useQuery<{ items: StorefrontCategory[] }>({
    queryKey: ["categories"],
    queryFn: () => api.get<{ items: StorefrontCategory[] }>("/categories"),
    staleTime: 5 * 60_000,
  });
}
