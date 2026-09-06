import { useMemo, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { useAuth, type AuthUser } from "@repo/mobile-shared/auth/provider";
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
 * Did a session ever exist on this device?
 *
 * Under Zitadel that is the persisted token record — present even once
 * EXPIRED, which is exactly the case where "your session ended" is both true
 * and useful. Under GIP the Firebase SDK owns persistence, so the provider's
 * user is the signal (it is always null under Zitadel, which is why this
 * cannot be one shared check).
 */
async function hadSession(gipUser: AuthUser | null): Promise<boolean> {
  if (!isZitadelProvider()) return gipUser !== null;
  return (await zitadelSession.read()) !== null;
}

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
  const { user, getToken, refreshToken, signOut } = useAuth();
  // Read at 401 time rather than captured into the memo below, so an
  // auth-state change does not rebuild the whole client.
  const userRef = useRef<AuthUser | null>(user);
  userRef.current = user;
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
          //
          // But only when a session actually existed. On a FIRST launch the
          // gate has not routed to /login yet, so the mounted dashboard fires
          // its queries and every one of them 401s with no token at all —
          // which is how someone who has never signed in was greeted with
          // "Your session ended. Sign in again." That copy asserts a past the
          // user does not have.
          //
          // `access-denied` is exempt: it is only reachable with a freshly
          // minted token in hand, so a session provably existed.
          if (reason === "access-denied" || (await hadSession(userRef.current))) {
            useAuthNoticeStore.getState().setNotice(reason);
          }
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
