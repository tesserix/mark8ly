import type { Metadata } from "next";
import { headers } from "next/headers";
import Link from "next/link";
import { resolveStoreSlug } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { StorefrontNav } from "@/components/StorefrontNav";
import { JoinStoreForm } from "@/components/auth/JoinStoreForm";
import { pendingJoinEmail } from "@/app/join/actions";

export const metadata: Metadata = {
  title: "Join this store",
  robots: { index: false, follow: false },
};

function sanitizeNextPath(next: string | undefined): string {
  if (!next || !next.startsWith("/")) return "/account";
  if (next.startsWith("//") || next.includes("\\")) return "/account";
  return next;
}

/**
 * /join — the explicit store join.
 *
 * Reached when a customer signed in correctly but has no account with
 * THIS store. Their Mark8ly login is real and their password was right;
 * what is missing is a membership, and this page is where they create it.
 *
 * The copy has one job beyond consent: it must not let the customer
 * believe this is a second, separate account. It is not — the password is
 * shared platform-wide, and saying otherwise here would be a lie the
 * password-reset flow immediately exposes (see the design doc's "What
 * this decision does NOT give you").
 */
export default async function JoinPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string }>;
}) {
  const { next } = await searchParams;
  const safeNext = sanitizeNextPath(next);

  const h = await headers();
  const host = h.get("host");
  const storeSlug = await resolveStoreSlug(host);
  const store = await fetchStoreBySlug(storeSlug).catch(() => null);
  const storeName = store?.name ?? "this store";

  const email = await pendingJoinEmail(storeSlug);

  const protocol = h.get("x-forwarded-proto") ?? "https";
  const origin = host ? `${protocol}://${host}` : "";

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav storeName={store?.name} />
      <main id="main" className="mx-auto max-w-md px-6 py-16 sm:px-8 sm:py-24">
        <header className="space-y-2">
          <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-[color:var(--storefront-text,var(--ink-900))] opacity-55">
            {store?.name ?? "Store"}
          </p>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            Join {storeName}
          </h1>
        </header>

        {email ? (
          <>
            <div className="mt-4 space-y-3 text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-75">
              <p>
                Your Mark8ly login worked. You just don&apos;t have an account
                with {storeName} yet.
              </p>
              <p>
                Joining creates your account here for{" "}
                <span className="font-medium opacity-100">{email}</span> so this
                store can hold your orders, addresses and rewards. You keep the
                same Mark8ly login and password you already use — nothing new to
                remember, and stores you&apos;ve already joined are unaffected.
              </p>
            </div>
            <JoinStoreForm
              storeName={storeName}
              returnUrl={`${origin}${safeNext}`}
            />
          </>
        ) : (
          <div className="mt-4 space-y-4">
            <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-75">
              This join request has expired. Sign in again to join {storeName}.
            </p>
            <Link
              href="/sign-in"
              className="inline-flex h-11 w-full items-center justify-center rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90"
            >
              Back to sign in
            </Link>
          </div>
        )}
      </main>
    </div>
  );
}
