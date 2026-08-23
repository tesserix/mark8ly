/**
 * HMAC-signed customer session cookie.
 *
 * Cookie format: `<base64-payload>.<hex-signature>`
 *
 * The payload is a JSON object with { uid, email, store_slug, store_id,
 * tenant_id }. The signature is HMAC-SHA256 over the raw payload bytes
 * using SESSION_ENCRYPT_KEY from the environment.
 *
 * This prevents cookie forgery — a user who crafts a fake payload
 * cannot produce a valid signature without the server-side key.
 */

import { createHmac, timingSafeEqual } from "node:crypto";

const DEV_SESSION_KEY = "dev-session-key-min-32-bytes!!!";

/** Matches the cookie's maxAge so the server-side and browser-side
 *  lifetimes agree. marketplace-api enforces the same `exp` claim. */
export const SESSION_TTL_SECONDS = 60 * 60 * 24 * 30;

export interface CustomerSession {
  uid: string;
  email: string;
  store_slug: string;
  store_id: string;
  tenant_id: string;
  /** Unix seconds. Present on every cookie this module mints. */
  iat?: number;
  exp?: number;
}

export interface CustomerSessionScope {
  storeSlug?: string | null;
  storeId?: string | null;
  tenantId?: string | null;
}

function sign(payload: string): string {
  return createHmac("sha256", sessionKey()).update(payload).digest("hex");
}

/** Encode + sign a session into a cookie value. */
export function encodeSession(
  session: CustomerSession,
  ttlSeconds: number = SESSION_TTL_SECONDS,
): string {
  const iat = Math.floor(Date.now() / 1000);
  const claims: CustomerSession = { ...session, iat, exp: iat + ttlSeconds };
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64");
  const sig = sign(payload);
  return `${payload}.${sig}`;
}

/** Decode + verify a signed cookie value. Returns null if the
 *  signature is invalid or the payload is malformed. */
export function decodeSession(cookieValue: string): CustomerSession | null {
  const dotIndex = cookieValue.lastIndexOf(".");
  if (dotIndex < 0) return null;

  const payload = cookieValue.slice(0, dotIndex);
  const sig = cookieValue.slice(dotIndex + 1);

  // Constant-time comparison to prevent timing attacks.
  const expected = sign(payload);
  if (sig.length !== expected.length) return null;
  if (!timingSafeEqual(Buffer.from(sig), Buffer.from(expected))) return null;

  return parsePayload(payload);
}

export function decodeSessionForScope(
  cookieValue: string,
  scope: CustomerSessionScope,
): CustomerSession | null {
  const session = decodeSession(cookieValue);
  if (!session || !sessionMatchesScope(session, scope)) return null;
  return session;
}

export function sessionMatchesScope(
  session: CustomerSession,
  scope: CustomerSessionScope,
): boolean {
  if (scope.storeSlug && session.store_slug !== scope.storeSlug) return false;
  if (scope.storeId && session.store_id !== scope.storeId) return false;
  if (scope.tenantId && session.tenant_id !== scope.tenantId) return false;
  return true;
}

function parsePayload(base64: string): CustomerSession | null {
  try {
    const json = Buffer.from(base64, "base64").toString();
    const parsed = JSON.parse(json) as Partial<CustomerSession>;
    if (!parsed.uid || !parsed.email) return null;
    // A cookie with no exp can never be aged out or revoked, so it is
    // rejected outright rather than trusted indefinitely.
    if (typeof parsed.exp !== "number") return null;
    if (Math.floor(Date.now() / 1000) > parsed.exp) return null;
    return {
      uid: parsed.uid,
      email: parsed.email,
      store_slug: parsed.store_slug ?? "",
      store_id: parsed.store_id ?? "",
      tenant_id: parsed.tenant_id ?? "",
      iat: parsed.iat,
      exp: parsed.exp,
    };
  } catch {
    return null;
  }
}

function sessionKey(): string {
  const key = process.env.SESSION_ENCRYPT_KEY;
  if (key) return key;
  if (process.env.NODE_ENV === "production") {
    throw new Error("SESSION_ENCRYPT_KEY is required in production");
  }
  return DEV_SESSION_KEY;
}
