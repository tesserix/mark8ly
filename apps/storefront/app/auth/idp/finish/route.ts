// Target of the successUrl/failureUrl this app hands auth-bff's
// POST /auth/customer/idp/start (see apps/storefront/app/auth/idp/actions.ts).
// Zitadel redirects the browser here directly — there is no
// mark8ly.com trampoline in this path: Zitadel takes the return URL per
// request, so the browser comes straight back to this store's own host.
//
// On success Zitadel appends `id` and `token` (plus `user`, only when the
// identity was already linked). On failure it appends `id`, `error`, and
// `error_description` instead — see idpintent.go's StartIDPIntent doc.
//
// `user` is NEVER READ here, for anything. It rides in a URL the browser
// followed and is attacker-controlled; the authoritative identity comes
// only from finishCustomerIDPIntent(id, token), which resolves it against
// auth-bff/Zitadel directly. See auth-bff's customerIDPFinishRequest.User
// doc for the full rationale — this route mirrors it.
//
// This route mints the SAME `mp_customer_session` cookie as
// apps/storefront/app/sign-in/actions.ts's customerSignIn, via the
// shared completeCustomerSignIn tail in @/lib/auth/customer-session —
// never a second cookie-minting path. No cookie is set on any failure
// branch below.
//
// Guarded on NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" below: this route
// only exists to finish a Zitadel IDP intent, so under GIP it must not
// process anything — an intent minted against a Zitadel-flagged store
// must not be replayable against a GIP-flagged store's finish route.

import { NextResponse } from "next/server";
import { resolveStoreSlug } from "@/lib/slug";
import { sanitizeHost } from "@/lib/host";
import { completeCustomerSignIn, resolveStore } from "@/lib/auth/customer-session";
import {
  AuthBffCustomerError,
  finishCustomerIDPIntent,
} from "@/lib/auth/auth-bff-customer";
import {
  isGoogleSignInDest,
  type GoogleSignInDest,
} from "@/lib/auth/google-sign-in-dest";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const DEFAULT_DEST: GoogleSignInDest = "/account";

export async function GET(req: Request): Promise<Response> {
  // This route only applies under the Zitadel provider — under GIP there
  // is no flow that could ever legitimately land a browser here, and an
  // intent minted against a Zitadel-flagged store must not be replayable
  // against a GIP-flagged store's finish route.
  if (process.env.NEXT_PUBLIC_AUTH_PROVIDER !== "zitadel") {
    return new NextResponse(null, { status: 404 });
  }

  const url = new URL(req.url);
  const intentId = url.searchParams.get("id");
  const intentToken = url.searchParams.get("token");
  const zitadelError = url.searchParams.get("error");
  const destParam = url.searchParams.get("dest");
  const dest: GoogleSignInDest = isGoogleSignInDest(destParam)
    ? destParam
    : DEFAULT_DEST;

  // Zitadel's own failure redirect: no token, `error`/`error_description`
  // instead. There is nothing to exchange with auth-bff in this case —
  // error_description is Zitadel's own free-text detail and must never be
  // rendered to the shopper (it could contain anything), so it is only
  // logged.
  if (zitadelError) {
    console.error(
      "google idp finish: zitadel reported a failure before an intent was ever created",
      zitadelError,
      url.searchParams.get("error_description"),
    );
    return errorRedirect(req, "google_sign_in_unavailable");
  }

  if (!intentId || !intentToken) {
    console.error("google idp finish: missing id/token on the callback");
    return errorRedirect(req, "invalid_request");
  }

  const forwardedHost = req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const storeSlug = await resolveStoreSlug(forwardedHost);
  if (!storeSlug) {
    console.error("google idp finish: could not resolve a store for this host");
    return errorRedirect(req, "store_not_found");
  }

  const store = await resolveStore(storeSlug);
  if (!store) {
    console.error("google idp finish: store lookup failed", storeSlug);
    return errorRedirect(req, "store_not_found");
  }

  const cookieHost = sanitizeHost(forwardedHost);
  if (!cookieHost) {
    console.error("google idp finish: request host failed cookie-domain validation");
    return errorRedirect(req, "invalid_host");
  }

  // The ONLY source of identity for this route. `id`/`token` above are the
  // sole inputs — `user` (see the file header) is never read or forwarded.
  let outcome;
  try {
    outcome = await finishCustomerIDPIntent({ intentId, intentToken });
  } catch (err) {
    if (err instanceof AuthBffCustomerError) {
      console.error("google idp finish: auth-bff call failed", err.code);
    } else {
      console.error("google idp finish: unexpected error calling auth-bff", err);
    }
    return errorRedirect(req, "zitadel_unavailable");
  }

  if (outcome.kind !== "complete") {
    console.error("google idp finish: rejected", outcome.code);
    return errorRedirect(req, outcome.code);
  }

  const result = await completeCustomerSignIn(store, cookieHost, storeSlug, {
    uid: outcome.uid,
    email: outcome.email,
  });
  if (!result.ok) {
    console.error("google idp finish: completeCustomerSignIn failed", result.code);
    return errorRedirect(req, result.code ?? "google_sign_in_unavailable");
  }

  const protocol = req.headers.get("x-forwarded-proto") ?? "https";
  return NextResponse.redirect(`${protocol}://${forwardedHost}${dest}`, {
    status: 303,
  });
}

function errorRedirect(req: Request, code: string): Response {
  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const isLocal =
    forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.");
  const proto =
    req.headers.get("x-forwarded-proto") ?? (isLocal ? "http" : "https");
  const dest = forwardedHost
    ? `${proto}://${forwardedHost}/sign-in?error=${encodeURIComponent(code)}`
    : `/sign-in?error=${encodeURIComponent(code)}`;
  return NextResponse.redirect(dest, { status: 303 });
}
