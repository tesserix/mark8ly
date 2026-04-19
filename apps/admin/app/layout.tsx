import type { ReactNode } from "react";
import type { Metadata } from "next";
import { Source_Sans_3, Source_Serif_4 } from "next/font/google";
import { SkipLink } from "@repo/ui/skip-link";
import { QueryProvider } from "@/lib/query/QueryProvider";

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

export const metadata: Metadata = {
  metadataBase: new URL(
    process.env.NEXT_PUBLIC_ADMIN_URL ?? "https://admin.mark8ly.com",
  ),
  title: {
    default: "Mark8ly Admin",
    template: "%s · Mark8ly Admin",
  },
  description:
    "The calm control room for your Mark8ly store — products, orders, customers, and storefront in one place.",
  // Admin is auth-gated. No SEO surface. No robots, no sitemap, no OG.
  robots: {
    index: false,
    follow: false,
    nocache: true,
    googleBot: { index: false, follow: false },
  },
  icons: {
    icon: "/favicon.ico",
    shortcut: "/favicon.ico",
  },
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
      <body>
        <SkipLink />
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
