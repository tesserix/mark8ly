import Link from "next/link";
import { ArrowRight } from "lucide-react";

/**
 * Admin login page — not a real login form. Admin doesn't issue its own
 * sessions; the merchant already authenticated on the marketing site
 * during onboarding. When the middleware detects a missing session
 * cookie it bounces here (or directly to the marketing /login page,
 * depending on the configured redirect target).
 *
 * This page is the "you were redirected — go back to the marketing
 * site" landing that exists so the URL doesn't 404 in the rare case a
 * user hits /admin/login directly.
 */
const MARKETING_URL =
  process.env.NEXT_PUBLIC_MARKETING_URL ?? "http://localhost:4201";

export default function LoginPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-6 py-16 text-foreground">
      <div className="max-w-md rounded-[2rem] border border-warm-200/90 bg-white/90 p-10 text-center shadow-[0_20px_60px_rgba(43,38,34,0.08)]">
        <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
          Mark8ly admin
        </p>
        <h1 className="mt-3 font-serif text-3xl font-medium tracking-tight">
          Sign in to continue
        </h1>
        <p className="mt-4 text-sm leading-6 text-foreground-secondary">
          Admin sessions are minted during onboarding. Head back to the
          marketing site to sign in or start a new store.
        </p>
        <Link
          href={MARKETING_URL}
          className="mt-8 inline-flex items-center gap-2 rounded-full bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-md"
        >
          Go to mark8ly.com
          <ArrowRight className="h-4 w-4" />
        </Link>
      </div>
    </main>
  );
}
