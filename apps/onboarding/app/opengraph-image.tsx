import { ImageResponse } from "next/og";

import { HERO } from "@/lib/copy/home";

/**
 * OpenGraph image — Next 16 file convention. Generated at build
 * into a 1200x630 PNG served as /opengraph-image. Automatically
 * wired into <meta property="og:image"> and <meta name="twitter:image">
 * by the metadata API.
 *
 * The composition is a single editorial moment: serif wordmark,
 * the homepage H1, moss accent rule, trial terms. The headline reads
 * from the same constant as the H1 so a link preview says what the
 * page says. No icons, no decoration — paper/ink/moss only.
 */

export const alt = `Mark8ly — ${HERO.title}`;
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default async function OpenGraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          width: "100%",
          height: "100%",
          background: "#F7F6F2",
          padding: "80px 96px",
          fontFamily: "Georgia, serif",
        }}
      >
        {/* Top eyebrow */}
        <div
          style={{
            display: "flex",
            fontSize: 20,
            letterSpacing: "0.16em",
            textTransform: "uppercase",
            color: "#7A766E",
            fontFamily: "system-ui, sans-serif",
          }}
        >
          Mark8ly
        </div>

        {/* Main typographic moment */}
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "24px",
          }}
        >
          <div
            style={{
              fontSize: 76,
              fontWeight: 500,
              color: "#0E0E0C",
              letterSpacing: "-0.03em",
              lineHeight: 1.02,
              display: "flex",
              flexDirection: "column",
            }}
          >
            {HERO.title}
          </div>
        </div>

        {/* Bottom: moss rule + tagline */}
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "20px",
          }}
        >
          <div
            style={{
              width: "80px",
              height: "3px",
              background: "#2D4A2B",
            }}
          />
          <div
            style={{
              fontSize: 28,
              color: "#45433E",
              fontFamily: "system-ui, sans-serif",
              display: "flex",
            }}
          >
            {HERO.terms}
          </div>
        </div>
      </div>
    ),
    { ...size },
  );
}
