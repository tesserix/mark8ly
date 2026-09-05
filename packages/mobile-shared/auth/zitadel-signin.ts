import { createZitadelAuthClient, type SignInResult, type CompleteSignIn } from "./zitadel-client";
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
