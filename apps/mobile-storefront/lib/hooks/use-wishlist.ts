import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import type { WishlistItem } from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";

interface WishlistList {
  items: WishlistItem[];
  total: number;
}

export function useWishlist() {
  const api = useStorefrontApi();
  const { user } = useAuth();
  return useQuery<WishlistList>({
    queryKey: ["wishlist"],
    queryFn: () => api.get<WishlistList>("/wishlist"),
    enabled: !!user,
  });
}

export function useWishlistContains(productId: string) {
  const api = useStorefrontApi();
  const { user } = useAuth();
  return useQuery<{ in_wishlist: boolean }>({
    queryKey: ["wishlist", "contains", productId],
    queryFn: () => api.get<{ in_wishlist: boolean }>(`/wishlist/check/${productId}`),
    enabled: !!user && !!productId,
    staleTime: 60_000,
  });
}

export function useAddToWishlist() {
  const api = useStorefrontApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (productId: string) => api.post<void>("/wishlist", { product_id: productId }),
    onSuccess: (_data, productId) => {
      queryClient.invalidateQueries({ queryKey: ["wishlist"] });
      queryClient.setQueryData(["wishlist", "contains", productId], { in_wishlist: true });
    },
  });
}

export function useRemoveFromWishlist() {
  const api = useStorefrontApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (productId: string) => api.delete<void>(`/wishlist/${productId}`),
    onSuccess: (_data, productId) => {
      queryClient.invalidateQueries({ queryKey: ["wishlist"] });
      queryClient.setQueryData(["wishlist", "contains", productId], { in_wishlist: false });
    },
  });
}
