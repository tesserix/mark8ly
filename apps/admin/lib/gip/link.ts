// apps/admin/lib/gip/link.ts
//
// Completes the GIP account-merge handshake for the MP-Internal pool.
// Called from SignInForm after the user enters their existing password
// in the LinkProviderPrompt overlay.
//
// Two REST calls:
//   1. accounts:signInWithPassword — verify the password, get a fresh
//      GIP id_token bound to the existing user.
//   2. accounts:signInWithIdp — re-call with the pending Google
//      credential AND the id_token from step (1). With the user signed
//      in, GIP links the Google provider to the existing account.

import { publicConfig } from "@/lib/config";
import { GIPError } from "./signup";

export interface LinkResult {
  uid: string;
  idToken: string;
  refreshToken: string;
  expiresIn: number;
}

export async function linkGoogleToInternalPassword(
  email: string,
  password: string,
  pendingIdpCredential: string,
): Promise<LinkResult> {
  if (!publicConfig.gipApiKey) {
    throw new GIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipTenantId) {
    throw new GIPError("config_missing", "GIP tenant id is not configured");
  }

  // Step 1: password sign-in.
  const passUrl = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${encodeURIComponent(publicConfig.gipApiKey)}`;
  const passRes = await fetch(passUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      tenantId: publicConfig.gipTenantId,
      returnSecureToken: true,
    }),
  });
  if (!passRes.ok) {
    const body = (await passRes.json().catch(() => ({}))) as {
      error?: { message?: string };
    };
    const code = body.error?.message ?? `HTTP ${passRes.status}`;
    if (
      code === "INVALID_PASSWORD" ||
      code === "EMAIL_NOT_FOUND" ||
      code === "INVALID_LOGIN_CREDENTIALS"
    ) {
      throw new GIPError("invalid_credentials", "Email or password is incorrect");
    }
    throw new GIPError("link_failed", code);
  }
  const passData = (await passRes.json()) as {
    localId: string;
    idToken: string;
  };

  // Step 2: link the Google credential to the now-signed-in user.
  const linkUrl = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${encodeURIComponent(publicConfig.gipApiKey)}`;
  const requestUri =
    typeof window !== "undefined" ? window.location.origin : "https://admin.mark8ly.com";
  const linkRes = await fetch(linkUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipTenantId,
      requestUri,
      postBody: `id_token=${encodeURIComponent(pendingIdpCredential)}&providerId=google.com`,
      returnSecureToken: true,
      returnIdpCredential: false,
      idToken: passData.idToken,
    }),
  });
  if (!linkRes.ok) {
    const body = (await linkRes.json().catch(() => ({}))) as {
      error?: { message?: string };
    };
    throw new GIPError("link_failed", body.error?.message ?? `HTTP ${linkRes.status}`);
  }
  const linkData = (await linkRes.json()) as {
    localId: string;
    idToken: string;
    refreshToken: string;
    expiresIn: string;
  };

  return {
    uid: linkData.localId,
    idToken: linkData.idToken,
    refreshToken: linkData.refreshToken,
    expiresIn: parseInt(linkData.expiresIn, 10),
  };
}
