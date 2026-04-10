import * as fs from "fs";
import * as path from "path";

const ROOT = path.resolve(__dirname, "..");

async function main(): Promise<void> {
  const storeSlug = process.env["STORE_SLUG"];
  const storeName = process.env["STORE_NAME"];
  const apiBaseUrl = process.env["API_BASE_URL"] ?? "https://api.mark8ly.com";

  if (!storeSlug) {
    console.log("STORE_SLUG not set — using default branding");
    return;
  }

  // Fetch branding from API
  const brandingUrl = `${apiBaseUrl}/api/v1/mobile/storefront/stores/${storeSlug}/branding`;
  let branding: Record<string, unknown> | null = null;

  try {
    const res = await fetch(brandingUrl);
    if (!res.ok) {
      console.warn(
        `generate-brand: branding fetch failed (${res.status}) — continuing with defaults`
      );
    } else {
      branding = (await res.json()) as Record<string, unknown>;
      const overridesPath = path.join(ROOT, "theme-overrides.json");
      fs.writeFileSync(overridesPath, JSON.stringify(branding, null, 2));
      console.log(`generate-brand: wrote theme-overrides.json for store "${storeSlug}"`);
    }
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    console.warn(
      `generate-brand: branding fetch error — ${message} — continuing with defaults`
    );
  }

  // Update app.json
  const appJsonPath = path.join(ROOT, "app.json");
  const appJson = JSON.parse(fs.readFileSync(appJsonPath, "utf-8")) as {
    expo: {
      name?: string;
      scheme?: string;
      extra?: Record<string, unknown>;
    };
  };

  if (storeName) {
    appJson.expo.name = storeName;
  }
  appJson.expo.scheme = `${storeSlug}-store`;
  appJson.expo.extra = {
    ...(appJson.expo.extra ?? {}),
    storeSlug,
  };

  fs.writeFileSync(appJsonPath, JSON.stringify(appJson, null, 2));
  console.log(
    `generate-brand: updated app.json — name="${appJson.expo.name}", scheme="${appJson.expo.scheme}"`
  );
}

main().catch((err: unknown) => {
  console.error("generate-brand: fatal error", err);
  process.exit(1);
});
