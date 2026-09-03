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
import type { AdminGoogleErrorCode } from "@/lib/auth/google-sign-in-admin";

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

  // /login re-derives a fresh `authRequest` via /login/authorize whenever
  // it renders under Zitadel without one already in the query string —
  // see app/login/page.tsx. Carrying the SAME auth_request_id (still
  // valid: this attempt never consumed it, it only failed to link a
  // Google identity to it) back onto every error redirect avoids that
  // detour and its side effect of losing `error` along the way, so a
  // merchant landing back on /login after a Google failure both sees the
  // truthful message AND can retry with a password using the exact auth
  // request already in flight, rather than an invisible one silently
  // restarted underneath them.
  function errorRedirect(code: AdminGoogleErrorCode): Response {
    const params = new URLSearchParams({ error: code });
    if (authRequestId) params.set("authRequest", authRequestId);
    const dest = forwardedHost
      ? `${proto}://${forwardedHost}/login?${params.toString()}`
      : `/login?${params.toString()}`;
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
          result.code === "invalid_return_url" ||
          result.code === "invalid_intent" ||
          result.code === "zitadel_unavailable"
        ? result.code
        : "internal_error";
    return errorRedirect(code);
  }

  const { data } = result;

  // A step-up (Zitadel's own TOTP, or auth-bff's usermfa/email-OTP gate)
  // is outstanding. auth-bff's usermfa/email-OTP gate DOES mint a pending
  // cookie even here (forwarded above via finishZitadelGoogleSignIn's
  // reuse of mapZitadelOutcome/applySetCookies), but there is no
  // interactive form on this redirect-only path to collect the code the
  // way SignInForm's challenge screens do for the password path — and
  // Zitadel's own TOTP step-up hands back a session id/session token that
  // must not travel in a URL. Rather than strand the merchant on a broken
  // continuation, this treats every step-up as unsupported for the
  // Google path today and sends them back to sign in with a password,
  // where the exact same account can complete its second factor.
  if (data.totpRequired || data.mfaRequired || data.emailOtpRequired) {
    console.error(
      "admin google idp finish: a step-up is outstanding, which this redirect-only route cannot collect",
    );
    return errorRedirect("step_up_unsupported");
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
