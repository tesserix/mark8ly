import {
  createZitadelAuthClient,
  ZitadelAuthError,
  type CompleteSignIn,
  type IdpProvider,
  type SignInResult,
} from "./zitadel-client";
import { parseIdpCallback } from "./zitadel-idp-callback";
import { zitadelSession } from "./zitadel-session";

/**
 * The sign-in flow, joining the wire client to persistence (#686).
 *
 * Which provider a build uses is decided in the APP (see
 * lib/auth-provider.ts), not here: reading process.env from a shared
 * package pulls in Expo's virtual env module, which is not resolvable
 * outside an app's bundler context.
 */

export interface SignInOutcome {
  /** "otp" means a code was emailed and the caller must show the screen. */
  kind: "signed_in" | "otp";
  email: string;
  tenantId: string;
  pendingToken?: string;
}

/**
 * The result shape of `WebBrowser.openAuthSessionAsync`, narrowed to what
 * this flow reads.
 *
 * Injected rather than imported so this package stays free of
 * expo-web-browser: mobile-shared has no Expo config plugin context of its
 * own, and the flow's mapping of a cancelled session to silent copy is the
 * part worth testing — which needs the opener to be substitutable.
 */
export type AuthSessionResult =
  | { type: "success"; url: string }
  | { type: "cancel" | "dismiss" | "locked" | string; url?: string };

export type AuthSessionOpener = (
  authUrl: string,
  redirectUrl: string,
) => Promise<AuthSessionResult>;

export interface FederatedSignInOptions {
  /**
   * The app's own scheme URL the bridge page redirects to, e.g.
   * "mark8ly-admin://auth/idp". Supplied by the app because the scheme is
   * registered in the app's config, not here.
   */
  redirectUrl: string;
  openAuthSession: AuthSessionOpener;
}

export function createZitadelSignIn(baseUrl: string) {
  const client = createZitadelAuthClient({ baseUrl });

  // Tokens and the tenant are persisted TOGETHER, and only on completion.
  // Writing the tenant earlier would leave a half-signed-in app that sends
  // X-Acting-Tenant-Id with no bearer token, which reads as a server fault
  // rather than an unfinished login.
  async function persist(res: CompleteSignIn, setTenantId: (id: string) => void): Promise<void> {
    await zitadelSession.save(
      res.tokens.accessToken,
      res.tokens.refreshToken,
      res.tokens.expiresIn,
    );
    if (res.tenantId) setTenantId(res.tenantId);
  }

  return {
    async signIn(
      email: string,
      password: string,
      setTenantId: (id: string) => void,
    ): Promise<SignInOutcome> {
      const res: SignInResult = await client.signIn(email, password);
      if (res.status === "otp_required") {
        return {
          kind: "otp",
          email: res.email,
          tenantId: res.tenantId,
          pendingToken: res.pendingToken,
        };
      }
      await persist(res, setTenantId);
      return { kind: "signed_in", email: res.email, tenantId: res.tenantId };
    },

    /**
     * "Continue with Google", end to end (#686 item 1).
     *
     * Four steps, and the middle two are why this cannot be one call:
     *   1. ask the server for an authUrl (the return URL is the server's,
     *      never ours — Zitadel does not validate successUrl at all);
     *   2. open it in an authentication session, which closes when the
     *      browser is sent to `redirectUrl`;
     *   3. read `id`/`token` (or `error`) off the URL it closed with;
     *   4. hand those to the server, which resolves the identity, looks
     *      the tenant up by the VERIFIED email, and completes.
     *
     * Returns the same union as `signIn`, including `kind: "otp"`: a
     * fresh install is always an unrecognised device, so a step-up is the
     * ordinary outcome here too, not an edge case.
     */
    async signInWithGoogle(
      opts: FederatedSignInOptions,
      setTenantId: (id: string) => void,
    ): Promise<SignInOutcome> {
      const provider: IdpProvider = "google";
      const authUrl = await client.idpStart(provider);

      const session = await opts.openAuthSession(authUrl, opts.redirectUrl);
      if (session.type !== "success" || !session.url) {
        // Dismissing the sheet is a DECISION, not a failure. It gets its
        // own code so the screen can stay silent, the way the existing
        // provider path already does for a cancelled native sheet.
        throw new ZitadelAuthError("cancelled", "");
      }

      const cb = parseIdpCallback(session.url);
      if (cb.error) {
        // Google's own "the user said no" is a cancellation too, however
        // it arrived. `error_description` is upstream free text and is
        // deliberately never shown.
        if (cb.error === "access_denied" || cb.error === "user_cancelled_login") {
          throw new ZitadelAuthError("cancelled", "");
        }
        throw new ZitadelAuthError(
          "google_sign_in_failed",
          "Couldn't sign you in with Google. Try again.",
        );
      }
      if (!cb.intentId || !cb.intentToken) {
        // The session closed on a URL carrying neither a result nor an
        // error. Reporting it as a credential problem would be a lie.
        throw new ZitadelAuthError(
          "google_sign_in_failed",
          "Couldn't sign you in with Google. Try again.",
        );
      }

      const res: SignInResult = await client.idpFinish(cb.intentId, cb.intentToken);
      if (res.status === "otp_required") {
        return {
          kind: "otp",
          email: res.email,
          tenantId: res.tenantId,
          pendingToken: res.pendingToken,
        };
      }
      await persist(res, setTenantId);
      return { kind: "signed_in", email: res.email, tenantId: res.tenantId };
    },

    async verifyOtp(
      pendingToken: string,
      code: string,
      setTenantId: (id: string) => void,
    ): Promise<SignInOutcome> {
      const res = await client.verifyOtp(pendingToken, code);
      await persist(res, setTenantId);
      return { kind: "signed_in", email: res.email, tenantId: res.tenantId };
    },

    async signOut(): Promise<void> {
      await zitadelSession.clear();
    },
  };
}
