// Cart identity for the buyer-upload routes (#965).
//
// marketplace-api REFUSES to mint a cart token for an upload, unlike the
// cart-holds endpoint which mints one on a first write. That asymmetry is
// deliberate upstream: minting there gives a first cart write an
// identity it legitimately lacks, whereas minting on an upload would
// hand the caller a fresh identity that owns nothing, and the only thing
// it could then do is create an orphan.
//
// So the storefront mints it, which is where the cookie lives anyway —
// `proxyJson` does not forward upstream Set-Cookie, and an upstream
// cookie would carry the API's domain rather than this origin.

import { cookies } from "next/headers";

export const CART_TOKEN_COOKIE = "mk_cart_token";

// Matches personalisationupload.TTL. If they drift, the cookie outlives
// or predeceases the uploads it identifies; the server's value is the one
// that decides, this only bounds how long the browser keeps the identity.
const UPLOAD_TTL_SECONDS = 72 * 60 * 60;

/**
 * Returns the cart token for this browser, minting and persisting one
 * when there is none.
 *
 * httpOnly for the same reason cart-holds uses it: the token is the ONLY
 * authorisation on a buyer's uploads, so script access buys the
 * storefront nothing and costs XSS exposure.
 */
export async function ensureCartToken(): Promise<string> {
  const store = await cookies();
  const existing = store.get(CART_TOKEN_COOKIE)?.value;
  if (existing) return existing;

  const token = crypto.randomUUID();
  store.set(CART_TOKEN_COOKIE, token, {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge: UPLOAD_TTL_SECONDS,
  });
  return token;
}

/** Reads the cart token without minting. */
export async function readCartToken(): Promise<string | undefined> {
  const store = await cookies();
  return store.get(CART_TOKEN_COOKIE)?.value;
}
