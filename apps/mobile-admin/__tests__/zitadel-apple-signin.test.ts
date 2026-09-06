/**
 * "Sign in with Apple" over Zitadel's IDP-intent flow (#771).
 *
 * Mirrors zitadel-google-signin.test.ts, because the two share one helper —
 * these tests exist to hold the PER-PROVIDER half of that helper still: the
 * provider name on both legs and the Apple-named failure code.
 *
 * The provider on `idp/finish` is the defect this file exists to prevent.
 * auth-bff resolves an IDP id from the provider the request names and pins
 * the intent against it, and treats an ABSENT provider as Google for
 * back-compat with builds shipped before #771 — so an Apple intent finished
 * without one is checked against Google's id and refused, with no wire error
 * that says so.
 */
import { createZitadelSignIn } from "@repo/mobile-shared/auth/zitadel-signin";
import { ZitadelAuthError } from "@repo/mobile-shared/auth/zitadel-client";
import { zitadelSession } from "@repo/mobile-shared/auth/zitadel-session";

jest.mock("expo-secure-store", () => {
  const mem: Record<string, string> = {};
  return {
    __mem: mem,
    getItemAsync: jest.fn(async (k: string) => mem[k] ?? null),
    setItemAsync: jest.fn(async (k: string, v: string) => {
      mem[k] = v;
    }),
    deleteItemAsync: jest.fn(async (k: string) => {
      delete mem[k];
    }),
  };
});

const mem = (jest.requireMock("expo-secure-store") as { __mem: Record<string, string> }).__mem;

const REDIRECT = "mark8ly-admin://auth/idp";

interface Reply {
  body: unknown;
  status?: number;
}

/** Each call to fetch consumes the next reply, so a start→finish flow can
 * be driven end to end rather than mocked one leg at a time. */
function respondInOrder(replies: Reply[]): jest.Mock {
  const calls = jest.fn();
  let i = 0;
  globalThis.fetch = jest.fn((url: unknown, init: unknown) => {
    calls(url, init);
    const reply = replies[Math.min(i, replies.length - 1)];
    i += 1;
    const status = reply?.status ?? 200;
    return Promise.resolve({
      ok: status < 300,
      status,
      json: () => Promise.resolve(reply?.body),
    } as unknown as Response);
  }) as unknown as typeof fetch;
  return calls;
}

function openerReturning(result: unknown) {
  return jest.fn(async () => result as never);
}

function bodyOf(calls: jest.Mock, i: number): Record<string, unknown> {
  return JSON.parse((calls.mock.calls[i]?.[1] as { body: string }).body) as Record<string, unknown>;
}

beforeEach(() => {
  for (const k of Object.keys(mem)) delete mem[k];
});

describe("signInWithApple", () => {
  // The whole happy path: start, browser round trip, finish, persist.
  it("persists tokens and the tenant on a completed Apple sign-in", async () => {
    const calls = respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp?x=1" } } },
      {
        body: {
          data: {
            uid: "u1",
            email: "a@b.test",
            tenant_id: "t-1",
            access_token: "AT",
            refresh_token: "RT",
            expires_in: 3600,
          },
        },
      },
    ]);
    const openAuthSession = openerReturning({
      type: "success",
      url: `${REDIRECT}?id=i1&token=t1`,
    });
    const setTenantId = jest.fn();

    const out = await createZitadelSignIn("https://api.mark8ly.com").signInWithApple(
      { redirectUrl: REDIRECT, openAuthSession },
      setTenantId,
    );

    expect(out.kind).toBe("signed_in");
    expect(await zitadelSession.accessTokenIfFresh()).toBe("AT");
    expect(setTenantId).toHaveBeenCalledWith("t-1");

    // The browser is opened with the server's authUrl and the app's own
    // scheme — the session cannot close on anything else.
    expect(openAuthSession).toHaveBeenCalledWith("https://zitadel.test/idp?x=1", REDIRECT);
    expect(calls.mock.calls[0]?.[0]).toBe(
      "https://api.mark8ly.com/api/v1/mobile/admin/auth/idp/start",
    );
    expect(calls.mock.calls[1]?.[0]).toBe(
      "https://api.mark8ly.com/api/v1/mobile/admin/auth/idp/finish",
    );
  });

  // THE test this file exists for. An omitted provider on finish is not a
  // wire error — the server reads it as Google and refuses the Apple intent.
  it("names apple on BOTH legs, so finish pins the right IDP", async () => {
    const calls = respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp" } } },
      { body: { data: { uid: "u1", email: "a@b.test", tenant_id: "t-1", access_token: "AT" } } },
    ]);

    await createZitadelSignIn("https://api.mark8ly.com").signInWithApple(
      {
        redirectUrl: REDIRECT,
        openAuthSession: openerReturning({ type: "success", url: `${REDIRECT}?id=i1&token=t1` }),
      },
      jest.fn(),
    );

    // No return URL is ever sent: the server builds it, because Zitadel
    // does not validate successUrl at all.
    expect(bodyOf(calls, 0)).toEqual({ provider: "apple" });
    expect(bodyOf(calls, 1)).toEqual({
      provider: "apple",
      intent_id: "i1",
      intent_token: "t1",
    });
  });

  // Google must keep naming itself now that the two share one helper.
  it("still names google on both legs for the Google path", async () => {
    const calls = respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp" } } },
      { body: { data: { uid: "u1", email: "a@b.test", tenant_id: "t-1", access_token: "AT" } } },
    ]);

    await createZitadelSignIn("https://api.mark8ly.com").signInWithGoogle(
      {
        redirectUrl: REDIRECT,
        openAuthSession: openerReturning({ type: "success", url: `${REDIRECT}?id=i1&token=t1` }),
      },
      jest.fn(),
    );

    expect(bodyOf(calls, 0)).toEqual({ provider: "google" });
    expect(bodyOf(calls, 1)).toEqual({
      provider: "google",
      intent_id: "i1",
      intent_token: "t1",
    });
  });

  // A fresh install is always an unrecognised device, so a step-up is the
  // ORDINARY outcome here. Nothing may be persisted while it is
  // outstanding: a stored tenant with no token leaves the app sending
  // X-Acting-Tenant-Id unauthenticated.
  it("returns kind:otp with a pendingToken and persists nothing", async () => {
    respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp" } } },
      {
        body: {
          data: {
            email: "a@b.test",
            tenant_id: "t-1",
            email_otp_required: true,
            pending_token: "sealed",
          },
        },
      },
    ]);
    const setTenantId = jest.fn();

    const out = await createZitadelSignIn("https://api.mark8ly.com").signInWithApple(
      {
        redirectUrl: REDIRECT,
        openAuthSession: openerReturning({ type: "success", url: `${REDIRECT}?id=i1&token=t1` }),
      },
      setTenantId,
    );

    expect(out.kind).toBe("otp");
    expect(out.pendingToken).toBe("sealed");
    expect(await zitadelSession.read()).toBeNull();
    expect(setTenantId).not.toHaveBeenCalled();
  });

  // An authenticator enrolment must reach the TOTP screen, not the emailed
  // one — the login screen branches on this kind alone.
  it("returns kind:totp for an authenticator challenge", async () => {
    respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp" } } },
      { body: { data: { email: "a@b.test", totp_required: true, pending_token: "sealed" } } },
    ]);

    const out = await createZitadelSignIn("https://api.mark8ly.com").signInWithApple(
      {
        redirectUrl: REDIRECT,
        openAuthSession: openerReturning({ type: "success", url: `${REDIRECT}?id=i1&token=t1` }),
      },
      jest.fn(),
    );

    expect(out.kind).toBe("totp");
    expect(out.pendingToken).toBe("sealed");
  });

  // A refusal must arrive as its own code so the screen shows the real
  // reason. `email_not_verified` is the one Apple private relay produces,
  // and it must NOT be flattened into apple_sign_in_failed.
  it("surfaces the server's own error code rather than a generic failure", async () => {
    respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp" } } },
      { body: { error: "email_not_verified", message: "unverified" }, status: 401 },
    ]);

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple(
        {
          redirectUrl: REDIRECT,
          openAuthSession: openerReturning({ type: "success", url: `${REDIRECT}?id=i1&token=t1` }),
        },
        jest.fn(),
      ),
    ).rejects.toMatchObject({ code: "email_not_verified" });
  });

  // Apple's own refusal comes back on the URL, never as an HTTP status —
  // the exchange never happens at all, and the screen stays silent.
  it("maps access_denied to a silent cancellation and never calls finish", async () => {
    const calls = respondInOrder([{ body: { data: { auth_url: "https://zitadel.test/idp" } } }]);

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple(
        {
          redirectUrl: REDIRECT,
          openAuthSession: openerReturning({
            type: "success",
            url: `${REDIRECT}?id=i1&error=access_denied&error_description=nope`,
          }),
        },
        jest.fn(),
      ),
    ).rejects.toMatchObject({ code: "cancelled" });

    expect(calls).toHaveBeenCalledTimes(1);
  });

  // Apple-named, not Google-named: the screen maps copy from this code
  // alone, so the wrong one tells a merchant the wrong button failed.
  it("maps another Zitadel error to apple_sign_in_failed", async () => {
    respondInOrder([{ body: { data: { auth_url: "https://zitadel.test/idp" } } }]);

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple(
        {
          redirectUrl: REDIRECT,
          openAuthSession: openerReturning({
            type: "success",
            url: `${REDIRECT}?id=i1&error=server_error`,
          }),
        },
        jest.fn(),
      ),
    ).rejects.toMatchObject({ code: "apple_sign_in_failed" });
  });

  // A callback carrying neither a result nor an error is the same failure,
  // not a credential problem.
  it("maps a resultless callback to apple_sign_in_failed", async () => {
    respondInOrder([{ body: { data: { auth_url: "https://zitadel.test/idp" } } }]);

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple(
        {
          redirectUrl: REDIRECT,
          openAuthSession: openerReturning({ type: "success", url: REDIRECT }),
        },
        jest.fn(),
      ),
    ).rejects.toMatchObject({ code: "apple_sign_in_failed" });
  });

  // Dismissing the sheet is a decision, not a failure — the screen stays
  // silent, matching the existing native-sheet behaviour.
  it("treats a dismissed browser session as a cancellation", async () => {
    respondInOrder([{ body: { data: { auth_url: "https://zitadel.test/idp" } } }]);

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple(
        { redirectUrl: REDIRECT, openAuthSession: openerReturning({ type: "cancel" }) },
        jest.fn(),
      ),
    ).rejects.toMatchObject({ code: "cancelled" });
  });

  // An empty authUrl would open a blank browser session, which the user
  // can only read as the button being broken.
  it("refuses an empty auth_url instead of opening a blank session", async () => {
    respondInOrder([{ body: { data: {} } }]);
    const openAuthSession = openerReturning({ type: "cancel" });

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple({ redirectUrl: REDIRECT, openAuthSession }, jest.fn()),
    ).rejects.toBeInstanceOf(ZitadelAuthError);
    expect(openAuthSession).not.toHaveBeenCalled();
  });

  // A challenge the app cannot resume is a code screen whose submit can
  // never succeed.
  it("refuses a step-up that carries no pending token", async () => {
    respondInOrder([
      { body: { data: { auth_url: "https://zitadel.test/idp" } } },
      { body: { data: { email: "a@b.test", email_otp_required: true } } },
    ]);

    const flow = createZitadelSignIn("https://api.mark8ly.com");
    await expect(
      flow.signInWithApple(
        {
          redirectUrl: REDIRECT,
          openAuthSession: openerReturning({ type: "success", url: `${REDIRECT}?id=i1&token=t1` }),
        },
        jest.fn(),
      ),
    ).rejects.toMatchObject({ code: "challenge_unresumable" });
  });
});
