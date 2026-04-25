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

export interface CustomerSession {
  uid: string;
  email: string;
  store_slug: string;
  store_id: string;
  tenant_id: string;
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
export function encodeSession(session: CustomerSession): string {
  const payload = Buffer.from(JSON.stringify(session)).toString("base64");
  const sig = sign(payload);
  return `${payload}.${sig}`;
}

/** Decode + verify a signed cookie value. Returns null if the
 *  signature is invalid or the payload is malformed. */
export function decodeSession(cookieValue: string): CustomerSession | null {
  const dotIndex = cookieValue.lastIndexOf(".");
  if (dotIndex < 0) {
    // Legacy unsigned cookie (from the brief v0 window). Accept it
    // but log a warning — it'll be replaced on next sign-in.
    return decodeLegacy(cookieValue);
  }

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

function decodeLegacy(raw: string): CustomerSession | null {
  if (process.env.NODE_ENV === "production") {
    return null;
  }
  // eslint-disable-next-line no-console
  console.warn(
    "[storefront] unsigned mp_customer_session cookie detected — " +
      "will be replaced with a signed version on next sign-in.",
  );
  return parsePayload(raw);
}

function parsePayload(base64: string): CustomerSession | null {
  try {
    const json = Buffer.from(base64, "base64").toString();
    const parsed = JSON.parse(json) as Partial<CustomerSession>;
    if (!parsed.uid || !parsed.email) return null;
    return {
      uid: parsed.uid,
      email: parsed.email,
      store_slug: parsed.store_slug ?? "",
      store_id: parsed.store_id ?? "",
      tenant_id: parsed.tenant_id ?? "",
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
