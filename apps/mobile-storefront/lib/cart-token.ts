// The cart's identity, for things the server has to tie to one shopper
// before there is an order: stock holds (#232) and personalisation uploads
// (#963).
//
// On web this is the mk_cart_token cookie, minted by the Next proxy. A
// native app has no proxy and no cookie jar it can rely on, so it mints
// the token itself and sends it explicitly — in the body for upload-url,
// confirm and checkout, in the query for preview and delete. The server
// accepts both forms (cartTokenFrom, cartTokenForCheckout).
//
// A v4 UUID from the OS's CSPRNG rather than Math.random: this token is
// the WHOLE of the authorisation on a buyer's photograph, and the upload
// routes are public.

import * as Crypto from "expo-crypto";

export function newCartToken(): string {
  return Crypto.randomUUID();
}
