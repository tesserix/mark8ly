"use client";

import Link from "next/link";
import { Button } from "@tesserix/web";

import { Header } from "./Header";
import { Footer } from "./Footer";

interface StubPageProps {
  title: string;
  description: string;
}

/**
 * One-screen "coming soon" placeholder for the marketing stub pages.
 * Footer links resolve to a real page rather than 404, with the same
 * Header/Footer chrome as the landing page so navigation feels coherent.
 */
export function StubPage({ title, description }: StubPageProps) {
  return (
    <div className="min-h-screen flex flex-col text-foreground">
      <Header />
      <main className="flex-1 flex items-center justify-center px-6 py-24">
        <div className="max-w-xl rounded-[2rem] border border-warm-200/90 bg-white/90 px-8 py-10 text-center shadow-[0_24px_80px_rgba(43,38,34,0.1)] backdrop-blur-sm">
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-4">
            {title}
          </h1>
          <p className="text-lg text-foreground-secondary mb-8">
            {description}
          </p>
          <p className="text-sm text-foreground-tertiary mb-8">Coming soon.</p>
          <Button asChild variant="outline">
            <Link href="/">Back to home</Link>
          </Button>
        </div>
      </main>
      <Footer />
    </div>
  );
}
