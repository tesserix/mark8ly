import { useQuery } from "@tanstack/react-query";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import type { Product, ProductDetail, PaginatedResponse } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

interface ProductListParams {
  status?: string;
  search?: string;
  low_stock?: boolean;
}

export function useProducts(params?: ProductListParams) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<PaginatedResponse<Product>>({
    queryKey: ["products", params?.status, params?.search, params?.low_stock],
    queryFn: () =>
      productsApi.list({
        ...(params?.status ? { status: params.status } : {}),
        ...(params?.search ? { search: params.search } : {}),
        ...(params?.low_stock ? { low_stock: "true" } : {}),
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
