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
  /**
   * Display name now attached to the GIP account, when we know one — either
   * because the IdP supplied it or because we just wrote it.
   *
   * PII. It is only ever sent to Google's Identity Toolkit; it must not be
   * forwarded to our own services or written to any log.
   */
  displayName?: string;
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
 * Attach a display name to the GIP account behind `idToken`.
 *
 * Identity Toolkit `accounts:update`. This is the only place onboarding
 * sends a person's name anywhere: it goes straight to Google, never to our
 * own services and never to a log. `marketplace-api` picks it up from the
 * GIP account record the first time it seeds `user_profiles.display_name`,
 * which is what makes the name show up in web admin and mobile admin.
 */
export async function updateDisplayName(
  idToken: string,
  displayName: string,
): Promise<void> {
  if (!publicConfig.gipApiKey) {
    throw new GIPSignupError("config_missing", "GIP Web API key is not configured");
  }
  const url = `https://identitytoolkit.googleapis.com/v1/accounts:update?key=${publicConfig.gipApiKey}`;

  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      idToken,
      displayName,
      returnSecureToken: false,
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
      "gip_profile_update_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }
}

/**
 * Sign up with email + password, then best-effort attach the merchant's name.
 *
 * The name write is deliberately non-fatal. Once `accounts:signUp` returns
 * the merchant HAS an account; aborting onboarding because a cosmetic
 * profile write failed would strand them with a credential they can't
 * finish signing up with. A blank name is recoverable, a half-created
 * tenant is not — so the second call can fail without the first being
 * undone.
 *
 * `onNameWriteError` is the observability seam for that swallow. It receives
 * the failure *code* only: neither the name nor the raw Identity Toolkit
 * message is passed on, because both can carry PII.
 */
export async function signUpWithName(
  email: string,
  password: string,
  displayName: string,
  onNameWriteError?: (code: string) => void,
): Promise<SignupResult> {
  const result = await signUp(email, password);

  const name = displayName.trim();
  if (!name) return result;

  try {
    await updateDisplayName(result.idToken, name);
    return { ...result, displayName: name };
  } catch (err) {
    onNameWriteError?.(
      err instanceof GIPSignupError ? err.code : "gip_profile_update_failed",
    );
    return result;
  }
}

/**
 * Sign in an existing GIP user with email + password.
 * Used when onboarding detects EMAIL_EXISTS — the user already has a GIP
 * account from a previous onboarding, so we sign them in to get an
 * id_token and proceed to create a new tenant for the same user.
 */
export async function signIn(
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
    throw new GIPSignupError(
      "gip_signin_failed",
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

  // signInWithIdp hands back the name Google already holds for this account,
  // and GIP copies it onto the account record. Keep it rather than dropping
  // it on the floor — there is nothing to ask the merchant to retype.
  const body = (await res.json()) as {
    localId: string;
    idToken: string;
    refreshToken: string;
    expiresIn: string;
    displayName?: string;
  };

  return {
    uid: body.localId,
    idToken: body.idToken,
    refreshToken: body.refreshToken,
    expiresIn: parseInt(body.expiresIn, 10),
    displayName: body.displayName?.trim() || undefined,
  };
}
