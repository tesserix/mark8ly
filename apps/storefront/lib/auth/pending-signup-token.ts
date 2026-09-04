// Signs and verifies the {uid, email} pair registerCustomer hands back to
// the client, so verifyCustomerEmail (app/create-account/actions.ts) never
// has to trust a client-supplied email on its own.
//
// THE ATTACK THIS CLOSES: POST /auth/customer/verify-email's 2xx body
// carries only {"data":{"verified":true}} — no email. Before this module
// existed, verifyCustomerEmail took `email` straight from the client and
// handed it to completeCustomerSignIn, which signs it into the session
// cookie AND upserts a marketplace-api customer profile keyed on
// (store_id, email). An attacker could register their OWN address
// (receiving a genuine uid + code in their own inbox — Zitadel checks
// only {uid, code}, never email, when verifying), then replay the verify
// call with `email` swapped to a victim's address. The session would mint
// under the victim's identity — their order history, addresses, loyalty
// balance — with zero interaction from the victim. This is a storefront
// account-takeover primitive, not a cosmetic mismatch.
//
// The fix: registerCustomer signs the {uid, email} pair it just created
// and hands the client an opaque token alongside them. verifyCustomerEmail
// refuses to proceed — no Zitadel call, no completeCustomerSignIn, no
// cookie — unless the token verifies against the EXACT {uid, email} it
// receives. An attacker who swaps `email` after register also has to swap
// `token` (or leave it matching the original email), and either way this
// module (via a constant-time comparison) rejects it: the token exists to
// certify a pair that already left this server intact.
//
// Reuses SESSION_ENCRYPT_KEY (see lib/session.ts) rather than introducing
// and provisioning a second secret. The two signed values cannot be
// confused for one another even under the same key: a session cookie's
// HMAC covers a base64-encoded JSON payload with five fields (uid, email,
// store_slug, store_id, tenant_id, plus iat/exp), while this token's HMAC
// covers a purpose-tagged "uid|email" string with no store/tenant/expiry
// fields at all — there is no input on which the two schemes agree, so a
// value valid for one is never even shaped like a candidate for the
// other.
//
// Deliberately UNEXPIRING: the code itself (verified by Zitadel, see
// customerVerificationCodeTTL in services/auth-bff) is what bounds how
// long a shopper has to finish signing up, and this token's only job is
// to keep the email attached to that code from being swapped in transit
// — it does not need its own clock.

import { createHmac, timingSafeEqual } from "node:crypto";

const DEV_SESSION_KEY = "dev-session-key-min-32-bytes!!!";

// Distinguishes this token's payload shape from any other value ever
// signed with the same key — see the file header's "no cross-protocol
// reuse" note.
const TOKEN_PURPOSE = "customer-signup-pending-verification:v1";

function sessionKey(): string {
  const key = process.env.SESSION_ENCRYPT_KEY;
  if (key) return key;
  if (process.env.NODE_ENV === "production") {
    throw new Error("SESSION_ENCRYPT_KEY is required in production");
  }
  return DEV_SESSION_KEY;
}

function payloadFor(uid: string, email: string): string {
  return `${TOKEN_PURPOSE}|${uid}|${email}`;
}

/**
 * signPendingSignup signs the {uid, email} pair registerCustomer just
 * created with Zitadel, for verifyCustomerEmail to check later. Called
 * exactly once, immediately after a successful register — never re-signed
 * for a client-supplied email, or this would just move the trust problem
 * one step over.
 */
export function signPendingSignup(uid: string, email: string): string {
  return createHmac("sha256", sessionKey()).update(payloadFor(uid, email)).digest("hex");
}

/**
 * verifyPendingSignup reports whether `token` was produced by
 * signPendingSignup for EXACTLY this {uid, email} pair. Constant-time
 * comparison, matching lib/session.ts's decodeSession discipline — this
 * guards a value an attacker gets to submit repeatedly, so a
 * length/early-exit timing leak is worth closing even though the
 * practical exploitation bar here is already high.
 */
export function verifyPendingSignup(uid: string, email: string, token: string): boolean {
  if (!token) return false;
  const expected = signPendingSignup(uid, email);
  if (token.length !== expected.length) return false;
  return timingSafeEqual(Buffer.from(token), Buffer.from(expected));
}
