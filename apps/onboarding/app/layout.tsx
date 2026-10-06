import type { ReactNode } from "react";
import type { Metadata, Viewport } from "next";
import { Source_Sans_3, Source_Serif_4 } from "next/font/google";
import { SkipLink } from "@repo/ui/skip-link";
import { ChunkRecovery } from "@repo/ui/chunk-recovery";
import { Analytics } from "./analytics";
import { AcquisitionCapture } from "@/components/AcquisitionCapture";

import "./globals.css";

const sourceSans = Source_Sans_3({
  subsets: ["latin"],
  variable: "--font-editorial-sans",
  display: "swap",
});

const sourceSerif = Source_Serif_4({
  subsets: ["latin"],
  variable: "--font-editorial-serif",
  display: "swap",
});

/* ============================================================
   Site metadata — single source of truth for SEO, social
   preview, and LLM discovery. Per-page metadata extends or
   overrides these defaults.
   ============================================================ */

import {
  SITE_DESCRIPTION,
  SITE_JSON_LD,
  SITE_NAME,
  SITE_TAGLINE,
  SITE_URL,
} from "@/lib/seo/site-json-ld";

/**
 * How long the edge may serve a marketing page before asking us again.
 *
 * Without this, every statically prerendered route under this layout ships
 * `cache-control: s-maxage=31536000` — Next's default for a page with no
 * revalidation period — and Cloudflare honours the year. On 2026-10-06 the
 * homepage was still the previous build two hours after a rollout, and the
 * sitemap fix the day before needed a manual purge to reach crawlers at all.
 *
 * 300 seconds makes the pages incremental-static: still prerendered, still
 * served from cache, regenerated in the background at most every five
 * minutes, so a deploy reaches visitors within that window with no purge.
 * Routes that declare `dynamic = "force-dynamic"` (the whole /onboarding
 * funnel) are unaffected; they were never cached.
 *
 * If the edge still holds pages longer than this, the override is a
 * Cloudflare cache rule, not the app — see docs and the memory note.
 */
export const revalidate = 300;

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: `${SITE_NAME} — ${SITE_TAGLINE.toLowerCase()}`,
    template: `%s · ${SITE_NAME}`,
  },
  description: SITE_DESCRIPTION,
  applicationName: SITE_NAME,
  authors: [{ name: "Tesserix" }],
  creator: "Tesserix",
  publisher: "Tesserix",
  category: "ecommerce",
  keywords: [
    "ecommerce platform",
    "indie commerce",
    "storefront builder",
    "online store",
    "small business",
    "editorial commerce",
    "Shopify alternative",
    "no transaction fees",
  ],
  alternates: {
    canonical: "/",
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-snippet": -1,
      "max-image-preview": "large",
      "max-video-preview": -1,
    },
  },
  openGraph: {
    type: "website",
    locale: "en_US",
    url: SITE_URL,
    siteName: SITE_NAME,
    title: `${SITE_NAME} — ${SITE_TAGLINE.toLowerCase()}`,
    description: SITE_DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: `${SITE_NAME} — ${SITE_TAGLINE.toLowerCase()}`,
    description: SITE_DESCRIPTION,
    creator: "@mark8ly",
  },
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "any" },
      { url: "/icon-192.png", type: "image/png", sizes: "192x192" },
    ],
    shortcut: "/favicon.ico",
    apple: "/apple-touch-icon.png",
  },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#F7F6F2" },
    { media: "(prefers-color-scheme: dark)", color: "#F7F6F2" },
  ],
  colorScheme: "light",
  width: "device-width",
  initialScale: 1,
};


interface RootLayoutProps {
  children: ReactNode;
}

export default function RootLayout({ children }: RootLayoutProps) {
  return (
    <html
      lang="en"
      className={`${sourceSans.variable} ${sourceSerif.variable}`}
    >
      <head>
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: SITE_JSON_LD }}
        />
      </head>
      <body>
        <SkipLink />
        {children}
        <ChunkRecovery />
        <Analytics />
        <AcquisitionCapture />
      </body>
    </html>
  );
}
