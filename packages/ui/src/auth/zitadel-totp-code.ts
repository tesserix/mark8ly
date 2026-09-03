// HMAC-SHA256 signed code carrying the server-resolved workspace tenant
// across the client round trip between a Zitadel `totp_required`
// outcome and the follow-up TOTP submission.
//
// `signInWithZitadel` resolves `tenantId`/`multipleTenants` server-side
// via `resolveWorkspaceTenant` — the same path `signIn` uses — before
// ever calling Zitadel. When Zitadel itself demands a TOTP step-up, the
// browser needs to carry that resolution forward to
// `confirmZitadelTotp` so the workspace_tenant used to complete the
// login is the one the server already picked, not a value the client
// could set at will (the client cannot reach a tenant it doesn't
// belong to either way — auth-bff re-checks membership — but this
// keeps posture consistent with the GIP path, where the tenant is
// always server-derived, never client-supplied).
//
// Wire format mirrors `exchange-code.ts` / `admin-handoff-code.ts`:
// `<base64url-payload>.<hex-signature>`. Same SESSION_ENCRYPT_KEY as
// the rest of the admin app's short-lived HMAC codes.

import { createHmac, timingSafeEqual } from "node:crypto";

export const ZITADEL_TOTP_CODE_KIND = "zitadel_totp_v1" as const;

export interface ZitadelTotpClaims {
  kind: typeof ZITADEL_TOTP_CODE_KIND;
  tenant_id: string;
  multiple_tenants: boolean;
  /** Unix epoch seconds. */
  exp: number;
}

export interface ZitadelTotpCodeInput {
  tenant_id: string;
  multiple_tenants: boolean;
}

export class ZitadelTotpCodeError extends Error {
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

export function mintZitadelTotpCode(
  input: ZitadelTotpCodeInput,
  key: string,
  ttlSeconds: number,
): string {
  if (!key) throw new ZitadelTotpCodeError("missing_key", "key is required");
  if (!input.tenant_id) {
    throw new ZitadelTotpCodeError("incomplete_input", "tenant_id is required");
  }
  const claims: ZitadelTotpClaims = {
    kind: ZITADEL_TOTP_CODE_KIND,
    tenant_id: input.tenant_id,
    multiple_tenants: input.multiple_tenants,
    exp: Math.floor(Date.now() / 1000) + ttlSeconds,
  };
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const sig = sign(payload, key);
  return `${payload}.${sig}`;
}

export function verifyZitadelTotpCode(code: string, key: string): ZitadelTotpClaims {
  if (!key) throw new ZitadelTotpCodeError("missing_key", "key is required");
  const dot = code.lastIndexOf(".");
  if (dot < 0) {
    throw new ZitadelTotpCodeError("malformed", "code missing signature");
  }

  const payload = code.slice(0, dot);
  const sig = code.slice(dot + 1);
  const expected = sign(payload, key);

  if (sig.length !== expected.length) {
    throw new ZitadelTotpCodeError("invalid_signature", "signature length mismatch");
  }
  if (!timingSafeEqual(Buffer.from(sig), Buffer.from(expected))) {
    throw new ZitadelTotpCodeError("invalid_signature", "signature mismatch");
  }

  let claims: ZitadelTotpClaims;
  try {
    claims = JSON.parse(Buffer.from(payload, "base64url").toString()) as ZitadelTotpClaims;
  } catch {
    throw new ZitadelTotpCodeError("malformed_payload", "payload is not valid JSON");
  }

  if (claims.kind !== ZITADEL_TOTP_CODE_KIND) {
    throw new ZitadelTotpCodeError("wrong_kind", `expected kind ${ZITADEL_TOTP_CODE_KIND}, got ${claims.kind}`);
  }
  if (Math.floor(Date.now() / 1000) >= claims.exp) {
    throw new ZitadelTotpCodeError("expired", "code expired");
  }
  if (!claims.tenant_id) {
    throw new ZitadelTotpCodeError("incomplete_claims", "claims missing tenant_id");
  }

  return claims;
}
