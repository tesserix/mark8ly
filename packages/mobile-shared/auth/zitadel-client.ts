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

/**
 * A step-up the caller must collect a code for.
 *
 * `challenge` separates the two: an emailed code and an authenticator-app
 * code are six digits each, but they are read from different places and
 * need different screens and different copy. Collapsing them — which this
 * client used to do — is how a merchant with TOTP enrolled was shown
 * "check your email" for a code no email would ever carry (#686 item 2).
 *
 * Both carry a `pendingToken`, so "a step-up is resumable" is simply true
 * on mobile rather than something each caller has to test for.
 */
export interface StepUpRequired {
  status: "step_up_required";
  challenge: "email_otp" | "totp";
  email: string;
  tenantId: string;
  tenants: TenantMembership[];
  /** Opaque; handed straight back to verifyOtp / verifyTotp. */
  pendingToken: string;
}

export type SignInResult = CompleteSignIn | StepUpRequired;

/**
 * Federated providers this client will start a sign-in with.
 *
 * A union, not a string: auth-bff pins the Zitadel IDP by this exact name
 * and refuses anything else outright, so a typo here must be a compile
 * error rather than a runtime "unsupported_provider" the merchant sees.
 *
 * Apple joined the union in #771, once auth-bff's idp/start AND idp/finish
 * both accepted it. Both legs matter: finish pins the intent's IDP against
 * the id it resolves from the provider the request names, so an Apple
 * intent finished without one is checked against Google's id and refused.
 */
export type IdpProvider = "google" | "apple";

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
  sent?: boolean;
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
  // Resending the emailed code (#686 item 3). It answers with a FRESH
  // pending token, not just `sent`, because the code and the challenge
  // expire together — see resendOtp below.
  otpResend: "/api/v1/mobile/admin/auth/otp/resend",
  // The authenticator-app half (#686 item 2). A separate route because the
  // server verifies it against a Zitadel session, not an emailed value.
  totpVerify: "/api/v1/mobile/admin/auth/totp/verify",
  // Federated sign-in (#686 item 1, Apple in #771). Two legs, because
  // which tenant a federated merchant belongs to is unknowable until the
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

  /**
   * Reads a step-up out of a response, or null when there is none.
   *
   * TOTP is tested FIRST and separately: the two flags can in principle
   * both be set (a policy demanding an authenticator AND an unrecognised
   * device), and the authenticator gate is the one the server is standing
   * at — its pending token resumes into the email challenge afterwards if
   * one is still outstanding.
   */
  function stepUpFrom(d: WireData, fallbackEmail: string): StepUpRequired | null {
    const challenge = d.totp_required
      ? ("totp" as const)
      : d.email_otp_required || d.mfa_required
        ? ("email_otp" as const)
        : null;
    if (!challenge) return null;

    // A challenge with nothing to resume from is unfinishable. Failing
    // here is better than a code screen whose submit can never succeed.
    if (!d.pending_token) {
      throw new ZitadelAuthError(
        "challenge_unresumable",
        "We couldn't start the verification step. Try signing in again.",
      );
    }
    return {
      status: "step_up_required",
      challenge,
      email: d.email ?? fallbackEmail,
      tenantId: d.tenant_id ?? "",
      tenants: d.tenants ?? [],
      pendingToken: d.pending_token,
    };
  }

  function tokensFrom(d: WireData): AuthTokens {
    return {
      accessToken: d.access_token ?? "",
      refreshToken: d.refresh_token ?? "",
      tokenType: d.token_type ?? "Bearer",
      expiresIn: d.expires_in ?? 0,
    };
  }

  /** The shape both code-verification routes answer with. */
  function completed(d: WireData): CompleteSignIn {
    return {
      status: "complete",
      uid: d.uid ?? "",
      email: d.email ?? "",
      tenantId: d.tenant_id ?? "",
      tenants: d.tenants ?? [],
      tokens: tokensFrom(d),
    };
  }

  return {
    async signIn(email: string, password: string): Promise<SignInResult> {
      const d = await post(PATHS.login, { email, password });

      const stepUp = stepUpFrom(d, email);
      if (stepUp) return stepUp;

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
     *
     * The provider is sent, and is not optional (#771). auth-bff resolves
     * an IDP id from it and pins the intent against that id, so omitting
     * it finishes every intent as Google — which refuses an Apple one with
     * a failure that reads as Apple being broken. The server still treats
     * an ABSENT provider as Google, deliberately, so builds shipped before
     * #771 keep working; that back-compat is not a licence to leave it out
     * here.
     */
    async idpFinish(
      provider: IdpProvider,
      intentId: string,
      intentToken: string,
    ): Promise<SignInResult> {
      const d = await post(PATHS.idpFinish, {
        provider,
        intent_id: intentId,
        intent_token: intentToken,
      });

      const stepUp = stepUpFrom(d, "");
      if (stepUp) return stepUp;

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
      return completed(await post(PATHS.otpVerify, { pending_token: pendingToken, code }));
    },

    /**
     * Mails a fresh emailed code and returns the pending token to resume
     * from (#686 item 3).
     *
     * The RETURN VALUE is the point. The emailed code and the sealed
     * challenge expire on the same order of minutes, so the server
     * re-seals rather than only re-mailing; a caller that keeps its
     * original token would then submit the stale half of the pair and be
     * told a correct code was wrong.
     *
     * A spent code budget arrives as `rate_limited` from the generic
     * error path above — its own code, so the screen can say "wait"
     * rather than "try again", which is advice that cannot work.
     */
    async resendOtp(pendingToken: string): Promise<string> {
      const d = await post(PATHS.otpResend, { pending_token: pendingToken });
      if (!d.pending_token) {
        // Better to fail here than to leave the caller verifying against a
        // challenge that is about to expire under a brand new code.
        throw new ZitadelAuthError(
          "challenge_unresumable",
          "We couldn't send a new code. Try signing in again.",
        );
      }
      return d.pending_token;
    },

    /**
     * Completes an authenticator-app challenge (#686 item 2).
     *
     * Its own route, not verifyOtp's: the server checks the code against a
     * Zitadel session rather than an emailed value, and answering one with
     * the other's endpoint fails in a way that reads as a wrong code.
     */
    async verifyTotp(pendingToken: string, code: string): Promise<CompleteSignIn> {
      return completed(await post(PATHS.totpVerify, { pending_token: pendingToken, code }));
    },
  };
}

export type ZitadelAuthClient = ReturnType<typeof createZitadelAuthClient>;
