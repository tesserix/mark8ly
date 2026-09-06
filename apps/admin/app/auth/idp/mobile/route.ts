// The bridge between Zitadel's browser redirect and the mobile admin app
// (#686 item 1).
//
// # Why this page exists at all
//
// The native app opens Google sign-in in an `ASWebAuthenticationSession`
// (expo-web-browser) that closes the moment the browser is sent to the
// app's own `mark8ly-admin://` scheme. The obvious design — make that
// custom scheme the Zitadel return URL — is impossible twice over:
//
//   - auth-bff's `ValidateReturnURL` (returnurl.go) requires `https`, an
//     allowlisted host, no port and no userinfo. A custom scheme parses
//     with an empty/`mark8ly-admin` scheme and is rejected. That check is
//     not defence in depth — Zitadel does not validate `successUrl` at
//     all, so it is the entire control against handing a completed admin
//     sign-in to somebody else's origin, and it is not being relaxed.
//   - A universal link would satisfy `https`, but
//     `https://admin.mark8ly.com/.well-known/apple-app-site-association`
//     currently 404s, so iOS would open this URL in Safari and never hand
//     it to the app.
//
// So Zitadel returns to THIS https page, on the allowlisted admin host,
// and this page issues a 302 to the custom scheme carrying the query
// through unchanged. The authentication session intercepts the scheme and
// closes with the URL, which is what the app parses.
//
// # Why the query is passed through verbatim, errors included
//
// Zitadel appends `id` + `token` on success and `id` + `error` +
// `error_description` on failure (see idpintent.go's StartIDPIntent doc).
// Dropping the failure params would leave a cancelled or failed Google
// sign-in with nothing to redirect to: the browser would sit on this page
// and the app would wait for a callback that never comes, which reads to
// the merchant as a frozen sign-in rather than a cancelled one. Forwarding
// them lets the app close the session and show real copy.
//
// # What this page must never do
//
// It never reads `id`/`token`, never calls auth-bff, and never mints a
// session. It is a redirect and nothing else — the app exchanges the
// intent through marketplace-api, which holds the internal-auth secret a
// browser does not. It also deliberately does NOT touch
// `app/auth/idp/finish/route.ts`: that is the web flow, and it stays
// exactly as it is.

import { NextResponse } from "next/server";
import { publicConfig } from "@/lib/config";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * The app's registered scheme (apps/mobile-admin/app.config.js `scheme`)
 * and the path its authentication session is opened with. A CONSTANT, not
 * config and never anything from the request: the whole safety property of
 * this route is that its redirect target cannot be influenced by the URL
 * it was reached with.
 */
const APP_CALLBACK = "mark8ly-admin://auth/idp";

/** Params forwarded to the app. Anything else Zitadel or an attacker
 * appends is dropped rather than reflected, so this route cannot be used
 * to smuggle arbitrary values into the app's deep-link handler. */
const FORWARDED = ["id", "token", "error", "error_description"] as const;

export async function GET(req: Request): Promise<Response> {
  // Same gate as the web finish route: under GIP there is no flow that
  // could land a browser here.
  if (publicConfig.authProvider !== "zitadel") {
    return new NextResponse(null, { status: 404 });
  }

  const incoming = new URL(req.url).searchParams;
  const params = new URLSearchParams();
  for (const key of FORWARDED) {
    const value = incoming.get(key);
    if (value !== null) params.set(key, value);
  }

  // No recognised param at all means this was not a Zitadel redirect.
  // Still hand the app SOMETHING it can close on — a session left open on
  // a blank page is the one outcome the user cannot recover from — but say
  // plainly that the callback was malformed rather than inventing a
  // success.
  if ([...params.keys()].length === 0) {
    params.set("error", "invalid_callback");
  }

  const query = params.toString();
  // 302, not 303: the app's authentication session matches on the scheme
  // and never issues the follow-up request, so the method-rewriting
  // semantics of 303 buy nothing, and a plain redirect is what
  // ASWebAuthenticationSession is documented to intercept.
  return NextResponse.redirect(`${APP_CALLBACK}?${query}`, { status: 302 });
}
