// HMAC-SHA256 signed handoff token used to bridge an admin session
// across TLDs.
//
// The canonical admin session cookie (`m8_session`) is scoped to
// `.mark8ly.com`. When a merchant has a custom admin domain
// (`admin.<merchant-tld>`), a navigation from the canonical host loses
// the cookie at the TLD boundary and the destination loops back to
// login. This token is the bridge: we mint one on the canonical side
// after a verified pick-tenant or sign-in, the destination reads it
// off the URL, hands it to auth-bff's `/auth/mint-from-admin-handoff`
// endpoint, and auth-bff issues a fresh session cookie scoped to the
// custom host.
//
// Wire format mirrors `exchange-code.ts`: `<base64url-payload>.<hex-sig>`.
// Both sides (admin app + auth-bff) HMAC with the same
// SESSION_ENCRYPT_KEY, so only services that already mint sessions
// can mint handoffs — there's no escalation surface beyond what
// those services already had.

import { createHmac, timingSafeEqual } from "node:crypto";

export const ADMIN_HANDOFF_KIND = "admin_handoff_v1" as const;

export interface AdminHandoffClaims {
  kind: typeof ADMIN_HANDOFF_KIND;
  uid: string;
  email: string;
  tenant_id: string;
  store_id?: string;
  /** The custom admin host this handoff is bound to. The receiving
   *  route handler MUST verify the request host matches before
   *  redeeming the code — a stolen code shouldn't redeem at admin.B
   *  even if it was minted for admin.A. */
  target_host: string;
  /** Unix epoch seconds. */
  exp: number;
}

export interface AdminHandoffInput {
  uid: string;
  email: string;
  tenant_id: string;
  store_id?: string;
  target_host: string;
}

export class AdminHandoffError extends Error {
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

export function mintAdminHandoffCode(
  input: AdminHandoffInput,
  key: string,
  ttlSeconds: number,
): string {
  if (!key) throw new AdminHandoffError("missing_key", "key is required");
  if (!input.uid || !input.tenant_id || !input.target_host) {
    throw new AdminHandoffError(
      "incomplete_input",
      "uid, tenant_id, and target_host are required",
    );
  }
  const claims: AdminHandoffClaims = {
    kind: ADMIN_HANDOFF_KIND,
    uid: input.uid,
    email: input.email,
    tenant_id: input.tenant_id,
    store_id: input.store_id,
    target_host: input.target_host,
    exp: Math.floor(Date.now() / 1000) + ttlSeconds,
  };
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const sig = sign(payload, key);
  return `${payload}.${sig}`;
}

export function verifyAdminHandoffCode(
  code: string,
  key: string,
): AdminHandoffClaims {
  if (!key) throw new AdminHandoffError("missing_key", "key is required");
  const dot = code.lastIndexOf(".");
  if (dot < 0) {
    throw new AdminHandoffError("malformed", "code missing signature");
  }

  const payload = code.slice(0, dot);
  const sig = code.slice(dot + 1);
  const expected = sign(payload, key);

  if (sig.length !== expected.length) {
    throw new AdminHandoffError(
      "invalid_signature",
      "signature length mismatch",
    );
  }
  if (!timingSafeEqual(Buffer.from(sig), Buffer.from(expected))) {
    throw new AdminHandoffError("invalid_signature", "signature mismatch");
  }

  let claims: AdminHandoffClaims;
  try {
    claims = JSON.parse(
      Buffer.from(payload, "base64url").toString(),
    ) as AdminHandoffClaims;
  } catch {
    throw new AdminHandoffError(
      "malformed_payload",
      "payload is not valid JSON",
    );
  }

  if (claims.kind !== ADMIN_HANDOFF_KIND) {
    throw new AdminHandoffError(
      "wrong_kind",
      `expected kind ${ADMIN_HANDOFF_KIND}, got ${claims.kind}`,
    );
  }
  if (Math.floor(Date.now() / 1000) >= claims.exp) {
    throw new AdminHandoffError("expired", "code expired");
  }
  if (!claims.uid || !claims.tenant_id || !claims.target_host) {
    throw new AdminHandoffError(
      "incomplete_claims",
      "claims missing uid, tenant_id, or target_host",
    );
  }

  return claims;
}
