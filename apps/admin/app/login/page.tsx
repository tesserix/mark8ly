import type { Metadata } from "next";
import { BrandBar } from "@repo/ui/brand-bar";

import { SignInForm } from "@/components/auth/SignInForm";

export const metadata: Metadata = { title: "Sign in" };

interface PageProps {
  searchParams: Promise<{ returnUrl?: string }>;
}

/**
 * Admin /login — returning-user funnel.
 *
 * Hosted at the canonical `admin.mark8ly.com` host. Two paths to a
 * session:
 *   1. Email + password — Identity Toolkit signInWithPassword + the
 *      `signIn` server action which looks up workspace_tenant by GIP
 *      UID and calls auth-bff /auth/auto-login.
 *   2. Continue with Google — gsi/client popup, exchanged via Identity
 *      Toolkit signInWithIdp, then the same server action.
 *
 * The `returnUrl` query param is set by middleware on per-tenant
 * subdomains that bounce here for authentication. After sign-in the
 * form redirects back to that URL — the session cookie is scoped to
 * .mark8ly.com so it carries across the bounce.
 */
export default async function LoginPage({ searchParams }: PageProps) {
  const { returnUrl } = await searchParams;
  const safeReturnUrl = sanitizeReturnUrl(returnUrl);

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md">
          <SignInForm returnUrl={safeReturnUrl} />
        </div>
      </main>
    </>
  );
}

/**
 * sanitizeReturnUrl accepts only URLs under mark8ly.com (and its
 * subdomains). Anything else — an external host, a javascript: URL,
 * or a protocol-relative `//evil.com` — gets dropped to prevent
 * open-redirect abuse.
 */
function sanitizeReturnUrl(raw: string | undefined): string | undefined {
  if (!raw) return undefined;
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:" && u.protocol !== "http:") return undefined;
    if (
      u.hostname === "mark8ly.com" ||
      u.hostname.endsWith(".mark8ly.com") ||
      u.hostname === "localhost"
    ) {
      return u.toString();
    }
  } catch {
    // fall through
  }
  return undefined;
}
