// apps/storefront/lib/gip/signup.ts
//
// Browser-side helper that exchanges a Google credential JWT for a GIP
// id_token in the MP-Customer tenant pool. Mirrors admin's signInWithGoogle
// but is parameterized on apiKey + tenantId so it fits the storefront's
// per-form gipConfig prop pattern.
//
// On success the caller hands { idToken, uid } to the customerSignIn
// server action, which mints mp_customer_session and triggers
// EnsureProfile in marketplace-api.

export class StorefrontGIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export interface CustomerSignInResult {
  uid: string;
  idToken: string;
}

export async function signInWithGoogleCustomer(
  googleIdToken: string,
  config: { apiKey: string; tenantId: string },
): Promise<CustomerSignInResult> {
  if (!config.apiKey) {
    throw new StorefrontGIPError(
      "config_missing",
      "GIP Web API key is not configured",
    );
  }
  if (!config.tenantId) {
    throw new StorefrontGIPError(
      "config_missing",
      "GIP customer tenant id is not configured",
    );
  }

  const url =
    "https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=" +
    encodeURIComponent(config.apiKey);
  const requestUri =
    typeof window !== "undefined" ? window.location.origin : "https://mark8ly.com";

  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: config.tenantId,
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
    throw new StorefrontGIPError(
      "google_signin_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const wrapper = (await res.json()) as {
    localId: string;
    idToken: string;
  };
  return { uid: wrapper.localId, idToken: wrapper.idToken };
}
