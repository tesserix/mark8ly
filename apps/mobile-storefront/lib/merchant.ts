import Constants from "expo-constants";

/**
 * Build-time merchant identity, baked into the bundle by app.config.js.
 *
 * Every storefront API call uses `defaultStoreSlug` to scope the
 * request to one merchant's storefront — that's the whole point of the
 * white-label model. The app never sees other merchants' data.
 */
export interface MerchantConfig {
  /** Unique merchant slug used internally by Mark8ly. */
  merchantSlug: string;
  /** Storefront slug used in the public-facing API path. */
  defaultStoreSlug: string;
  /** Mark8ly storefront API base URL — always prod for white-label apps. */
  apiBaseUrl: string;
  /** Customer GIP/Identity Platform tenant pool id. */
  gipTenantId: string;
  /** Short brand label for UI strings ("Welcome to Acme"). */
  shortName: string;
  /** Build-time palette. Runtime branding from /storefront/branding can override. */
  colors: {
    primary: string;
    accent: string;
    background: string;
    text: string;
  };
}

interface ExpoExtra extends Partial<MerchantConfig> {}

let cached: MerchantConfig | null = null;

function readMerchantFromExpoConfig(): MerchantConfig {
  const extra = (Constants.expoConfig?.extra ?? {}) as ExpoExtra;

  if (!extra.merchantSlug || !extra.defaultStoreSlug) {
    throw new Error(
      "[mobile-storefront] Merchant config missing — was app.config.js loaded? " +
        "Set MERCHANT_SLUG=<slug> when starting Expo or building.",
    );
  }

  return {
    merchantSlug: extra.merchantSlug,
    defaultStoreSlug: extra.defaultStoreSlug,
    apiBaseUrl: extra.apiBaseUrl ?? "https://api.mark8ly.com",
    gipTenantId: extra.gipTenantId ?? "",
    shortName: extra.shortName ?? "Shop",
    colors: extra.colors ?? {
      primary: "#0E0E0C",
      accent: "#2D4A2B",
      background: "#F7F6F2",
      text: "#0E0E0C",
    },
  };
}

/**
 * Returns the build-time merchant config. Stable across the app
 * lifetime — the only way to change it is to ship a new build.
 */
export function getMerchant(): MerchantConfig {
  if (!cached) cached = readMerchantFromExpoConfig();
  return cached;
}
