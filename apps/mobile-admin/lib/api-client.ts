import { useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useAuthNoticeStore } from "@repo/mobile-shared/stores/auth-notice";
import { zitadelSession } from "@repo/mobile-shared/auth/zitadel-session";
import { isZitadelProvider } from "@/lib/auth-provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useEnvironment } from "@repo/mobile-shared/config/env";
import { createDemoApiClient } from "./demo-api-client";

// Demo/simulator builds have no real GIP token, so the real API would 401
// and bounce the user back to /login. Serve canned data instead.
const DEMO_MODE = process.env.EXPO_PUBLIC_AUTH_BACKEND === "demo";
const demoClient = DEMO_MODE ? createDemoApiClient() : null;

/**
 * Shared API client hook used by every admin data hook.
 *
 * Beyond plumbing the env baseUrl + bearer token, this is also where we
 * mirror the admin web's "validate and self-correct" loop:
 *
 *   • 401 → force a fresh GIP id_token, retry once. Still 401? sign out.
 *   • 403/404 on a `/stores/{id}/...` path → drop the active store and
 *     invalidate the stores list so the TenantGate re-resolves which
 *     tenant the user can actually use, the same way admin middleware
 *     redirects a member-revoked user back to /pick-tenant.
 */
export function useApiClient() {
  const { getToken, refreshToken, signOut } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  const tenantId = useTenantStore((s) => s.tenantId);
  const clearActiveStore = useTenantStore((s) => s.clearActiveStore);
  const env = useEnvironment();
  const queryClient = useQueryClient();

  const realClient = useMemo(
    () =>
      createApiClient({
        baseUrl: env.apiBaseUrl,
        // Under Zitadel the bearer is a plain token this app persisted at
        // sign-in, not something a Firebase SDK mints on demand. Falls back
        // to the GIP getter so a non-Zitadel build is unchanged.
        getToken: isZitadelProvider()
          ? () => zitadelSession.accessTokenIfFresh()
          : getToken,
        refreshToken,
        getStoreId: () => activeStore?.id ?? null,
        // Tenancy for a Zitadel token, which carries no tenant claim
        // (#686). Read from the tenant slot, never from activeStore — see
        // tenant-store.ts for why those must not be conflated.
        getActingTenantId: () => tenantId ?? null,
        onUnauthorized: async (reason) => {
          // Record WHY before tearing the session down — /login reads this and
          // explains itself instead of bouncing the user with no message.
          useAuthNoticeStore.getState().setNotice(reason);
          await signOut();
        },
        onTenantInvalid: async () => {
          // Active store no longer accessible to this user. Drop it and
          // force the resolver to re-pick from the (refetched) list.
          await clearActiveStore();
          await queryClient.invalidateQueries({ queryKey: ["stores"] });
        },
      }),
    [
      env.apiBaseUrl,
      getToken,
      refreshToken,
      activeStore?.id,
      tenantId,
      clearActiveStore,
      signOut,
      queryClient,
    ],
  );

  // Demo build: serve canned data instead of the real (network) client, so
  // there's no 401 → signOut bounce. `demoClient` is a build-time constant,
  // so hook order stays stable across renders.
  return demoClient ?? realClient;
}
