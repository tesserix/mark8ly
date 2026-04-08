// Browser-side GIP signup using the public Identity Toolkit REST API.
//
// As of Phase M the merchant chooses their own password during onboarding,
// so they can return later via the /login form (email + password) or via
// "Continue with Google". The password is stored only in GIP — auth-bff
// session cookies are still the long-lived auth artifact for active
// sessions.
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
 *
 * `password` is the merchant-chosen credential collected during onboarding.
 * It is sent to GIP and never stored in our own systems.
 */
export async function signUp(
  email: string,
  password: string,
): Promise<SignupResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPSignupError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPSignupError("config_missing", "GIP tenant id is not configured");
  }
  if (!password || password.length < 8) {
    throw new GIPSignupError(
      "weak_password",
      "Password must be at least 8 characters",
    );
  }
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

/**
 * Sign in an existing GIP user with email + password.
 * Returns the same shape as `signUp` so the caller can treat both flows
 * uniformly when handing off to auth-bff /auth/auto-login.
 */
export async function signInWithPassword(
  email: string,
  password: string,
): Promise<SignupResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPSignupError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPSignupError("config_missing", "GIP tenant id is not configured");
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${publicConfig.gipApiKey}`;
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
    const code = body.error?.message ?? `HTTP ${res.status}`;
    // Map common GIP errors to friendlier wire codes the UI can branch on.
    let friendly = "signin_failed";
    if (code === "EMAIL_NOT_FOUND" || code === "INVALID_PASSWORD" || code === "INVALID_LOGIN_CREDENTIALS") {
      friendly = "invalid_credentials";
    } else if (code.startsWith("USER_DISABLED")) {
      friendly = "account_disabled";
    }
    throw new GIPSignupError(friendly, code);
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
 * Exchange a Google OAuth credential (id_token from Google Identity Services)
 * for a GIP id_token in the configured tenant pool.
 *
 * Uses Identity Toolkit's REST `accounts:signInWithIdp` so we don't need to
 * pull in the firebase JS SDK. The Google credential is obtained client-side
 * via the Google Identity Services library (gsi/client) which handles the
 * popup / one-tap UX.
 */
export async function signInWithGoogle(
  googleIdToken: string,
): Promise<SignupResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPSignupError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPSignupError("config_missing", "GIP tenant id is not configured");
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${publicConfig.gipApiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipTenantId,
      requestUri: typeof window !== "undefined" ? window.location.origin : "https://mark8ly.com",
      postBody: `id_token=${encodeURIComponent(googleIdToken)}&providerId=google.com`,
      returnSecureToken: true,
      returnIdpCredential: true,
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
      "google_signin_failed",
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
