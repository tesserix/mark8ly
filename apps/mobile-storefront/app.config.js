// Per-merchant build configuration for the white-label storefront app.
//
// One Expo source tree → one branded native app per merchant. The
// merchant's identity (name, bundle id, palette, logo, splash, default
// store slug) is loaded from `branding/<MERCHANT_SLUG>/config.json` at
// build time and folded into the Expo app config — so EAS produces a
// binary that already looks and feels like the merchant's brand.
//
// Usage:
//
//   # Local dev (against a specific merchant config):
//   MERCHANT_SLUG=example npm --workspace @repo/mobile-storefront run dev
//
//   # Production build for a merchant:
//   cd apps/mobile-storefront
//   MERCHANT_SLUG=acme eas build --profile production --platform all
//
// To onboard a new merchant:
//
//   1. cp -r branding/example branding/<slug>
//   2. Edit branding/<slug>/config.json (name, bundle id, colors, default
//      storefront slug).
//   3. Drop the merchant's icon.png / splash.png / adaptive-icon.png
//      into branding/<slug>/assets/ (or set assetsRef to "shared" to use
//      the bundled defaults).
//   4. MERCHANT_SLUG=<slug> eas build --profile production.
//   5. Submit to the merchant's App Store / Play Store accounts via
//      `eas submit` with a per-merchant submit profile.

const fs = require("fs");
const path = require("path");

const slug = process.env.MERCHANT_SLUG || "example";
const brandingRoot = path.resolve(__dirname, "branding", slug);
const configPath = path.join(brandingRoot, "config.json");

if (!fs.existsSync(configPath)) {
  throw new Error(
    `[mobile-storefront] No branding config at ${configPath}. ` +
      `Set MERCHANT_SLUG=<slug> with a corresponding folder under branding/.`,
  );
}

/**
 * Shape of branding/<slug>/config.json. Every field is required so
 * accidental builds against an incomplete config fail loudly.
 *
 * @typedef {Object} MerchantConfig
 * @property {string} merchantSlug      Unique slug identifying the merchant in our system. Sent on every storefront API call.
 * @property {string} name              App display name (App Store / Play Store / launcher).
 * @property {string} shortName         Optional short label (defaults to `name`).
 * @property {string} bundleIdentifier  iOS bundle id (e.g. com.acme.shop).
 * @property {string} androidPackage    Android package name (typically the same as bundleIdentifier).
 * @property {string} scheme            Custom URL scheme for deep links (e.g. acme-shop).
 * @property {string} associatedDomain  Hostname used for universal/app links (e.g. acme.com).
 * @property {string} defaultStoreSlug  Storefront slug bound at build time (the `{slug}` in api.mark8ly.com/.../storefront/stores/{slug}).
 * @property {Object} colors            Brand palette (overrides theme defaults at runtime).
 * @property {string} colors.primary
 * @property {string} colors.accent
 * @property {string} colors.background
 * @property {string} colors.text
 * @property {string} gipTenantId       Customer GIP/Identity Platform tenant pool id.
 */

/** @type {MerchantConfig} */
const merchant = JSON.parse(fs.readFileSync(configPath, "utf8"));

const REQUIRED = [
  "merchantSlug",
  "name",
  "bundleIdentifier",
  "androidPackage",
  "scheme",
  "associatedDomain",
  "defaultStoreSlug",
  "colors",
  "gipTenantId",
];
for (const key of REQUIRED) {
  if (!merchant[key]) {
    throw new Error(`[mobile-storefront] branding/${slug}/config.json missing required field: ${key}`);
  }
}

// Resolve asset paths. A merchant can either ship their own assets in
// branding/<slug>/assets/ or fall back to the bundled defaults under
// apps/mobile-storefront/assets/.
function resolveAsset(name) {
  const branded = path.join(brandingRoot, "assets", name);
  if (fs.existsSync(branded)) return `./branding/${slug}/assets/${name}`;
  return `./assets/${name}`;
}

const ICON = resolveAsset("icon.png");
const ADAPTIVE_ICON = resolveAsset("adaptive-icon.png");
const SPLASH = resolveAsset("splash.png");

module.exports = {
  expo: {
    name: merchant.name,
    slug: `mark8ly-storefront-${merchant.merchantSlug}`,
    version: "1.0.0",
    orientation: "portrait",
    icon: ICON,
    scheme: merchant.scheme,
    userInterfaceStyle: "light",
    splash: {
      image: SPLASH,
      resizeMode: "contain",
      backgroundColor: merchant.colors.background,
    },
    ios: {
      supportsTablet: true,
      bundleIdentifier: merchant.bundleIdentifier,
      associatedDomains: [`applinks:${merchant.associatedDomain}`],
    },
    android: {
      package: merchant.androidPackage,
      adaptiveIcon: {
        foregroundImage: ADAPTIVE_ICON,
        backgroundColor: merchant.colors.background,
      },
      intentFilters: [
        {
          action: "VIEW",
          autoVerify: true,
          data: [{ scheme: "https", host: merchant.associatedDomain, pathPrefix: "/" }],
          category: ["BROWSABLE", "DEFAULT"],
        },
      ],
    },
    plugins: [
      "expo-router",
      "expo-secure-store",
      "expo-notifications",
      "@react-native-firebase/app",
    ],
    // `extra` is the runtime side of the merchant config — every value
    // here lands in Constants.expoConfig.extra at runtime so the app can
    // bind to the right merchant slug, API base, palette, and identity
    // pool without rebuilding.
    extra: {
      eas: { projectId: merchant.easProjectId || "" },
      apiBaseUrl: "https://api.mark8ly.com",
      merchantSlug: merchant.merchantSlug,
      defaultStoreSlug: merchant.defaultStoreSlug,
      gipTenantId: merchant.gipTenantId,
      shortName: merchant.shortName || merchant.name,
      colors: merchant.colors,
    },
  },
};
