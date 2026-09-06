// Target of the successUrl/failureUrl this app hands auth-bff's
// POST /auth/zitadel/idp/start (see app/auth/idp/actions.ts).
// Zitadel redirects the browser here directly.
//
// On success Zitadel appends `id` and `token` (plus `user`, only when the
// identity was already linked). On failure it appends `id`, `error`, and
// `error_description` instead — see idpintent.go's StartIDPIntent doc.
//
// `user` is NEVER READ here, for anything. It rides in a URL the browser
// followed and is attacker-controlled; the authoritative identity comes
// only from finishZitadelGoogleSignIn(id, token, ...), which resolves it
// against auth-bff/Zitadel directly. See auth-bff's
// idpFinishRequest.User doc for the full rationale — this route mirrors
// it, exactly like apps/storefront/app/auth/idp/finish/route.ts does for
// the customer path.
//
// This route mints the SAME `m8_session` cookie every other Zitadel
// sign-in on this app mints — finishZitadelGoogleSignIn
// (app/login/actions.ts) reuses mapZitadelOutcome, the identical
// session-minting path signInWithZitadel/confirmZitadelTotp use. No
// cookie is set on any failure branch below.
//
// Which tenant a merchant lands in is resolved by finishZitadelGoogleSignIn
// from the VERIFIED Google identity's email (via the same
// resolveWorkspaceTenant the password path uses), never from the request
// host. An earlier version of this route derived the tenant from
// `{slug}-admin.mark8ly.com` instead — which meant Google sign-in was
// 100% broken in production, because the admin /login page (and so this
// route) is only ever reached on the CANONICAL `admin.mark8ly.com` host.
// This route does not need to know which host it is on to resolve a
// tenant; it only needs the host to build its own absolute redirect URLs.

import { NextResponse } from "next/server";
import { publicConfig } from "@/lib/config";
import { isTrustedCallbackUrl, isTrustedZitadelHostedUrl } from "@/lib/auth/zitadel-oidc";
import { finishZitadelGoogleSignIn } from "@/app/login/actions";
import {
  RECOVERY_AUTH_REQUEST_SENTINEL,
  type AdminGoogleErrorCode,
} from "@/lib/auth/google-sign-in-admin";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const DEFAULT_DESTINATION = "/dashboard";
const MULTI_TENANT_DESTINATION = "/pick-tenant";

export async function GET(req: Request): Promise<Response> {
  // This route only applies under the Zitadel provider — under GIP there
  // is no flow that could ever land a browser here.
  if (publicConfig.authProvider !== "zitadel") {
    return new NextResponse(null, { status: 404 });
  }

  const url = new URL(req.url);
  const intentId = url.searchParams.get("id");
  const intentToken = url.searchParams.get("token");
  const zitadelError = url.searchParams.get("error");
  const authRequestId = url.searchParams.get("auth_request_id");
  // `user` is intentionally never read — see the file header.

  const headerBag = new Headers(req.headers);
  const forwardedHost = headerBag.get("x-forwarded-host") ?? headerBag.get("host") ?? "";
  const proto = headerBag.get("x-forwarded-proto") ??
    (forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.") ? "http" : "https");

  // Every error redirect lands on canonical /login carrying the RECOVERY
  // sentinel rather than this attempt's own `auth_request_id`.
  //
  // An earlier version handed the original id back, on the theory that a
  // failed Google link leaves the auth request untouched and reusable. It
  // does not: once the flow has reached auth-bff's idp/complete, Zitadel
  // has SPENT that auth request. /login then rendered its form around a
  // dead id and the password fallback this route's own copy recommends
  // came back as raw provider JSON —
  // `{"error": "No valid authentication request found"}` — so the one
  // recovery path offered to the merchant could not work. Reported from
  // production.
  //
  // The sentinel keeps the redirect past middleware's canonical-/login
  // 404 gate (which needs either a valid slug returnUrl — this flow has
  // no slug — or a non-empty authRequest) and tells /login to mint a
  // FRESH auth request via /login/authorize while carrying `error`
  // across that hop, so the merchant still sees why Google failed AND
  // gets a form that actually submits. See
  // RECOVERY_AUTH_REQUEST_SENTINEL, app/login/page.tsx and
  // app/login/authorize/route.ts.
  function errorRedirect(code: AdminGoogleErrorCode): Response {
    const params = new URLSearchParams({ error: code });
    params.set("authRequest", RECOVERY_AUTH_REQUEST_SENTINEL);
    const dest = forwardedHost
      ? `${proto}://${forwardedHost}/login?${params.toString()}`
      : `/login?${params.toString()}`;
    return NextResponse.redirect(dest, { status: 303 });
  }

  // The email-OTP continuation: NOT an error, so it keeps this attempt's
  // real `auth_request_id` and does not detour through /login/authorize
  // (that detour would lose `challenge` — Zitadel rebuilds the /login URL
  // itself and only appends its own authRequest). The id is spent by now,
  // but nothing on the code screen uses it: confirmEmailOTPLogin resumes
  // purely from the pending cookie auth-bff minted plus the typed code.
  // It rides along only so /login renders instead of 404ing, and so the
  // form has the same prop shape it has on the password path.
  //
  // `multi` is a UI hint, not a credential — it decides /pick-tenant vs
  // /dashboard after the code verifies, exactly like `mfaMultipleTenants`
  // does on the password path. Forging it buys nothing: /pick-tenant
  // re-lists the caller's real memberships server-side and auto-skips to
  // /dashboard for a single-tenant account.
  //
  // No session id, session token, intent id or intent token is ever put
  // in this URL — see the TOTP refusal below for why that rule exists.
  function emailOtpRedirect(multipleTenants: boolean): Response {
    const params = new URLSearchParams({ authRequest: authRequestId ?? "" });
    params.set("challenge", "email_otp");
    if (multipleTenants) params.set("multi", "1");
    const dest = `${proto}://${forwardedHost}/login?${params.toString()}`;
    return NextResponse.redirect(dest, { status: 303 });
  }

  // Zitadel's own failure redirect: no token, `error`/`error_description`
  // instead. There is nothing to exchange with auth-bff in this case —
  // error_description is Zitadel's own free-text detail and must never be
  // rendered to the merchant (it could contain anything), so it is only
  // logged.
  if (zitadelError) {
    console.error(
      "admin google idp finish: zitadel reported a failure before an intent was ever created",
      zitadelError,
      url.searchParams.get("error_description"),
    );
    return errorRedirect("google_sign_in_unavailable");
  }

  if (!intentId || !intentToken || !authRequestId) {
    console.error("admin google idp finish: missing id/token/auth_request_id on the callback");
    return errorRedirect("invalid_request");
  }

  if (!forwardedHost) {
    // Needed to build this route's own absolute redirect URLs (and to
    // validate a returned callback_url's origin below) — not for tenant
    // resolution, which never touches the host on this path.
    console.error("admin google idp finish: no request host available");
    return errorRedirect("invalid_request");
  }

  // The ONLY source of identity for this route. `id`/`token` above are
  // the sole inputs — `user` (see the file header) is never read or
  // forwarded. Tenant resolution happens INSIDE finishZitadelGoogleSignIn,
  // off the verified Google identity's email, never off this request's
  // host.
  const result = await finishZitadelGoogleSignIn({
    authRequestId,
    intentId,
    intentToken,
  });

  if (!result.ok) {
    console.error("admin google idp finish: sign-in failed", result.code);
    const code: AdminGoogleErrorCode =
      result.code === "no_admin_account" || result.code === "tenant_not_found"
        ? "no_admin_account"
        : result.code === "unexpected_idp" ||
          result.code === "email_not_verified" ||
          result.code === "email_ambiguous" ||
          result.code === "invalid_intent" ||
          result.code === "zitadel_unavailable"
        ? result.code
        : "internal_error";
    return errorRedirect(code);
  }

  const { data } = result;

  // Zitadel's OWN TOTP step-up, and auth-bff's usermfa gate, both stay
  // unsupported on this path. The reason has not changed: Zitadel's TOTP
  // step-up hands back a session id and session token that
  // confirmZitadelTotp must carry forward, and those must never travel in
  // a URL — which is the only channel a redirect-only route has. The
  // merchant is sent back to sign in with a password, where the exact
  // same account can complete its second factor.
  if (data.totpRequired || data.mfaRequired) {
    console.error(
      "admin google idp finish: a totp/mfa step-up is outstanding, which this redirect-only route cannot collect",
    );
    return errorRedirect("step_up_unsupported");
  }

  // Email OTP is the one step-up this path CAN continue, and it is the
  // common case rather than an edge: a merchant signing in with Google
  // from a browser auth-bff has not fingerprinted before always trips the
  // deviceguard/emailotp gate, so refusing it refused web Google sign-in
  // outright (#686).
  //
  // Nothing needs to be smuggled through the URL to resume it. auth-bff
  // minted a PENDING cookie on this very response —
  // finishZitadelGoogleSignIn's mapZitadelOutcome ran applySetCookies,
  // which writes through next/headers' cookies() and so rides onto the
  // 303 below, the same mechanism app/auth/handoff/route.ts uses to land
  // a session cookie on a redirect. The code screen then calls
  // confirmEmailOTPLogin, which needs exactly that pending cookie plus
  // the typed code and nothing else.
  if (data.emailOtpRequired) {
    return emailOtpRedirect(data.multipleTenants);
  }

  if (data.handoffUrl) {
    if (isTrustedZitadelHostedUrl(data.handoffUrl, publicConfig.zitadelIssuer)) {
      return NextResponse.redirect(data.handoffUrl, { status: 303 });
    }
    console.error("admin google idp finish: rejected an untrusted Zitadel handoffUrl");
    return errorRedirect("zitadel_unavailable");
  }

  if (data.callbackUrl) {
    // A completed Zitadel login carries its own /auth/callback URL —
    // navigate there instead of straight to the dashboard so that route
    // can verify state, clear the flow cookies, and only then land the
    // merchant on their destination. Validate first: the only legitimate
    // target is this app's own /auth/callback, on this app's own origin.
    const appOrigin = `${proto}://${forwardedHost}`;
    if (isTrustedCallbackUrl(data.callbackUrl, appOrigin)) {
      return NextResponse.redirect(data.callbackUrl, { status: 303 });
    }
    console.error("admin google idp finish: rejected an untrusted Zitadel callbackUrl", data.callbackUrl);
  }

  const destination = data.multipleTenants ? MULTI_TENANT_DESTINATION : DEFAULT_DESTINATION;
  return NextResponse.redirect(`${proto}://${forwardedHost}${destination}`, {
    status: 303,
  });
}
