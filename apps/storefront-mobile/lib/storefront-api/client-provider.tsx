import { createContext, useContext, useMemo, type ReactNode } from "react";
import Constants from "expo-constants";
import { createStorefrontClient } from "@repo/mobile-shared/api/storefront-client";
import { useAuth } from "@repo/mobile-shared/auth/provider";

export type ApiClient = ReturnType<typeof createStorefrontClient>;

const ApiClientContext = createContext<ApiClient | null>(null);

function getExtraConfig(): { storeSlug: string; apiBaseUrl: string } {
  const extra = Constants.expoConfig?.extra ?? {};
  return {
    storeSlug: (extra.storeSlug as string) ?? "default",
    apiBaseUrl: (extra.apiBaseUrl as string) ?? "https://api.mark8ly.com",
  };
}

export function ApiClientProvider({ children }: { children: ReactNode }) {
  const { getToken } = useAuth();
  const { storeSlug, apiBaseUrl } = getExtraConfig();

  const client = useMemo(
    () =>
      createStorefrontClient({
        baseUrl: apiBaseUrl,
        storeSlug,
        getToken,
      }),
    [apiBaseUrl, storeSlug, getToken],
  );

  return (
    <ApiClientContext.Provider value={client}>
      {children}
    </ApiClientContext.Provider>
  );
}

export function useApiClient(): ApiClient {
  const ctx = useContext(ApiClientContext);
  if (!ctx) {
    throw new Error("useApiClient must be used within ApiClientProvider");
  }
  return ctx;
}

export function useStoreSlug(): string {
  return getExtraConfig().storeSlug;
}
