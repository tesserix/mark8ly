import type { ApiClient } from "./client-provider";
import type {
  StorefrontProduct,
  StorefrontProductDetail,
} from "@repo/mobile-shared/api/storefront-types";

interface ListProductsOptions {
  search?: string;
  category?: string;
  sort?: string;
  page?: number;
  limit?: number;
}

interface ProductListResponse {
  data: StorefrontProduct[];
  meta: { total: number };
}

interface ProductDetailResponse {
  data: StorefrontProductDetail;
}

export function listProducts(
  client: ApiClient,
  options?: ListProductsOptions,
): Promise<ProductListResponse> {
  const params: Record<string, string> = {};
  if (options?.search) params.search = options.search;
  if (options?.category) params.category = options.category;
  if (options?.sort) params.sort = options.sort;
  if (options?.page) params.page = String(options.page);
  if (options?.limit) params.limit = String(options.limit);

  return client.get<ProductListResponse>("/products", params);
}

export function getProductByHandle(
  client: ApiClient,
  handle: string,
): Promise<ProductDetailResponse> {
  return client.get<ProductDetailResponse>(`/products/${handle}`);
}
