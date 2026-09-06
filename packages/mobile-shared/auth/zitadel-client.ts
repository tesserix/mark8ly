/**
 * Network layer for Zitadel-backed mobile sign-in (#686).
 *
 * The app keeps mark8ly's own login form rather than sending merchants to
 * Zitadel's hosted login, so it posts credentials to marketplace-api's
 * public front door, which proxies to auth-bff. Nothing here talks to
 * Zitadel directly and nothing here holds a client secret.
 *
 * Deliberately free of React and of storage so the wire contract and its
 * error mapping can be tested on their own — this is the layer where a
 * wrong status-to-copy mapping strands a merchant.
 */

export interface ZitadelAuthClientConfig {
  baseUrl: string;
}

export interface AuthTokens {
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  /** Seconds from issue, as returned. Absolute expiry is the caller's job. */
  expiresIn: number;
}

export interface TenantMembership {
  tenant_id: string;
  name: string;
  role: string;
}

export interface CompleteSignIn {
  status: "complete";
  uid: string;
  email: string;
  tenantId: string;
  tenants: TenantMembership[];
  tokens: AuthTokens;
}

export interface OtpRequired {
  status: "otp_required";
  email: string;
  tenantId: string;
  tenants: TenantMembership[];
  /** Opaque; handed straight back to verifyOtp. */
  pendingToken: string;
}

export type SignInResult = CompleteSignIn | OtpRequired;

/**
 * Federated providers this client will start a sign-in with.
 *
 * A union, not a string: auth-bff pins the Zitadel IDP by this exact name
 * and refuses anything else outright, so a typo here must be a compile
 * error rather than a runtime "unsupported_provider" the merchant sees.
 * Apple is deliberately absent — it is provisioned on the Zitadel org but
 * has never been exercised end to end (see auth-bff's zitadellogin
 * README), and adding it here without that is a promise the backend does
 * not keep.
 */
export type IdpProvider = "google";

/**
 * A failure with a stable `code`, so screens map to copy from the code
 * rather than from a status number or a message string.
 *
 * `auth_unavailable` matters most: it means the credential may have been
 * CORRECT and the service was not reachable. Showing "wrong password" there
 * makes a merchant retype a correct one indefinitely.
 */
export class ZitadelAuthError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ZitadelAuthError";
  }
}

interface WireData {
  uid?: string;
  email?: string;
  tenant_id?: string;
  tenants?: TenantMembership[];
  access_token?: string;
  refresh_token?: string;
  token_type?: string;
  expires_in?: number;
  email_otp_required?: boolean;
  mfa_required?: boolean;
  totp_required?: boolean;
  pending_token?: string;
  /** idp/start only. */
  auth_url?: string;
}

const PATHS = {
  login: "/api/v1/mobile/admin/auth/login",
  otpVerify: "/api/v1/mobile/admin/auth/otp/verify",
  // Federated sign-in (#686 item 1). Two legs, because which tenant a
  // Google-authenticated merchant belongs to is unknowable until the
  // identity has been resolved: start opens the intent, finish exchanges
  // it AND resolves the tenant server-side.
  idpStart: "/api/v1/mobile/admin/auth/idp/start",
  idpFinish: "/api/v1/mobile/admin/auth/idp/finish",
} as const;

export function createZitadelAuthClient(config: ZitadelAuthClientConfig) {
  async function post(path: string, body: unknown): Promise<WireData> {
    let res: Response;
    try {
      res = await fetch(config.baseUrl + path, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify(body),
      });
    } catch {
      // Offline and server-down are the same to the user: try again later.
      throw new ZitadelAuthError("network", "No connection. Check your network and try again.");
    }

    let parsed: { data?: WireData; error?: string; message?: string } = {};
    try {
      parsed = (await res.json()) as typeof parsed;
    } catch {
      parsed = {};
    }

    if (!res.ok) {
      // The server's own code is preserved rather than re-derived from the
      // status, because 401 covers both a wrong password and a wrong OTP
      // code, which need different copy.
      throw new ZitadelAuthError(parsed.error ?? `http_${res.status}`, parsed.message ?? "");
    }
    return parsed.data ?? {};
  }

  function tokensFrom(d: WireData): AuthTokens {
    return {
      accessToken: d.access_token ?? "",
      refreshToken: d.refresh_token ?? "",
      tokenType: d.token_type ?? "Bearer",
      expiresIn: d.expires_in ?? 0,
    };
  }

  return {
    async signIn(email: string, password: string): Promise<SignInResult> {
      const d = await post(PATHS.login, { email, password });

      if (d.email_otp_required || d.mfa_required || d.totp_required) {
        // A challenge with nothing to resume from is unfinishable. Failing
        // here is better than a code screen whose submit can never succeed.
        if (!d.pending_token) {
          throw new ZitadelAuthError(
            "challenge_unresumable",
            "We couldn't start the verification step. Try signing in again.",
          );
        }
        return {
          status: "otp_required",
          email: d.email ?? email,
          tenantId: d.tenant_id ?? "",
          tenants: d.tenants ?? [],
          pendingToken: d.pending_token,
        };
      }

      return {
        status: "complete",
        uid: d.uid ?? "",
        email: d.email ?? email,
        tenantId: d.tenant_id ?? "",
        tenants: d.tenants ?? [],
        tokens: tokensFrom(d),
      };
    },

    /**
     * Opens a federated sign-in and returns the URL to show the user.
     *
     * No return URL is sent: the server builds it from configuration.
     * Zitadel does not validate an intent's successUrl at all, so a
     * client-supplied one would put the server's allowlist alone between
     * an attacker and a completed admin sign-in.
     */
    async idpStart(provider: IdpProvider): Promise<string> {
      const d = await post(PATHS.idpStart, { provider });
      if (!d.auth_url) {
        // Returning "" would open a blank browser session, which the user
        // can only read as the button being broken.
        throw new ZitadelAuthError(
          "auth_unavailable",
          "Sign-in is temporarily unavailable. Try again shortly.",
        );
      }
      return d.auth_url;
    },

    /**
     * Exchanges the intent the browser handed back for a session. Answers
     * with the SAME union `signIn` does — tokens, or an outstanding OTP —
     * because the server answers with the same body.
     */
    async idpFinish(intentId: string, intentToken: string): Promise<SignInResult> {
      const d = await post(PATHS.idpFinish, { intent_id: intentId, intent_token: intentToken });

      if (d.email_otp_required || d.mfa_required || d.totp_required) {
        if (!d.pending_token) {
          throw new ZitadelAuthError(
            "challenge_unresumable",
            "We couldn't start the verification step. Try signing in again.",
          );
        }
        return {
          status: "otp_required",
          email: d.email ?? "",
          tenantId: d.tenant_id ?? "",
          tenants: d.tenants ?? [],
          pendingToken: d.pending_token,
        };
      }

      return {
        status: "complete",
        uid: d.uid ?? "",
        email: d.email ?? "",
        tenantId: d.tenant_id ?? "",
        tenants: d.tenants ?? [],
        tokens: tokensFrom(d),
      };
    },

    async verifyOtp(pendingToken: string, code: string): Promise<CompleteSignIn> {
      const d = await post(PATHS.otpVerify, { pending_token: pendingToken, code });
      return {
        status: "complete",
        uid: d.uid ?? "",
        email: d.email ?? "",
        tenantId: d.tenant_id ?? "",
        tenants: d.tenants ?? [],
        tokens: tokensFrom(d),
      };
    },
  };
}

export type ZitadelAuthClient = ReturnType<typeof createZitadelAuthClient>;
