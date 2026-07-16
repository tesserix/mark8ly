import { useQuery } from "@tanstack/react-query";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import type { ProductDetail } from "@repo/mobile-shared/api/types";
import type { ProductListResponse } from "@repo/mobile-shared/api/schemas/products";
import { useApiClient } from "@/lib/api-client";

interface ProductListParams {
  status?: string;
  search?: string;
}

// The backend defaults to page_size=20 (validation.go:329-336) and the store
// has 161 products, so the default silently hides 141 of them. 100 is the
// backend's max (validation.go:325). This is an interim ceiling: the app has
// no pagination yet, so anything past 100 is still unreachable. Real
// infinite scroll is a follow-up.
const PAGE_SIZE = "100";

export function useProducts(params?: ProductListParams) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<ProductListResponse>({
    queryKey: ["products", params?.status, params?.search],
    queryFn: () =>
      productsApi.list({
        page_size: PAGE_SIZE,
        ...(params?.status ? { status: params.status } : {}),
        ...(params?.search ? { search: params.search } : {}),
      }),
    refetchOnWindowFocus: true,
  });
}

export function useProduct(id: string) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<ProductDetail>({
    queryKey: ["product", id],
    queryFn: () => productsApi.get(id),
    enabled: !!id,
  });
}
