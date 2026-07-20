// Browser-side GIP sign-in helpers for the admin app.
//
// Paths to a fresh GIP id_token:
//
//   1. signInWithPassword — Identity Toolkit accounts:signInWithPassword
//   2. signInWithGoogle   — Identity Toolkit accounts:signInWithIdp,
//      called with the Google credential from getGoogleCredential().
//   3. signUp             — Identity Toolkit accounts:signUp. Phase P
//      added this for the /accept-invite new-account path; the main
//      sign-up funnel still lives in the onboarding app. Admin's copy
//      only runs when a brand-new user clicks an invite link without
//      a Mark8ly account.
//
// All three return the same shape so the caller can hand off to a
// server action without branching on auth method.

import { publicConfig } from "@/lib/config";

export interface GipResult {
  uid: string;
  idToken: string;
  refreshToken: string;
  expiresIn: number;
}

export type GoogleSignInResult =
  | { kind: "ok"; uid: string; idToken: string; refreshToken: string; expiresIn: number }
  | {
      kind: "needConfirmation";
      email: string;
      pendingIdpCredential: string;
      verifiedProvider: string[];
    };

export class GIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export async function signInWithPassword(
  email: string,
  password: string,
): Promise<GipResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPError("config_missing", "GIP tenant id is not configured");
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
    let friendly = "signin_failed";
    if (
      code === "EMAIL_NOT_FOUND" ||
      code === "INVALID_PASSWORD" ||
      code === "INVALID_LOGIN_CREDENTIALS"
    ) {
      friendly = "invalid_credentials";
    } else if (code.startsWith("USER_DISABLED")) {
      friendly = "account_disabled";
    }
    throw new GIPError(friendly, code);
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

export async function signInWithGoogle(
  googleIdToken: string,
): Promise<GoogleSignInResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPError("config_missing", "GIP tenant id is not configured");
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${publicConfig.gipApiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipTenantId,
      requestUri:
        typeof window !== "undefined" ? window.location.origin : "https://admin.mark8ly.com",
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
    throw new GIPError(
      "google_signin_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const data = (await res.json()) as {
    localId?: string;
    idToken?: string;
    refreshToken?: string;
    expiresIn?: string;
    needConfirmation?: boolean;
    email?: string;
    oauthIdToken?: string;
    verifiedProvider?: string[];
  };

  if (data.needConfirmation && data.email && data.oauthIdToken) {
    return {
      kind: "needConfirmation",
      email: data.email,
      pendingIdpCredential: data.oauthIdToken,
      verifiedProvider: data.verifiedProvider ?? [],
    };
  }

  if (!data.localId || !data.idToken || !data.refreshToken || !data.expiresIn) {
    throw new GIPError("malformed_response", "GIP response missing required fields");
  }

  return {
    kind: "ok",
    uid: data.localId,
    idToken: data.idToken,
    refreshToken: data.refreshToken,
    expiresIn: parseInt(data.expiresIn, 10),
  };
}

/**
 * Sign up a new GIP user in the configured tenant pool.
 *
 * Used by the Phase P accept-invite flow when the invitee doesn't
 * yet have a Mark8ly account. Enforces a minimum 8-char password
 * client-side — platform-api re-validates nothing about passwords,
 * GIP owns the full password policy.
 */
/**
 * Send a password reset email via Identity Toolkit accounts:sendOobCode.
 * GIP hosts the actual reset UI at its configured action URL. Returns
 * silently on success so we can show a generic "check your inbox" UX
 * without leaking whether the email is registered.
 */
export async function sendPasswordReset(email: string): Promise<void> {
  if (!publicConfig.gipApiKey) {
    throw new GIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPError("config_missing", "GIP tenant id is not configured");
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:sendOobCode?key=${publicConfig.gipApiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      requestType: "PASSWORD_RESET",
      email,
      tenantId: publicConfig.gipTenantId,
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
    // Don't leak whether the email exists — treat EMAIL_NOT_FOUND as success.
    if (code === "EMAIL_NOT_FOUND") {
      return;
    }
    throw new GIPError("reset_failed", code);
  }
}

export async function signUp(
  email: string,
  password: string,
): Promise<GipResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPError("config_missing", "GIP tenant id is not configured");
  }
  if (!password || password.length < 8) {
    throw new GIPError(
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
    const code = body.error?.message ?? `HTTP ${res.status}`;
    let friendly = "signup_failed";
    if (code.startsWith("EMAIL_EXISTS")) {
      friendly = "email_exists";
    } else if (code.startsWith("WEAK_PASSWORD")) {
      friendly = "weak_password";
    }
    throw new GIPError(friendly, code);
  }

  const signupBody = (await res.json()) as {
    localId: string;
    idToken: string;
    refreshToken: string;
    expiresIn: string;
  };
  return {
    uid: signupBody.localId,
    idToken: signupBody.idToken,
    refreshToken: signupBody.refreshToken,
    expiresIn: parseInt(signupBody.expiresIn, 10),
  };
}

/**
 * Exchanges an Apple identity token for a GIP id_token in our tenant pool.
 *
 * Same shape as signInWithGoogle — including the needConfirmation branch,
 * which fires when GIP has an existing account with this email under a
 * different provider and the caller must re-authenticate to link.
 *
 * The nonce must be the one getAppleCredential() bound into Apple's token;
 * GIP verifies the pair, which is what stops the token being replayed.
 *
 * Caveat worth knowing: if the user picks Apple's "Hide My Email", Apple
 * reports a @privaterelay.appleid.com address rather than their real one.
 * GIP therefore sees a different email, no conflict is raised, and a
 * SEPARATE account is created. Email-based linking cannot fix that — the
 * user must link Apple from account settings while already signed in.
 */
export async function signInWithApple(
  appleIdToken: string,
  nonce: string,
): Promise<GoogleSignInResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPError("config_missing", "GIP tenant id is not configured");
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${publicConfig.gipApiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipTenantId,
      requestUri:
        typeof window !== "undefined" ? window.location.origin : "https://admin.mark8ly.com",
      postBody: `id_token=${encodeURIComponent(appleIdToken)}&providerId=apple.com&nonce=${encodeURIComponent(nonce)}`,
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
    throw new GIPError(
      "apple_signin_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const data = (await res.json()) as {
    localId?: string;
    idToken?: string;
    refreshToken?: string;
    expiresIn?: string;
    needConfirmation?: boolean;
    email?: string;
    oauthIdToken?: string;
    verifiedProvider?: string[];
  };

  if (data.needConfirmation && data.email && data.oauthIdToken) {
    return {
      kind: "needConfirmation",
      email: data.email,
      pendingIdpCredential: data.oauthIdToken,
      verifiedProvider: data.verifiedProvider ?? [],
    };
  }

  if (!data.localId || !data.idToken || !data.refreshToken || !data.expiresIn) {
    throw new GIPError("malformed_response", "GIP response missing required fields");
  }

  return {
    kind: "ok",
    uid: data.localId,
    idToken: data.idToken,
    refreshToken: data.refreshToken,
    expiresIn: parseInt(data.expiresIn, 10),
  };
}
