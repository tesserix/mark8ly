import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import {
  listCategories,
  listProductsByCategory,
} from "@/lib/storefront-api/categories";
import type {
  StorefrontCategory,
  StorefrontProduct,
} from "@repo/mobile-shared/api/storefront-types";

const PAGE_SIZE = 20;

export function useCategories() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery<StorefrontCategory[]>({
    queryKey: ["store", slug, "categories"],
    queryFn: async () => {
      const result = await listCategories(client);
      return result.data;
    },
  });
}

interface UseCategoryProductsOptions {
  sort?: string;
}

interface CategoryProductsPage {
  products: StorefrontProduct[];
  total: number;
  nextPage: number | undefined;
}

export function useCategoryProducts(
  categorySlug: string,
  options?: UseCategoryProductsOptions,
) {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useInfiniteQuery<CategoryProductsPage>({
    queryKey: ["store", slug, "categories", categorySlug, "products", options],
    queryFn: async ({ pageParam }) => {
      const page = pageParam as number;
      const result = await listProductsByCategory(client, categorySlug, {
        sort: options?.sort,
        page,
        limit: PAGE_SIZE,
      });
      const fetched = page * PAGE_SIZE;
      const hasMore = fetched + result.data.length < result.meta.total;
      return {
        products: result.data,
        total: result.meta.total,
        nextPage: hasMore ? page + 1 : undefined,
      };
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage) => lastPage.nextPage,
    enabled: !!categorySlug,
  });
}
