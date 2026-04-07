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
    <div className="min-h-screen flex flex-col bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-50">
      <Header />
      <main className="flex-1 flex items-center justify-center px-6 py-24">
        <div className="max-w-xl text-center">
          <h1 className="text-4xl sm:text-5xl font-semibold tracking-tight mb-4">
            {title}
          </h1>
          <p className="text-lg text-zinc-600 dark:text-zinc-400 mb-8">
            {description}
          </p>
          <p className="text-sm text-zinc-500 mb-8">Coming soon.</p>
          <Button asChild variant="outline">
            <Link href="/">Back to home</Link>
          </Button>
        </div>
      </main>
      <Footer />
    </div>
  );
}
