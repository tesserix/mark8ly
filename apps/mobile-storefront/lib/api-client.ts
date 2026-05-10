import { useMemo } from "react";
import { createStorefrontClient } from "@repo/mobile-shared/api/storefront-client";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { getMerchant } from "@/lib/merchant";

/**
 * Storefront API client hook. Pre-binds every call to the merchant's
 * store slug (baked in at build time) and forwards the customer's GIP
 * id_token when they're signed in. Anonymous browse works without a
 * token — only account/cart-on-server endpoints require auth.
 *
 * Self-correction:
 *   - 401 → force a fresh GIP id_token via refreshToken, retry. Still
 *     401? signOut so the AuthGate routes back to /sign-in.
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
