import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { BrandBar } from "@repo/ui/brand-bar";

import { SignInForm } from "@/components/auth/SignInForm";
import { sanitizeReturnUrl } from "@/lib/auth/sanitize-return-url";
import { RECOVERY_AUTH_REQUEST_SENTINEL } from "@/lib/auth/google-sign-in-admin";
import { ZITADEL_LOGIN_ERROR_COOKIE } from "@/lib/auth/zitadel-oidc";
import { publicConfig } from "@/lib/config";

export const metadata: Metadata = { title: "Sign in" };

interface PageProps {
  searchParams: Promise<{
    returnUrl?: string;
    authRequest?: string;
    error?: string;
    // Set only by app/auth/idp/finish/route.ts when a Google sign-in
    // reached auth-bff's email-OTP gate. The single recognised value is
    // "email_otp" — Zitadel's own TOTP and auth-bff's usermfa gate are
    // still refused on that path (they need a session id/token this page
    // must never receive in a URL), so nothing else may open the code
    // screen from a query string.
    challenge?: string;
    // "1" when the resolved account belongs to more than one store, so
    // the post-code redirect goes to /pick-tenant instead of /dashboard.
    // A display hint only — /pick-tenant re-resolves memberships itself.
    multi?: string;
  }>;
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
  const { returnUrl, authRequest, error, challenge, multi } = await searchParams;
  const safeReturnUrl = sanitizeReturnUrl(returnUrl);

  const isZitadel = publicConfig.authProvider === "zitadel";

  // The recovery sentinel is not an auth request — it is the marker
  // app/auth/idp/finish/route.ts sets to say "the one I had is spent,
  // mint a fresh one but keep my message". Treating it as a real id is
  // what put a dead auth request behind the form and turned the password
  // fallback into raw provider JSON.
  const needsFreshAuthRequest =
    !authRequest || authRequest === RECOVERY_AUTH_REQUEST_SENTINEL;

  if (isZitadel && needsFreshAuthRequest) {
    // Zitadel's login-client model needs an auth_request_id, which
    // only exists after Zitadel's /authorize bounces the browser back
    // here with ?authRequest=. /login/authorize is a Route Handler
    // (not inline here) because minting the PKCE verifier + state
    // cookies requires cookie writes, and Next.js only allows those
    // from a Server Action or Route Handler — never a Server
    // Component's render.
    //
    // `error` is forwarded so the truthful Google-failure message
    // survives the detour; /login/authorize re-validates it against the
    // outcome-code allowlist and parks it in a short-lived cookie,
    // because Zitadel rebuilds this page's URL itself and would drop a
    // query param.
    const params = new URLSearchParams();
    if (safeReturnUrl) params.set("returnUrl", safeReturnUrl);
    if (error) params.set("error", error);
    const suffix = params.toString();
    redirect(`/login/authorize${suffix ? `?${suffix}` : ""}`);
  }

  // Query param first (the direct, same-hop case — e.g. /auth/callback's
  // own state_mismatch), then the cookie that survived a Zitadel detour.
  // Either way the value only ever reaches the browser through
  // messageForAdminGoogleError, which maps an unrecognised code onto a
  // generic message rather than echoing it.
  const googleErrorCode =
    error ?? (await cookies()).get(ZITADEL_LOGIN_ERROR_COOKIE)?.value ?? undefined;

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md">
          <SignInForm
            returnUrl={safeReturnUrl}
            authRequestId={authRequest}
            provider={publicConfig.authProvider}
            googleErrorCode={googleErrorCode || undefined}
            initialChallenge={challenge === "email_otp" ? "email_otp" : undefined}
            initialMultipleTenants={multi === "1"}
          />
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
