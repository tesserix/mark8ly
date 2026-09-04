import { afterEach, describe, expect, it, vi } from "vitest";

import {
  AuthBffCustomerError,
  finishCustomerIDPIntent,
  registerCustomerAccount,
  startCustomerIDPIntent,
  verifyCustomerCredential,
  verifyCustomerEmailCode,
  verifyCustomerTotp,
} from "./auth-bff-customer";

const SECRET_PASSWORD = "correct-horse-battery-staple";
const SECRET_TEST_CODE = "A1B2C3"; // low-entropy, test-only — never a live credential

function stubFetch(status: number, body: unknown) {
  const res = new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
  const fn = vi.fn().mockResolvedValue(res);
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("verifyCustomerCredential", () => {
  const loginArgs = {
    loginName: "customer@example.com",
    password: SECRET_PASSWORD,
  };

  it("sends the exact snake_case wire fields for /auth/customer/login", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "customer@example.com" },
    });

    await verifyCustomerCredential(loginArgs);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("http://localhost:8087/auth/customer/login");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      login_name: "customer@example.com",
      password: SECRET_PASSWORD,
    });
  });

  it("maps a 2xx identity response to the completed outcome", async () => {
    stubFetch(200, { data: { uid: "u1", email: "customer@example.com" } });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });
  });

  it("maps a totp_required response to the factor-required outcome", async () => {
    stubFetch(200, {
      totp_required: true,
      session_id: "sess-1",
      session_token: "sess-token-1",
    });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({
      kind: "totp_required",
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
    });
  });

  it("maps a handoff response to the handoff outcome", async () => {
    stubFetch(200, {
      handoff_url: "https://zitadel.example.com/ui/v2/login/login",
    });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({
      kind: "handoff",
      handoffUrl: "https://zitadel.example.com/ui/v2/login/login",
    });
  });

  it("collapses a 401 to the single rejected outcome regardless of which error auth-bff logged internally", async () => {
    stubFetch(401, { error: "invalid_credentials" });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({ kind: "rejected" });
  });

  it("does not distinguish a 401 by response body content (no enumeration oracle)", async () => {
    // Same status, arbitrary/different error body — must still collapse
    // to the identical outcome as the canonical invalid_credentials case.
    stubFetch(401, { error: "user_not_found_but_should_never_say_so" });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({ kind: "rejected" });
  });

  it("surfaces a 401 email_not_verified as its own outcome instead of collapsing it", async () => {
    // Unlike invalid_credentials/invalid_totp above, this ONE 401 code is
    // safe to distinguish — reaching it already required
    // CreatePasswordSession to succeed (see customer_handler.go:363-369),
    // so the caller already holds the correct password.
    stubFetch(401, { error: "email_not_verified" });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({ kind: "email_not_verified" });
  });

  it("still collapses a 401 with an unrecognised error code to rejected", async () => {
    stubFetch(401, { error: "some_future_code_this_client_does_not_know" });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({ kind: "rejected" });
  });

  it("still collapses a 401 with no error field at all to rejected", async () => {
    stubFetch(401, {});

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({ kind: "rejected" });
  });

  it("throws a distinguishable error on a 5xx, not the credential-rejection outcome", async () => {
    stubFetch(503, { error: "zitadel_unavailable" });

    await expect(verifyCustomerCredential(loginArgs)).rejects.toBeInstanceOf(
      AuthBffCustomerError,
    );
    await expect(verifyCustomerCredential(loginArgs)).rejects.toMatchObject({
      status: 503,
    });
  });

  it("throws a distinguishable error on a transport failure", async () => {
    const fn = vi.fn().mockRejectedValue(new Error("ECONNREFUSED"));
    vi.stubGlobal("fetch", fn);

    const rejection = verifyCustomerCredential(loginArgs);
    await expect(rejection).rejects.toBeInstanceOf(AuthBffCustomerError);
    await expect(rejection).rejects.toMatchObject({ status: 0 });
  });

  it("never lets the password appear in a thrown error", async () => {
    stubFetch(500, { error: "internal_error" });

    try {
      await verifyCustomerCredential(loginArgs);
      throw new Error("expected verifyCustomerCredential to throw");
    } catch (err) {
      expect(err).toBeInstanceOf(AuthBffCustomerError);
      const serialised = JSON.stringify(err) + String((err as Error).message) + String((err as Error).stack);
      expect(serialised).not.toContain(SECRET_PASSWORD);
    }
  });

  it("never lets the password appear in a rejection outcome value", async () => {
    stubFetch(401, { error: "invalid_credentials" });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(JSON.stringify(outcome)).not.toContain(SECRET_PASSWORD);
  });
});

describe("verifyCustomerTotp", () => {
  const totpArgs = {
    sessionId: "sess-1",
    sessionToken: "sess-token-1",
    code: "123456",
  };

  it("sends the exact snake_case wire fields for /auth/customer/totp", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "customer@example.com" },
    });

    await verifyCustomerTotp(totpArgs);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("http://localhost:8087/auth/customer/totp");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      session_id: "sess-1",
      session_token: "sess-token-1",
      code: "123456",
    });
  });

  it("maps a 2xx identity response to the completed outcome", async () => {
    stubFetch(200, { data: { uid: "u1", email: "customer@example.com" } });

    const outcome = await verifyCustomerTotp(totpArgs);

    expect(outcome).toEqual({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });
  });

  it("collapses a 401 (bad code or vanished session) to the single rejected outcome", async () => {
    stubFetch(401, { error: "invalid_totp" });

    const outcome = await verifyCustomerTotp(totpArgs);

    expect(outcome).toEqual({ kind: "rejected" });
  });

  it("throws a distinguishable error on a 5xx", async () => {
    stubFetch(503, { error: "zitadel_unavailable" });

    await expect(verifyCustomerTotp(totpArgs)).rejects.toBeInstanceOf(
      AuthBffCustomerError,
    );
  });
});

describe("startCustomerIDPIntent", () => {
  it("sends the exact snake_case wire field for /auth/customer/idp/start", async () => {
    const fetchMock = stubFetch(200, {
      auth_url: "https://zitadel.example.com/idp/authorize/abc",
    });

    await startCustomerIDPIntent("https://shop.mark8ly.com/auth/idp/finish");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("http://localhost:8087/auth/customer/idp/start");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      return_url: "https://shop.mark8ly.com/auth/idp/finish",
    });
  });

  it("returns the auth_url on a 2xx response", async () => {
    stubFetch(200, { auth_url: "https://zitadel.example.com/idp/authorize/abc" });

    const authUrl = await startCustomerIDPIntent(
      "https://shop.mark8ly.com/auth/idp/finish",
    );

    expect(authUrl).toBe("https://zitadel.example.com/idp/authorize/abc");
  });

  it("throws a distinguishable error on invalid_return_url (400)", async () => {
    stubFetch(400, { error: "invalid_return_url" });

    const rejection = startCustomerIDPIntent("https://evil.example.com/x");
    await expect(rejection).rejects.toBeInstanceOf(AuthBffCustomerError);
    await expect(rejection).rejects.toMatchObject({
      status: 400,
      code: "invalid_return_url",
    });
  });

  it("throws a distinguishable error on a 5xx", async () => {
    stubFetch(503, { error: "zitadel_unavailable" });

    await expect(
      startCustomerIDPIntent("https://shop.mark8ly.com/auth/idp/finish"),
    ).rejects.toMatchObject({ status: 503, code: "zitadel_unavailable" });
  });
});

describe("finishCustomerIDPIntent", () => {
  const finishArgs = { intentId: "intent-1", intentToken: "intent-token-1" };

  it("sends the exact snake_case wire fields for /auth/customer/idp/finish, and NEVER a user field", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "customer@example.com" },
    });

    await finishCustomerIDPIntent(finishArgs);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("http://localhost:8087/auth/customer/idp/finish");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      intent_id: "intent-1",
      intent_token: "intent-token-1",
    });
    expect(parsedBody).not.toHaveProperty("user");
  });

  it("has no parameter for a `user` value at all — a tampered one has nothing to attach to", async () => {
    stubFetch(200, { data: { uid: "u1", email: "customer@example.com" } });

    // finishCustomerIDPIntent's signature only accepts intentId/intentToken
    // (see FinishCustomerIDPArgs) — there is no field to smuggle an
    // attacker-controlled `user` value through even if a caller tried.
    const outcome = await finishCustomerIDPIntent({
      ...finishArgs,
      // @ts-expect-error — proving the extra field is not part of the type
      user: "attacker@evil.com",
    });

    expect(outcome).toEqual({ kind: "complete", uid: "u1", email: "customer@example.com" });
  });

  it("maps a 2xx identity response to the completed outcome", async () => {
    stubFetch(200, { data: { uid: "u1", email: "customer@example.com" } });

    const outcome = await finishCustomerIDPIntent(finishArgs);

    expect(outcome).toEqual({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });
  });

  it.each([
    ["email_not_verified", 401],
    ["email_taken", 409],
    ["email_ambiguous", 409],
    ["unexpected_idp", 401],
    ["invalid_intent", 401],
    ["zitadel_unavailable", 503],
  ])(
    "maps %s (%i) to a distinct failed outcome, not a thrown error",
    async (code, status) => {
      stubFetch(status, { error: code });

      const outcome = await finishCustomerIDPIntent(finishArgs);

      expect(outcome).toEqual({ kind: "failed", code });
    },
  );

  it("email_taken and email_ambiguous stay distinguishable from each other", async () => {
    stubFetch(409, { error: "email_taken" });
    const taken = await finishCustomerIDPIntent(finishArgs);

    stubFetch(409, { error: "email_ambiguous" });
    const ambiguous = await finishCustomerIDPIntent(finishArgs);

    expect(taken).toEqual({ kind: "failed", code: "email_taken" });
    expect(ambiguous).toEqual({ kind: "failed", code: "email_ambiguous" });
    expect(taken).not.toEqual(ambiguous);
  });

  it("throws (not a failed outcome) on a transport failure", async () => {
    const fn = vi.fn().mockRejectedValue(new Error("ECONNREFUSED"));
    vi.stubGlobal("fetch", fn);

    await expect(finishCustomerIDPIntent(finishArgs)).rejects.toBeInstanceOf(
      AuthBffCustomerError,
    );
  });
});

// The X-Internal-Auth header is what stops auth-bff's publicly reachable
// /auth/customer/{login,totp} from being a credential-validity oracle over
// every user in the Zitadel instance. These pin that this server-side-only
// client actually sends it, and that it never comes from a NEXT_PUBLIC_*
// variable (which would ship the secret to the browser bundle).
describe("internal auth header", () => {
  const loginArgs = {
    loginName: "customer@example.com",
    password: SECRET_PASSWORD,
  };
  const totpArgs = {
    sessionId: "s1",
    sessionToken: "tok-1",
    code: "123456",
  };

  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("sends X-Internal-Auth on /auth/customer/login", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "customer@example.com" },
    });

    await verifyCustomerCredential(loginArgs);

    const [, init] = fetchMock.mock.calls[0]!;
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
  });

  it("sends X-Internal-Auth on /auth/customer/totp", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "customer@example.com" },
    });

    await verifyCustomerTotp(totpArgs);

    const [, init] = fetchMock.mock.calls[0]!;
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
  });

  it("never reads the secret from a NEXT_PUBLIC_ variable", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "");
    vi.stubEnv("NEXT_PUBLIC_MARKETPLACE_INTERNAL_AUTH_SECRET", "leaked");
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "customer@example.com" },
    });

    await verifyCustomerCredential(loginArgs);

    const [, init] = fetchMock.mock.calls[0]!;
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBeUndefined();
    expect(JSON.stringify(init)).not.toContain("leaked");
  });
});

describe("registerCustomerAccount", () => {
  const registerArgs = {
    email: "newcustomer@example.com",
    password: SECRET_PASSWORD,
  };

  it("sends the exact snake_case wire fields for /auth/customer/register", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u-new", email: registerArgs.email },
    });

    await registerCustomerAccount(registerArgs);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("http://localhost:8087/auth/customer/register");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      email: registerArgs.email,
      password: SECRET_PASSWORD,
    });
  });

  it("maps a 2xx identity response to the created outcome", async () => {
    stubFetch(200, { data: { uid: "u-new", email: registerArgs.email } });

    const outcome = await registerCustomerAccount(registerArgs);

    expect(outcome).toEqual({
      kind: "created",
      uid: "u-new",
      email: registerArgs.email,
    });
  });

  it.each([
    ["email_taken", 409],
    ["email_ambiguous", 409],
    ["weak_password", 400],
    ["verification_email_failed", 503],
    ["zitadel_unavailable", 503],
  ])(
    "maps %s (%i) to a distinct failed outcome, not a thrown error",
    async (code, status) => {
      stubFetch(status, { error: code });

      const outcome = await registerCustomerAccount(registerArgs);

      expect(outcome).toEqual({ kind: "failed", code });
    },
  );

  it("email_taken and weak_password stay distinguishable from each other", async () => {
    stubFetch(409, { error: "email_taken" });
    const taken = await registerCustomerAccount(registerArgs);

    stubFetch(400, { error: "weak_password" });
    const weak = await registerCustomerAccount(registerArgs);

    expect(taken).toEqual({ kind: "failed", code: "email_taken" });
    expect(weak).toEqual({ kind: "failed", code: "weak_password" });
    expect(taken).not.toEqual(weak);
  });

  it("throws (not a failed outcome) on a transport failure", async () => {
    const fn = vi.fn().mockRejectedValue(new Error("ECONNREFUSED"));
    vi.stubGlobal("fetch", fn);

    await expect(registerCustomerAccount(registerArgs)).rejects.toBeInstanceOf(
      AuthBffCustomerError,
    );
  });

  it("rejects a 2xx body missing/mistyped email as unrecognised, rather than passing `undefined` through as if it were a real address", async () => {
    // If auth-bff ever stopped sending `email`, the untyped field would
    // otherwise flow straight into a `email: string`-typed outcome — and
    // from there into signPendingSignup, which would happily sign the
    // literal string "undefined" (see app/create-account/actions.ts).
    stubFetch(200, { data: { uid: "u-new" } });

    const outcome = await registerCustomerAccount(registerArgs);

    expect(outcome).toEqual({ kind: "failed", code: "unrecognised_response_shape" });
  });

  it("sends X-Internal-Auth", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, {
      data: { uid: "u-new", email: registerArgs.email },
    });

    await registerCustomerAccount(registerArgs);

    const [, init] = fetchMock.mock.calls[0]!;
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
    vi.unstubAllEnvs();
  });
});

describe("verifyCustomerEmailCode", () => {
  const verifyArgs = { uid: "u-new", code: SECRET_TEST_CODE };

  it("sends the exact snake_case wire fields for /auth/customer/verify-email", async () => {
    const fetchMock = stubFetch(200, { data: { verified: true } });

    await verifyCustomerEmailCode(verifyArgs);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("http://localhost:8087/auth/customer/verify-email");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({ uid: "u-new", code: SECRET_TEST_CODE });
  });

  it("maps a 2xx response to the verified outcome", async () => {
    stubFetch(200, { data: { verified: true } });

    const outcome = await verifyCustomerEmailCode(verifyArgs);

    expect(outcome).toEqual({ kind: "verified" });
  });

  it.each([
    ["invalid_verification_code", 400],
    ["zitadel_unavailable", 503],
  ])("maps %s (%i) to a distinct failed outcome, not a thrown error", async (code, status) => {
    stubFetch(status, { error: code });

    const outcome = await verifyCustomerEmailCode(verifyArgs);

    expect(outcome).toEqual({ kind: "failed", code });
  });

  it("throws (not a failed outcome) on a transport failure", async () => {
    const fn = vi.fn().mockRejectedValue(new Error("ECONNREFUSED"));
    vi.stubGlobal("fetch", fn);

    await expect(verifyCustomerEmailCode(verifyArgs)).rejects.toBeInstanceOf(
      AuthBffCustomerError,
    );
  });

  it("never logs or otherwise surfaces the verification code", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const consoleLogSpy = vi.spyOn(console, "log").mockImplementation(() => {});
    stubFetch(400, { error: "invalid_verification_code" });

    const outcome = await verifyCustomerEmailCode(verifyArgs);

    expect(JSON.stringify(outcome)).not.toContain(SECRET_TEST_CODE);
    for (const call of [...consoleErrorSpy.mock.calls, ...consoleLogSpy.mock.calls]) {
      expect(JSON.stringify(call)).not.toContain(SECRET_TEST_CODE);
    }
    consoleErrorSpy.mockRestore();
    consoleLogSpy.mockRestore();
  });

  it("sends X-Internal-Auth", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, { data: { verified: true } });

    await verifyCustomerEmailCode(verifyArgs);

    const [, init] = fetchMock.mock.calls[0]!;
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
    vi.unstubAllEnvs();
  });
});
