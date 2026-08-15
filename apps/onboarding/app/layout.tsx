import type { ReactNode } from "react";
import type { Metadata, Viewport } from "next";
import { Source_Sans_3, Source_Serif_4 } from "next/font/google";
import { SkipLink } from "@repo/ui/skip-link";
import { Analytics } from "./analytics";

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

const SITE_URL = "https://mark8ly.com";
const SITE_NAME = "Mark8ly";
const SITE_TAGLINE = "Quiet commerce for people who make things";
const SITE_DESCRIPTION =
  "A modern editorial commerce platform for independent merchants. Launch your store in an afternoon, keep every sale, and look considered from day one.";

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

/* ============================================================
   JSON-LD structured data. Two graphs injected as a single
   script tag so Google + LLM crawlers can pick up the
   organization + website in one pass.
   ============================================================ */

const jsonLd = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "Organization",
      "@id": `${SITE_URL}#organization`,
      name: SITE_NAME,
      legalName: "Tesserix",
      url: SITE_URL,
      // A square mark, not the 1200×630 social card. Google's
      // Organization logo guidance wants a logo image it can crop to
      // a square; handing it the OG banner gets the logo dropped from
      // knowledge-panel and AI-answer attribution.
      logo: `${SITE_URL}/icon-192.png`,
      description: SITE_DESCRIPTION,
      email: "hello@mark8ly.com",
      sameAs: [
        "https://twitter.com/mark8ly",
        "https://instagram.com/mark8ly",
        "https://linkedin.com/company/mark8ly",
      ],
      // Tesserix is an Australian entity. No specific locality
      // is emitted here until the legal registration address is
      // confirmed — fabricating a city to fill the schema shape is
      // worse than a narrower-but-accurate address.
      address: {
        "@type": "PostalAddress",
        addressCountry: "AU",
      },
    },
    {
      "@type": "WebSite",
      "@id": `${SITE_URL}#website`,
      url: SITE_URL,
      name: SITE_NAME,
      description: SITE_TAGLINE,
      publisher: { "@id": `${SITE_URL}#organization` },
      inLanguage: "en",
    },
  ],
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
          dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
        />
      </head>
      <body>
        <SkipLink />
        {children}
        <Analytics />
      </body>
    </html>
  );
}
