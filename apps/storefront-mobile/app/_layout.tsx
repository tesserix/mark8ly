import { Stack } from "expo-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "@repo/mobile-shared/auth/provider";
import { StatusBar } from "expo-status-bar";
import { MerchantThemeProvider } from "@/lib/theme/theme-provider";
import { ApiClientProvider } from "@/lib/storefront-api/client-provider";

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 60_000, retry: 2 } },
});

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider tenantId="mp-customer">
        <ApiClientProvider>
          <MerchantThemeProvider>
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
