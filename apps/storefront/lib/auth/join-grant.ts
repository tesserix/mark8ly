// The short-lived, HMAC-signed grant that carries a verified customer
// identity from "you signed in correctly, but you have no account with
// THIS store" to the join screen.
//
// WHY THIS EXISTS AT ALL. A Mark8ly login is one platform identity;
// access to a store is a separate membership the customer joins
// deliberately (docs/superpowers/specs/2026-09-05-customer-store-membership-design.md).
// So completeCustomerSignIn must NOT hand the browser an
// `mp_customer_session` cookie for a store the customer has not joined —
// that cookie is exactly the thing the whole gate is about. But joining
// still has to prove who is joining, and the only party that verified the
// credential is this server, one request ago.
//
// The grant closes that gap: it is a session-shaped claim set, signed the
// same way and scoped to the same host, but issued under its own purpose
// tag, with a 10-minute lifetime, under its own cookie name — and it
// authorises exactly one thing (creating the membership), never a
// customer session. The real session is minted only after the join
// succeeds, by the same completeCustomerSignIn tail every other path
// uses.
//
// It is deliberately a COOKIE rather than a value returned to the client:
// the Google/OIDC finish routes reach the join screen through a browser
// redirect, and putting a bearer-ish credential in a URL puts it in
// access logs, referrers, and history. HttpOnly keeps it out of page
// scripts too.
//
// Reuses SESSION_ENCRYPT_KEY (see lib/session.ts and
// lib/auth/pending-signup-token.ts) rather than provisioning a second
// secret. Cross-protocol confusion is prevented by the purpose tag inside
// the signed payload: a session cookie's payload has no `purpose` field,
// and verifyJoinGrant rejects any payload whose purpose is not exactly
// this module's, so a grant can never be replayed as a session and a
// session can never be replayed as a grant.

import { createHmac, timingSafeEqual } from "node:crypto";

const DEV_SESSION_KEY = "dev-session-key-min-32-bytes!!!";

const GRANT_PURPOSE = "customer-store-join-grant:v1";

/** The customer has ten minutes to accept the join before signing in
 *  again. Short because the grant stands in for a freshly verified
 *  credential, and a credential check that old should be redone. */
export const JOIN_GRANT_TTL_SECONDS = 10 * 60;

export const JOIN_GRANT_COOKIE = "mp_join_grant";

export interface JoinGrantClaims {
  uid: string;
  email: string;
  store_slug: string;
  store_id: string;
  tenant_id: string;
}

interface SignedJoinGrant extends JoinGrantClaims {
  purpose: string;
  exp: number;
}

function grantKey(): string {
  const key = process.env.SESSION_ENCRYPT_KEY;
  if (key) return key;
  if (process.env.NODE_ENV === "production") {
    throw new Error("SESSION_ENCRYPT_KEY is required in production");
  }
  return DEV_SESSION_KEY;
}

function sign(payload: string): string {
  return createHmac("sha256", grantKey()).update(payload).digest("hex");
}

/** signJoinGrant encodes and signs a grant for the identity THIS server
 *  just verified. Never called with values a client supplied. */
export function signJoinGrant(
  claims: JoinGrantClaims,
  ttlSeconds: number = JOIN_GRANT_TTL_SECONDS,
): string {
  const body: SignedJoinGrant = {
    ...claims,
    purpose: GRANT_PURPOSE,
    exp: Math.floor(Date.now() / 1000) + ttlSeconds,
  };
  const payload = Buffer.from(JSON.stringify(body)).toString("base64");
  return `${payload}.${sign(payload)}`;
}

/**
 * verifyJoinGrant returns the claims a grant carries, or null if the
 * value is malformed, tampered with, expired, not a join grant, or not
 * for the store named by `expectedStoreSlug`.
 *
 * The store check is not decorative: without it a grant minted at store2
 * — where the customer legitimately failed the membership gate — would
 * authorise a join at store3.
 */
export function verifyJoinGrant(
  value: string | undefined | null,
  expectedStoreSlug: string,
): JoinGrantClaims | null {
  if (typeof value !== "string" || !value) return null;

  const dotIndex = value.lastIndexOf(".");
  if (dotIndex < 0) return null;
  const payload = value.slice(0, dotIndex);
  const sig = value.slice(dotIndex + 1);

  const expected = sign(payload);
  if (sig.length !== expected.length) return null;
  try {
    if (!timingSafeEqual(Buffer.from(sig), Buffer.from(expected))) return null;
  } catch {
    return null;
  }

  let parsed: Partial<SignedJoinGrant>;
  try {
    parsed = JSON.parse(
      Buffer.from(payload, "base64").toString(),
    ) as Partial<SignedJoinGrant>;
  } catch {
    return null;
  }

  if (parsed.purpose !== GRANT_PURPOSE) return null;
  if (typeof parsed.exp !== "number") return null;
  if (Math.floor(Date.now() / 1000) > parsed.exp) return null;
  if (!parsed.uid || !parsed.email) return null;
  if (!parsed.store_slug || parsed.store_slug !== expectedStoreSlug) return null;
  if (!parsed.store_id || !parsed.tenant_id) return null;

  return {
    uid: parsed.uid,
    email: parsed.email,
    store_slug: parsed.store_slug,
    store_id: parsed.store_id,
    tenant_id: parsed.tenant_id,
  };
}
