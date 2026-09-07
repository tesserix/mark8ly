import type { Metadata } from "next";
import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { CustomerSignInForm } from "@/components/auth/CustomerSignInForm";
import { StorefrontNav } from "@/components/StorefrontNav";
import { googleIdpErrorMessage } from "@/lib/auth/google-idp-error-messages";

export const metadata: Metadata = {
  title: "Sign in",
  robots: { index: false, follow: false },
};

/**
 * Sanitize the `next` query param into a safe same-origin redirect target.
 * Only site-relative paths ("/account/orders/…") are allowed — anything that
 * could point off-site (protocol-relative "//evil", backslash tricks, absolute
 * URLs) falls back to the account home. This is the open-redirect guard for
 * the "sign in and come back" flow used by invoice/receipt email links.
 */
function sanitizeNextPath(next: string | undefined): string {
  if (!next || !next.startsWith("/")) return "/account";
  if (next.startsWith("//") || next.includes("\\")) return "/account";
  return next;
}

/**
 * /sign-in — customer sign-in page.
 *
 * Server component resolves the store context and hands it to the
 * client-side form. The form posts the credentials to a server action,
 * which mints the session via auth-bff.
 */
export default async function SignInPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; error?: string }>;
}) {
  const { next, error } = await searchParams;
  const safeNext = sanitizeNextPath(next);
  // Populated only when the browser was just bounced back here by
  // apps/storefront/app/auth/idp/finish/route.ts's ?error=<code> redirect
  // (the Zitadel Google flow's failure path). `error` is always one of a
  // small fixed set of codes that route itself put in the URL — never raw
  // text from auth-bff/Zitadel — so this lookup never risks rendering an
  // internal error string; see googleIdpErrorMessage's file header.
  const googleError = googleIdpErrorMessage(error);
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);

  const store = await fetchStoreBySlug(storeSlug).catch(() => null);

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
            Sign in
          </h1>
          <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
            Sign in to track orders, manage addresses, and earn loyalty rewards.
          </p>
        </header>
        <CustomerSignInForm
          storeSlug={storeSlug}
          returnUrl={`${origin}${safeNext}`}
          initialError={googleError}
        />
      </main>
    </div>
  );
}
