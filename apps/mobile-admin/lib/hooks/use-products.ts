import { useQuery } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { Product, ProductDetail, PaginatedResponse } from "@repo/mobile-shared/api/types";

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

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
        ...(params?.low_stock ? { low_stock: params.low_stock } : {}),
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
