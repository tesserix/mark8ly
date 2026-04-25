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

import {
  normalizeStorefrontTheme,
  themeCssVariables,
} from "@repo/ui/storefront-theme";

import { CartProvider } from "@/components/CartProvider";
import { CustomerAuthProvider } from "@/components/CustomerAuthProvider";
import { Footer } from "@/components/Footer";
import { PromotionBar } from "@/components/PromotionBar";
// Otto support chat is temporarily disabled — not shipping to end
// users yet. The widget component is still in the tree (imported
// by `components/OttoSupportChat.tsx`) so re-enabling is a
// single-line change below. See docs/otto-disabled.md for the
// full re-enable checklist.
// import { OttoSupportChat } from "@/components/OttoSupportChat";
import { Toaster } from "@/components/Toaster";
import { resolveStoreSlug } from "@/lib/slug";
import { buildLoginUrl, buildLogoutUrl } from "@/lib/auth";
import { decodeSessionForScope } from "@/lib/session";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { fetchStoreBySlug } from "@/lib/api/platform-api";

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

// Only Source Sans + Source Serif preload on every request — these
// are the house defaults and are already in the render path for most
// tenants. Merchant-specialty fonts (Inter / Manrope / Newsreader /
// Space Grotesk) register their CSS vars but ship with preload: false
// so they don't block FCP for tenants who never reference them. When
// the tenant theme points at one, next/font still serves it with
// font-display: swap once that tenant's CSS actually needs the stack.

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
  preload: false,
});

const manrope = Manrope({
  subsets: ["latin"],
  variable: "--font-manrope",
  display: "swap",
  preload: false,
});

const newsreader = Newsreader({
  subsets: ["latin"],
  variable: "--font-newsreader",
  display: "swap",
  preload: false,
});

const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-space-grotesk",
  display: "swap",
  preload: false,
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
  // Default favicon for any page not wrapped by generateMetadata
  // (error boundary, 404, sign-out). Per-tenant pages override via
  // makeTenantMetadata → branding.favicon_url → fallback to the
  // same /favicon.ico shipped here.
  icons: {
    icon: "/favicon.ico",
    shortcut: "/favicon.ico",
    apple: "/icon-192.png",
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
    await resolveStoreSlug(host);

  // --- Customer auth state ---
  const cookieStore = await cookies();
  const protocol = h.get("x-forwarded-proto") ?? "https";
  const origin = host ? `${protocol}://${host}` : "";
  const loginUrl = buildLoginUrl(`${origin}/`);
  const logoutUrl = buildLogoutUrl(`${origin}/`);

  let displayName: string | null = null;
  let email: string | null = null;

  const sessionCookie = cookieStore.get("mp_customer_session");
  const session = sessionCookie
    ? decodeSessionForScope(sessionCookie.value, { storeSlug })
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

  const authState = {
    isAuthenticated: Boolean(email),
    displayName,
    email,
    loginUrl,
    logoutUrl,
  };

  const [brandingData, store] = await Promise.all([
    fetchBranding(storeSlug),
    fetchStoreBySlug(storeSlug).catch(() => null),
  ]);

  // Compute the merchant's theme CSS vars at the layout root so every
  // page inherits the merchant's font + radius + color choices.
  // Without this the storefront rendered with the browser's default
  // body font regardless of what they picked in /settings/themes.
  const normalizedTheme = store?.storefront_theme
    ? normalizeStorefrontTheme(store.storefront_theme)
    : null;
  const themeStyle = normalizedTheme
    ? (themeCssVariables(normalizedTheme) as Record<string, string>)
    : undefined;

  const seo = brandingData?.branding;
  const storeName = brandingData?.store?.name ?? store?.name ?? null;

  return (
    <html lang="en" className={fontVars}>
      <head>
        {seo?.seo_google_verification ? (
          <meta name="google-site-verification" content={seo.seo_google_verification} />
        ) : null}
        {seo?.seo_bing_verification ? (
          <meta name="msvalidate.01" content={seo.seo_bing_verification} />
        ) : null}
        {seo?.seo_og_image_url ? (
          <meta property="og:image" content={seo.seo_og_image_url} />
        ) : null}
        {seo?.seo_twitter_handle ? (
          <meta name="twitter:site" content={seo.seo_twitter_handle} />
        ) : null}
        {seo?.seo_ai_policy === "deny" ? (
          <meta name="robots" content="noai, noimageai" />
        ) : seo?.seo_ai_policy === "training-only-denied" ? (
          <meta name="robots" content="noai-training" />
        ) : null}
        <JsonLd raw={seo?.seo_json_ld} />
      </head>
      <body
        style={themeStyle}
        data-motion={normalizedTheme?.motion ?? "subtle"}
        data-density={normalizedTheme?.density ?? "balanced"}
        data-radius={normalizedTheme?.radius ?? "soft"}
      >
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
          <Footer
            branding={brandingData?.branding ?? null}
            storeName={brandingData?.store?.name ?? null}
          />
          {/* Otto support chat disabled until product launch —
              see docs/otto-disabled.md to re-enable. */}
          {/* <OttoSupportChat storeName={brandingData?.store?.name ?? undefined} /> */}
        </CustomerAuthProvider>
        <Toaster />
        <MerchantCSS css={brandingData?.branding?.custom_css} />
      </body>
    </html>
  );
}

/**
 * Sanitize and inject merchant-authored CSS. Strips @import, url(),
 * expression(), javascript:, behavior:, -moz-binding, and HTML tags
 * to prevent XSS / data exfiltration via stylesheets.
 */
function MerchantCSS({ css }: { css?: string | null }) {
  if (!css) return null;
  const sanitized = sanitizeCss(css);
  if (!sanitized) return null;
  return <style dangerouslySetInnerHTML={{ __html: sanitized }} />;
}

/**
 * Emit merchant-authored JSON-LD. Parses to validate — invalid JSON is
 * silently dropped rather than producing broken markup.
 */
function JsonLd({ raw }: { raw?: string | null }) {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    return (
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(parsed) }}
      />
    );
  } catch {
    return null;
  }
}

function sanitizeCss(raw: string): string {
  let css = raw;
  css = css.replace(/<[^>]*>/g, "");
  css = css.replace(/@import\b[^;]*/gi, "/* blocked @import */");
  css = css.replace(/url\s*\([^)]*\)/gi, "/* blocked url() */");
  css = css.replace(/expression\s*\([^)]*\)/gi, "/* blocked expression() */");
  css = css.replace(/javascript\s*:/gi, "/* blocked */");
  css = css.replace(/behavior\s*:/gi, "/* blocked */");
  css = css.replace(/-moz-binding\s*:/gi, "/* blocked */");
  return css.trim();
}
