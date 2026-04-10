import type { ApiClient } from "./client-provider";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

interface WishlistResponse {
  data: StorefrontProduct[];
}

interface WishlistMutationResponse {
  data: { id: string };
}

export function listWishlist(
  client: ApiClient,
): Promise<WishlistResponse> {
  return client.get<WishlistResponse>("/wishlist");
}

export function addToWishlist(
  client: ApiClient,
  productId: string,
): Promise<WishlistMutationResponse> {
  return client.post<WishlistMutationResponse>("/wishlist", {
    product_id: productId,
  });
}

export function removeFromWishlist(
  client: ApiClient,
  productId: string,
): Promise<void> {
  return client.delete(`/wishlist/${productId}`);
}
