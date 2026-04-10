import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import { listProducts, getProductByHandle } from "@/lib/storefront-api/products";
import type {
  StorefrontProduct,
  StorefrontProductDetail,
} from "@repo/mobile-shared/api/storefront-types";

const PAGE_SIZE = 20;

interface UseProductsOptions {
  search?: string;
  category?: string;
  sort?: string;
}

interface ProductsPage {
  products: StorefrontProduct[];
  total: number;
  nextPage: number | undefined;
}

export function useProducts(options?: UseProductsOptions) {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useInfiniteQuery<ProductsPage>({
    queryKey: ["store", slug, "products", options],
    queryFn: async ({ pageParam }) => {
      const page = pageParam as number;
      const result = await listProducts(client, {
        search: options?.search,
        category: options?.category,
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
  });
}

export function useProductByHandle(handle: string) {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery<StorefrontProductDetail>({
    queryKey: ["store", slug, "products", handle],
    queryFn: async () => {
      const result = await getProductByHandle(client, handle);
      return result.data;
    },
    enabled: !!handle,
  });
}
