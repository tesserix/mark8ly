import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";
import {
  Source_Sans_3,
  Source_Serif_4,
  Inter,
  Manrope,
  Newsreader,
  Space_Grotesk,
} from "next/font/google";
import { cookies, headers } from "next/headers";
import { SkipLink } from "@repo/ui/skip-link";

import { CartProvider } from "@/components/CartProvider";
import { CustomerAuthProvider } from "@/components/CustomerAuthProvider";
import { PromotionBar } from "@/components/PromotionBar";
import { Toaster } from "@/components/Toaster";
import { slugFromHost } from "@/lib/slug";
import { buildLoginUrl, buildLogoutUrl, hasSessionCookie, decodeSession } from "@/lib/auth";
import { fetchBranding } from "@/lib/api/marketplace-api";

import "./globals.css";

/* ============================================================
   Fonts — load every themeable choice.
   ------------------------------------------------------------
   The merchant picks one heading font + one body font in the
   admin StorefrontThemeForm. Since we can't know their choice
   at build time, we preload all supported fonts via next/font
   and let the body's `font-family: var(--storefront-body-font)`
   resolve to whichever font-family stack the tenant picked.
   Each font registers its own CSS variable so the fontStacks
   helper in @repo/ui/storefront-theme can reference them.
   ============================================================ */

const sourceSans = Source_Sans_3({
  subsets: ["latin"],
  variable: "--font-source-sans",
  display: "swap",
});

const sourceSerif = Source_Serif_4({
  subsets: ["latin"],
  variable: "--font-source-serif",
  display: "swap",
});

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

const manrope = Manrope({
  subsets: ["latin"],
  variable: "--font-manrope",
  display: "swap",
});

const newsreader = Newsreader({
  subsets: ["latin"],
  variable: "--font-newsreader",
  display: "swap",
});

const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-space-grotesk",
  display: "swap",
});

/* ============================================================
   Default metadata — per-tenant overrides are emitted from
   individual pages via generateMetadata. This root metadata is
   the fallback and the shared-shape baseline.
   ============================================================ */

export const metadata: Metadata = {
  title: {
    default: "Storefront",
    template: "%s",
  },
  description: "A store on Mark8ly.",
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-snippet": -1,
      "max-image-preview": "large",
    },
  },
  applicationName: "Mark8ly Storefront",
};

export const viewport: Viewport = {
  themeColor: "#F7F6F2",
  colorScheme: "light",
  width: "device-width",
  initialScale: 1,
};

interface RootLayoutProps {
  children: ReactNode;
}

export default async function RootLayout({ children }: RootLayoutProps) {
  const fontVars = [
    sourceSans.variable,
    sourceSerif.variable,
    inter.variable,
    manrope.variable,
    newsreader.variable,
    spaceGrotesk.variable,
  ].join(" ");

  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";

  // --- Customer auth state ---
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  const authenticated = hasSessionCookie(cookieHeader);

  const protocol = h.get("x-forwarded-proto") ?? "https";
  const origin = host ? `${protocol}://${host}` : "";
  const loginUrl = buildLoginUrl(`${origin}/`);
  const logoutUrl = buildLogoutUrl(`${origin}/`);

  let displayName: string | null = null;
  let email: string | null = null;

  if (authenticated) {
    const sessionCookie = cookieStore.get("mp_customer_session");
    const session = sessionCookie
      ? decodeSession(sessionCookie.value)
      : null;
    if (session) {
      email = session.email;
      // Derive a display name from the email for now. Once the
      // customer profile has first_name/last_name populated, we can
      // fetch those from marketplace-api.
      const local = session.email.split("@")[0] ?? "";
      const first = local.split(/[+.\-_]/)[0] ?? "";
      displayName =
        first && /^[a-z]+$/i.test(first)
          ? first.charAt(0).toUpperCase() + first.slice(1).toLowerCase()
          : null;
    }
  }

  const authState = {
    isAuthenticated: authenticated,
    displayName,
    email,
    loginUrl,
    logoutUrl,
  };

  const brandingData = await fetchBranding(storeSlug);

  return (
    <html lang="en" className={fontVars}>
      <body>
        <SkipLink />
        <PromotionBar
          promotion={brandingData?.active_promotion}
          announcement={
            brandingData?.branding
              ? {
                  text: brandingData.branding.announcement_text,
                  link: brandingData.branding.announcement_link,
                  bg: brandingData.branding.announcement_bg,
                  active: brandingData.branding.announcement_active,
                }
              : undefined
          }
        />
        <CustomerAuthProvider value={authState}>
          <CartProvider storeSlug={storeSlug}>{children}</CartProvider>
        </CustomerAuthProvider>
        <Toaster />
      </body>
    </html>
  );
}
