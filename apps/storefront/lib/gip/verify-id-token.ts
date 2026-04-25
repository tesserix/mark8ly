import { createPublicKey, createVerify } from "node:crypto";

const CERTS_URL =
  "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com";

interface GoogleCerts {
  certs: Record<string, string>;
  expiresAt: number;
}

let cachedCerts: GoogleCerts | null = null;

export interface VerifiedGIPToken {
  uid: string;
  email: string;
  tenantId: string;
}

interface JWTHeader {
  alg?: string;
  kid?: string;
}

interface JWTPayload {
  aud?: string;
  exp?: number;
  iat?: number;
  iss?: string;
  sub?: string;
  email?: string;
  email_verified?: boolean;
  firebase?: {
    tenant?: string;
  };
}

export class GIPTokenVerificationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "GIPTokenVerificationError";
  }
}

export async function verifyGIPIdToken(
  idToken: string,
  expectedProjectId: string,
  expectedTenantId: string,
): Promise<VerifiedGIPToken> {
  if (!idToken || !expectedProjectId || !expectedTenantId) {
    throw new GIPTokenVerificationError("missing verification configuration");
  }

  const parts = idToken.split(".");
  if (parts.length !== 3) {
    throw new GIPTokenVerificationError("malformed token");
  }

  const [headerB64, payloadB64, signatureB64] = parts as [string, string, string];
  const header = parseBase64URLJSON<JWTHeader>(headerB64);
  const payload = parseBase64URLJSON<JWTPayload>(payloadB64);

  if (header.alg !== "RS256" || !header.kid) {
    throw new GIPTokenVerificationError("unsupported token header");
  }

  const cert = (await getGoogleCerts())[header.kid];
  if (!cert) {
    throw new GIPTokenVerificationError("unknown token key");
  }

  const verifier = createVerify("RSA-SHA256");
  verifier.update(`${headerB64}.${payloadB64}`);
  verifier.end();
  const validSignature = verifier.verify(
    createPublicKey(cert),
    Buffer.from(signatureB64, "base64url"),
  );
  if (!validSignature) {
    throw new GIPTokenVerificationError("invalid token signature");
  }

  const now = Math.floor(Date.now() / 1000);
  const issuer = `https://securetoken.google.com/${expectedProjectId}`;
  if (payload.iss !== issuer || payload.aud !== expectedProjectId) {
    throw new GIPTokenVerificationError("token project mismatch");
  }
  if (!payload.exp || payload.exp <= now || !payload.iat || payload.iat > now + 300) {
    throw new GIPTokenVerificationError("token is expired or not yet valid");
  }
  if (payload.firebase?.tenant !== expectedTenantId) {
    throw new GIPTokenVerificationError("token tenant mismatch");
  }
  if (!payload.sub || payload.sub.length > 128 || !payload.email) {
    throw new GIPTokenVerificationError("token missing required identity claims");
  }

  return {
    uid: payload.sub,
    email: payload.email,
    tenantId: payload.firebase.tenant,
  };
}

async function getGoogleCerts(): Promise<Record<string, string>> {
  const now = Date.now();
  if (cachedCerts && cachedCerts.expiresAt > now) {
    return cachedCerts.certs;
  }

  const res = await fetch(CERTS_URL, { cache: "no-store" });
  if (!res.ok) {
    throw new GIPTokenVerificationError("unable to fetch token verification keys");
  }

  const maxAge = parseMaxAge(res.headers.get("cache-control"));
  const certs = (await res.json()) as Record<string, string>;
  cachedCerts = {
    certs,
    expiresAt: now + maxAge * 1000,
  };
  return certs;
}

function parseMaxAge(cacheControl: string | null): number {
  const fallbackSeconds = 60 * 60;
  if (!cacheControl) return fallbackSeconds;
  const match = cacheControl.match(/max-age=(\d+)/i);
  if (!match?.[1]) return fallbackSeconds;
  const parsed = Number.parseInt(match[1], 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallbackSeconds;
}

function parseBase64URLJSON<T>(value: string): T {
  try {
    return JSON.parse(Buffer.from(value, "base64url").toString("utf8")) as T;
  } catch {
    throw new GIPTokenVerificationError("malformed token payload");
  }
}
