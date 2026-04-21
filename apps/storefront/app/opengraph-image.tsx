import { ImageResponse } from "next/og";
import { headers } from "next/headers";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { resolveStoreSlug } from "@/lib/slug";
import {
  defaultStorefrontTheme,
  normalizeStorefrontTheme,
} from "@repo/ui/storefront-theme";

export const runtime = "edge";
export const alt = "Mark8ly store";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

/**
 * Per-tenant OpenGraph image.
 *
 * Resolves the merchant's storefront_theme and paints the card in
 * that palette — background / text / accent all flow from the same
 * settings that drive the live store, so the social preview matches
 * what shoppers see on the landing page. Falls back to the default
 * theme for tenants that haven't configured one yet.
 */

const HEX_RE = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;
function safeHex(value: string | undefined, fallback: string): string {
  return value && HEX_RE.test(value) ? value : fallback;
}

function rgbaFromHex(hex: string, alpha: number): string {
  const normalized = hex.length === 4
    ? `#${hex[1]!}${hex[1]!}${hex[2]!}${hex[2]!}${hex[3]!}${hex[3]!}`
    : hex;
  const r = parseInt(normalized.slice(1, 3), 16);
  const g = parseInt(normalized.slice(3, 5), 16);
  const b = parseInt(normalized.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

export default async function OgImage() {
  const h = await headers();
  const host = h.get("host");
  const slug = await resolveStoreSlug(host);
  const store = slug ? await fetchStoreBySlug(slug).catch(() => null) : null;

  const theme = store?.storefront_theme
    ? normalizeStorefrontTheme(store.storefront_theme)
    : defaultStorefrontTheme;

  const background = safeHex(theme.colors.background, "#F7F6F2");
  const text = safeHex(theme.colors.text, "#0E0E0C");
  const accent = safeHex(theme.colors.accent, "#2D4A2B");

  const name = store?.name ?? "Mark8ly";
  const subtitle = store
    ? `${store.slug}.mark8ly.com`
    : "Open your store on Mark8ly";

  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "80px",
          background,
          color: text,
          fontFamily: "serif",
        }}
      >
        <div
          style={{
            fontSize: 22,
            letterSpacing: "0.24em",
            textTransform: "uppercase",
            color: accent,
          }}
        >
          mark8ly
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <div
            style={{
              fontSize: 96,
              lineHeight: 1.05,
              letterSpacing: "-0.025em",
              fontWeight: 500,
            }}
          >
            {name}
          </div>
          <div style={{ fontSize: 32, color: rgbaFromHex(text, 0.6) }}>
            {subtitle}
          </div>
        </div>
        <div
          style={{
            fontSize: 20,
            color: rgbaFromHex(text, 0.45),
            borderTop: `1px solid ${rgbaFromHex(text, 0.15)}`,
            paddingTop: 24,
          }}
        >
          Powered by Mark8ly
        </div>
      </div>
    ),
    { ...size },
  );
}
