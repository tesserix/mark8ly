import type { ApiClient } from "./client-provider";
import type {
  StorefrontCategory,
  StorefrontProduct,
} from "@repo/mobile-shared/api/storefront-types";

interface CategoryListResponse {
  data: StorefrontCategory[];
}

interface CategoryProductsResponse {
  data: StorefrontProduct[];
  meta: { total: number };
}

interface ListByCategoryOptions {
  sort?: string;
  page?: number;
  limit?: number;
}

export function listCategories(
  client: ApiClient,
): Promise<CategoryListResponse> {
  return client.get<CategoryListResponse>("/categories");
}

export function listProductsByCategory(
  client: ApiClient,
  slug: string,
  options?: ListByCategoryOptions,
): Promise<CategoryProductsResponse> {
  const params: Record<string, string> = {};
  if (options?.sort) params.sort = options.sort;
  if (options?.page) params.page = String(options.page);
  if (options?.limit) params.limit = String(options.limit);

  return client.get<CategoryProductsResponse>(
    `/categories/${slug}/products`,
    params,
  );
}
