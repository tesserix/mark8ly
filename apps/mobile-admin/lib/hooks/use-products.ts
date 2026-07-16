import { useQuery } from "@tanstack/react-query";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import type { ProductDetail } from "@repo/mobile-shared/api/types";
import type { ProductListResponse } from "@repo/mobile-shared/api/schemas/products";
import { useApiClient } from "@/lib/api-client";

interface ProductListParams {
  status?: string;
  search?: string;
}

export function useProducts(params?: ProductListParams) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<ProductListResponse>({
    queryKey: ["products", params?.status, params?.search],
    queryFn: () =>
      productsApi.list({
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
