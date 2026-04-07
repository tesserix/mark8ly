import type { ReactNode } from "react";
import type { Metadata } from "next";
import { Source_Sans_3, Source_Serif_4 } from "next/font/google";

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
  title: "mark8ly",
  description: "Get your store live on mark8ly.",
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
    <html lang="en" className={`${sourceSans.variable} ${sourceSerif.variable}`}>
      <body>{children}</body>
    </html>
  );
}
