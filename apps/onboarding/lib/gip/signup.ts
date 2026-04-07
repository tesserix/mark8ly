// Browser-side GIP signup using the public Identity Toolkit REST API.
//
// We sign the user up with a randomly generated throwaway password.
// The user never sees the password — they sign in via auth-bff session
// cookies after onboarding completes. If they ever need to sign in from
// a different device they'll use "forgot password" → email reset link,
// which is the standard GIP recovery flow.
//
// This avoids needing the Firebase Admin SDK on the server, which would
// require ADC / service account creds. Everything here runs client-side
// with the public Web API key.

import { publicConfig } from "@/lib/config";

export interface SignupResult {
  uid: string;
  idToken: string;
  refreshToken: string;
  expiresIn: number;
}

export class GIPSignupError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

/**
 * Sign up a new GIP user in the configured tenant pool.
 * Returns the id_token immediately — no email verification gate.
 */
export async function signUp(email: string): Promise<SignupResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPSignupError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPSignupError("config_missing", "GIP tenant id is not configured");
  }

  const password = randomPassword(32);
  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=${publicConfig.gipApiKey}`;

  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      tenantId: publicConfig.gipTenantId,
      returnSecureToken: true,
    }),
  });

  if (!res.ok) {
    let body: { error?: { message?: string } } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new GIPSignupError(
      "gip_signup_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const body = (await res.json()) as {
    localId: string;
    idToken: string;
    refreshToken: string;
    expiresIn: string;
  };

  return {
    uid: body.localId,
    idToken: body.idToken,
    refreshToken: body.refreshToken,
    expiresIn: parseInt(body.expiresIn, 10),
  };
}

/**
 * Refresh an id_token using its refresh_token. Used right before auto-login
 * if the original token might have expired (e.g. user took 5+ minutes to
 * fill out the wizard).
 */
export async function refreshIdToken(refreshToken: string): Promise<{
  idToken: string;
  refreshToken: string;
  expiresIn: number;
}> {
  const url = `https://securetoken.googleapis.com/v1/token?key=${publicConfig.gipApiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `grant_type=refresh_token&refresh_token=${encodeURIComponent(refreshToken)}`,
  });
  if (!res.ok) {
    throw new GIPSignupError("refresh_failed", `HTTP ${res.status}`);
  }
  const body = (await res.json()) as {
    id_token: string;
    refresh_token: string;
    expires_in: string;
  };
  return {
    idToken: body.id_token,
    refreshToken: body.refresh_token,
    expiresIn: parseInt(body.expires_in, 10),
  };
}

// randomPassword generates a cryptographically random alphanumeric+symbol
// string of the requested length. Used as a throwaway password during
// onboarding signup.
function randomPassword(length: number): string {
  const charset =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*";
  const buf = new Uint32Array(length);
  crypto.getRandomValues(buf);
  let out = "";
  for (let i = 0; i < length; i++) {
    out += charset[buf[i]! % charset.length];
  }
  return out;
}
