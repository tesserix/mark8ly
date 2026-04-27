// HMAC-SHA256 signed token used to bounce a verified Google sign-in
// from the mark8ly.com trampoline back to the originating tenant
// store. Token format: `<base64url-payload>.<hex-signature>`.
//
// Carries the GIP id_token (already verified by signInWithIdp on the
// trampoline) plus storeSlug + returnTo + intent. The receiving
// storefront /auth/google/finish handler verifies the HMAC and matches
// storeSlug to the request host before completing sign-in.
//
// 30-second default TTL keeps the bearer-style risk small.

import { createHmac, timingSafeEqual } from "node:crypto";

export interface ExchangeCodeClaims {
  idToken: string;
  storeSlug: string;
  returnTo: string;
  intent: "signin" | "signup";
  /** Unix epoch seconds. */
  exp: number;
}

export interface ExchangeCodeInput {
  idToken: string;
  storeSlug: string;
  returnTo: string;
  intent: "signin" | "signup";
}

export class ExchangeCodeError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

function sign(payload: string, key: string): string {
  return createHmac("sha256", key).update(payload).digest("hex");
}

export function mintExchangeCode(
  input: ExchangeCodeInput,
  key: string,
  ttlSeconds: number,
): string {
  if (!key) throw new ExchangeCodeError("missing_key", "key is required");
  const claims: ExchangeCodeClaims = {
    ...input,
    exp: Math.floor(Date.now() / 1000) + ttlSeconds,
  };
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const sig = sign(payload, key);
  return `${payload}.${sig}`;
}

export function verifyExchangeCode(code: string, key: string): ExchangeCodeClaims {
  if (!key) throw new ExchangeCodeError("missing_key", "key is required");
  const dot = code.lastIndexOf(".");
  if (dot < 0) throw new ExchangeCodeError("malformed", "code missing signature");

  const payload = code.slice(0, dot);
  const sig = code.slice(dot + 1);
  const expected = sign(payload, key);

  if (sig.length !== expected.length) {
    throw new ExchangeCodeError("invalid_signature", "signature length mismatch");
  }
  if (!timingSafeEqual(Buffer.from(sig), Buffer.from(expected))) {
    throw new ExchangeCodeError("invalid_signature", "signature mismatch");
  }

  let claims: ExchangeCodeClaims;
  try {
    claims = JSON.parse(Buffer.from(payload, "base64url").toString()) as ExchangeCodeClaims;
  } catch {
    throw new ExchangeCodeError("malformed_payload", "payload is not valid JSON");
  }

  if (Math.floor(Date.now() / 1000) >= claims.exp) {
    throw new ExchangeCodeError("expired", "code expired");
  }

  return claims;
}
