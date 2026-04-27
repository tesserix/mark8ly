// apps/onboarding/lib/gip/customer-signin.ts
//
// Browser-side helper that exchanges a Google credential JWT for a
// GIP id_token in the MP-Customer tenant pool. Used by the
// /auth/google trampoline page that bounces customer sign-ins from
// per-tenant subdomains back to the originating store. Mirrors the
// existing onboarding/lib/gip/signup.ts but targets MP-Customer
// instead of MP-Internal.
//
// Returns a discriminated union: when the email is already in use
// with a different provider AND the GIP project has "Link accounts
// that use the same email" enabled, GIP returns 200 with
// needConfirmation=true plus a pending Google credential. The
// trampoline renders LinkProviderPrompt and the user enters their
// existing password to complete the link via customer-link.ts.

import { publicConfig } from "@/lib/config";

export class CustomerGIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export type CustomerSigninResult =
  | { kind: "ok"; uid: string; idToken: string }
  | {
      kind: "needConfirmation";
      email: string;
      pendingIdpCredential: string;
      verifiedProvider: string[];
    };

export async function signInWithGoogleCustomer(
  googleIdToken: string,
): Promise<CustomerSigninResult> {
  if (!publicConfig.gipApiKey) {
    throw new CustomerGIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipCustomerTenantId) {
    throw new CustomerGIPError(
      "config_missing",
      "GIP customer tenant id is not configured",
    );
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${encodeURIComponent(publicConfig.gipApiKey)}`;
  const requestUri =
    typeof window !== "undefined" ? window.location.origin : "https://mark8ly.com";

  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipCustomerTenantId,
      requestUri,
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
    throw new CustomerGIPError(
      "google_signin_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const data = (await res.json()) as {
    localId?: string;
    idToken?: string;
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

  if (!data.localId || !data.idToken) {
    throw new CustomerGIPError(
      "malformed_response",
      "GIP response missing required fields",
    );
  }

  return { kind: "ok", uid: data.localId, idToken: data.idToken };
}
