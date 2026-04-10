import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import {
  listWishlist,
  addToWishlist,
  removeFromWishlist,
} from "@/lib/storefront-api/wishlist";

export function useWishlist() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "wishlist"],
    queryFn: () => listWishlist(client),
    select: (res) => res.data,
  });
}

export function useAddToWishlist() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (productId: string) => addToWishlist(client, productId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "wishlist"] });
    },
  });
}

export function useRemoveFromWishlist() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (productId: string) => removeFromWishlist(client, productId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "wishlist"] });
    },
  });
}

export function useIsWishlisted(productId: string): boolean {
  const { data: items } = useWishlist();
  if (!items) return false;
  return items.some((item) => item.id === productId);
}
