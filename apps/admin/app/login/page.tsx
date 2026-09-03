import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { BrandBar } from "@repo/ui/brand-bar";

import { SignInForm } from "@/components/auth/SignInForm";
import { sanitizeReturnUrl } from "@/lib/auth/sanitize-return-url";
import { publicConfig } from "@/lib/config";

export const metadata: Metadata = { title: "Sign in" };

interface PageProps {
  searchParams: Promise<{ returnUrl?: string; authRequest?: string }>;
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
  const { returnUrl, authRequest } = await searchParams;
  const safeReturnUrl = sanitizeReturnUrl(returnUrl);

  const isZitadel = publicConfig.authProvider === "zitadel";

  if (isZitadel && !authRequest) {
    // Zitadel's login-client model needs an auth_request_id, which
    // only exists after Zitadel's /authorize bounces the browser back
    // here with ?authRequest=. /login/authorize is a Route Handler
    // (not inline here) because minting the PKCE verifier + state
    // cookies requires cookie writes, and Next.js only allows those
    // from a Server Action or Route Handler — never a Server
    // Component's render.
    const params = new URLSearchParams();
    if (safeReturnUrl) params.set("returnUrl", safeReturnUrl);
    const suffix = params.toString();
    redirect(`/login/authorize${suffix ? `?${suffix}` : ""}`);
  }

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md">
          <SignInForm returnUrl={safeReturnUrl} authRequestId={authRequest} />
          {/* Operator disclosure on the sign-in screen: this is where a
              merchant decides to trust the platform, and the entity on their
              settlement statement should not be a surprise later. */}
          <p className="mt-10 text-center text-xs leading-relaxed text-muted-foreground">
            Tesserix Pty Ltd
            <br />
            ACN 694 070 865 · ABN 59 694 070 865
          </p>
        </div>
      </main>
    </>
  );
}
