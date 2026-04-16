import { ImageResponse } from "next/og";
import { headers } from "next/headers";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { resolveStoreSlug } from "@/lib/slug";

export const runtime = "edge";
export const alt = "Mark8ly store";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

/**
 * Per-tenant OpenGraph image.
 *
 * Renders an editorial Paper · Ink · Moss frame with the store name
 * in serif over the warm paper background. Honest placeholder — no
 * fake hero photos, no fake metrics. Resolves the tenant from the
 * request host on every request.
 */
export default async function OgImage() {
  const h = await headers();
  const host = h.get("host");
  const slug = await resolveStoreSlug(host);
  const store = slug ? await fetchStoreBySlug(slug).catch(() => null) : null;

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
          background: "#F7F6F2",
          color: "#0E0E0C",
          fontFamily: "serif",
        }}
      >
        <div
          style={{
            fontSize: 22,
            letterSpacing: "0.24em",
            textTransform: "uppercase",
            color: "#2D4A2B",
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
          <div style={{ fontSize: 32, color: "#5C5953" }}>{subtitle}</div>
        </div>
        <div
          style={{
            fontSize: 20,
            color: "#7A766E",
            borderTop: "1px solid #DDDACE",
            paddingTop: 24,
          }}
        >
          Paper · Ink · Moss · Powered by Mark8ly
        </div>
      </div>
    ),
    { ...size },
  );
}
