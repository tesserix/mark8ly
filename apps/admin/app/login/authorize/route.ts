// Kicks off Zitadel's OIDC authorization-code flow to obtain an
// `auth_request_id` for the login-client model. See Task 4 of
// docs/superpowers/plans/2026-09-03-zitadel-phase3a-admin-frontend.md:
//
//   browser -> /login                    (no authRequest, provider=zitadel)
//           -> 302 to THIS route
//           -> 302 to Zitadel /oauth/v2/authorize?...&redirect_uri=…/auth/callback
//   Zitadel -> 302 back to /login?authRequest=V2_…
//
// Why this is its own Route Handler and not inline in
// app/login/page.tsx: Next.js only allows `cookies()` writes from a
// Server Action or Route Handler — a Server Component's render phase
// is read-only and throws `ReadonlyRequestCookiesError` on `.set()`
// (verified against this repo's Next 16.2.11 install). The PKCE
// verifier and `state` must be stored in httpOnly cookies, so /login
// redirects here instead of trying to write them itself.

import { NextResponse, type NextRequest } from "next/server";
import {
  ZITADEL_STATE_COOKIE,
  ZITADEL_VERIFIER_COOKIE,
  ZITADEL_RETURN_URL_COOKIE,
  ZITADEL_FLOW_COOKIE_MAX_AGE_SECONDS,
  generateState,
  generatePkcePair,
  buildZitadelAuthorizeUrl,
} from "@/lib/auth/zitadel-oidc";
import { sanitizeReturnUrl } from "@/lib/auth/sanitize-return-url";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest): Promise<Response> {
  const issuer = process.env.NEXT_PUBLIC_ZITADEL_ISSUER ?? "";
  const clientId = process.env.NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID ?? "";
  if (!issuer || !clientId) {
    // The provider flag says Zitadel but the issuer/client id aren't
    // configured. Fail loudly instead of redirecting back into
    // /login — that would just bounce here again forever.
    return new NextResponse("zitadel_misconfigured", { status: 500 });
  }

  const forwardedHost = req.headers.get("x-forwarded-host");
  const forwardedProto = req.headers.get("x-forwarded-proto");
  const externalOrigin = forwardedHost
    ? `${forwardedProto ?? "https"}://${forwardedHost}`
    : new URL(req.url).origin;

  // Re-sanitize even though /login already did: this route is itself
  // a public GET endpoint and must not trust a returnUrl handed to it
  // directly.
  const safeReturnUrl = sanitizeReturnUrl(
    req.nextUrl.searchParams.get("returnUrl"),
  );

  const state = generateState();
  const { verifier, challenge } = await generatePkcePair();

  const authorizeUrl = buildZitadelAuthorizeUrl({
    issuer,
    clientId,
    redirectUri: `${externalOrigin}/auth/callback`,
    state,
    codeChallenge: challenge,
  });

  const res = NextResponse.redirect(authorizeUrl);
  const cookieBase = {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    maxAge: ZITADEL_FLOW_COOKIE_MAX_AGE_SECONDS,
  };
  res.cookies.set({ name: ZITADEL_STATE_COOKIE, value: state, ...cookieBase });
  res.cookies.set({
    name: ZITADEL_VERIFIER_COOKIE,
    value: verifier,
    ...cookieBase,
  });
  if (safeReturnUrl) {
    res.cookies.set({
      name: ZITADEL_RETURN_URL_COOKIE,
      value: safeReturnUrl,
      ...cookieBase,
    });
  }
  return res;
}
