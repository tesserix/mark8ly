import type { ReactNode } from "react";
import type { Metadata } from "next";
import { Source_Sans_3, Source_Serif_4 } from "next/font/google";
import { SkipLink } from "@repo/ui/skip-link";

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
  title: {
    default: "mark8ly — quiet commerce for people who make things",
    template: "%s · mark8ly",
  },
  description:
    "A modern editorial commerce platform for indie merchants and considered brands. Launch your store in an afternoon.",
  icons: {
    icon: "/favicon.ico",
    shortcut: "/favicon.ico",
    apple: "/apple-touch-icon.png",
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
        {children}
      </body>
    </html>
  );
}
