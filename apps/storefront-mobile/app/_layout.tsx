import { useEffect, useRef } from "react";
import { Stack } from "expo-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, useAuth } from "@repo/mobile-shared/auth/provider";
import { StatusBar } from "expo-status-bar";
import { MerchantThemeProvider } from "@/lib/theme/theme-provider";
import { ApiClientProvider, useApiClient } from "@/lib/storefront-api/client-provider";
import { setupPushNotifications } from "@/lib/push/setup";

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 60_000, retry: 2 } },
});

function PushNotificationManager() {
  const { user } = useAuth();
  const client = useApiClient();
  const cleanupRef = useRef<{ dispose: () => void } | null>(null);

  useEffect(() => {
    if (!user) {
      if (cleanupRef.current) {
        cleanupRef.current.dispose();
        cleanupRef.current = null;
      }
      return;
    }

    let cancelled = false;

    setupPushNotifications(client).then((cleanup) => {
      if (cancelled) {
        cleanup.dispose();
      } else {
        cleanupRef.current = cleanup;
      }
    });

    return () => {
      cancelled = true;
      if (cleanupRef.current) {
        cleanupRef.current.dispose();
        cleanupRef.current = null;
      }
    };
  }, [user, client]);

  return null;
}

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider tenantId="mp-customer">
        <ApiClientProvider>
          <MerchantThemeProvider>
            <PushNotificationManager />
            <StatusBar style="dark" />
            <Stack screenOptions={{ headerShown: false }}>
              <Stack.Screen name="(tabs)" />
              <Stack.Screen name="(auth)" options={{ presentation: "modal" }} />
              <Stack.Screen name="checkout" />
            </Stack>
          </MerchantThemeProvider>
        </ApiClientProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}
