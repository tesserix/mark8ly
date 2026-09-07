import { useMemo } from "react";
import { createStorefrontClient } from "@repo/mobile-shared/api/storefront-client";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { getMerchant } from "@/lib/merchant";

/**
 * Storefront API client hook. Pre-binds every call to the merchant's
 * store slug (baked in at build time) and forwards the customer's bearer
 * token when they're signed in. Anonymous browse works without a token —
 * only account/cart-on-server endpoints require auth.
 *
 * Self-correction:
 *   - 401 → `refreshToken` re-reads the persisted session token and the
 *     call is retried. There is no force-refresh on this path: the stored
 *     token is the only one there is, so a lapsed token yields null and the
 *     retry 401s again. Still 401? signOut, and the AuthGate routes back to
 *     /sign-in.
 *
 * Note: customer sign-in is not available in this build (see app/sign-in.tsx),
 * so in practice every call here is currently anonymous. The token plumbing is
 * kept intact rather than ripped out — it is provider-agnostic and is what a
 * future customer auth flow will feed.
 */
export function useStorefrontApi() {
  const { getToken, refreshToken, signOut } = useAuth();
  const merchant = getMerchant();

  return useMemo(
    () =>
      createStorefrontClient({
        baseUrl: merchant.apiBaseUrl,
        storeSlug: merchant.defaultStoreSlug,
        getToken,
        refreshToken,
        onUnauthorized: () => signOut(),
      }),
    [merchant.apiBaseUrl, merchant.defaultStoreSlug, getToken, refreshToken, signOut],
  );
}
