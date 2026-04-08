// Browser-side GIP sign-in helpers for the admin app.
//
// Sign-up lives in the onboarding app — admin only signs returning users
// in. Two paths to a fresh GIP id_token:
//
//   1. signInWithPassword — Identity Toolkit accounts:signInWithPassword
//   2. signInWithGoogle   — Identity Toolkit accounts:signInWithIdp,
//      called with the Google credential from getGoogleCredential().
//
// Both return the same shape so the caller can hand off to the signIn
// server action without branching.

import { publicConfig } from "@/lib/config";

export interface GipResult {
  uid: string;
  idToken: string;
  refreshToken: string;
  expiresIn: number;
}

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
): Promise<GipResult> {
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
