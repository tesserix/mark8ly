import { useEffect, useRef } from "react";
import { Slot, useRouter, useSegments } from "expo-router";
import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from "@tanstack/react-query";
import { AuthProvider, useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import * as SplashScreen from "expo-splash-screen";

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, retry: 2 },
  },
});

function AuthGate() {
  const { user, loading } = useAuth();
  const segments = useSegments();
  const router = useRouter();
  const queryClient = useQueryClient();
  const hydrate = useTenantStore((s) => s.hydrate);
  const clearTenant = useTenantStore((s) => s.clear);
  const previousUid = useRef<string | null>(null);

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  useEffect(() => {
    if (loading) return;

    const inAuthGroup = segments[0] === "login";

    // Whenever the signed-in identity changes (sign out, or sign in as a
    // different account), throw away every cached query and the tenant
    // selection so user A's data can't leak into user B's session.
    const currentUid = user?.uid ?? null;
    if (previousUid.current !== null && previousUid.current !== currentUid) {
      queryClient.clear();
      clearTenant();
    }
    previousUid.current = currentUid;

    if (!user && !inAuthGroup) {
      router.replace("/login");
    } else if (user && inAuthGroup) {
      router.replace("/");
    }

    SplashScreen.hideAsync();
  }, [user, loading, segments, router, clearTenant, queryClient]);

  return <Slot />;
}

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <AuthGate />
      </AuthProvider>
    </QueryClientProvider>
  );
}
