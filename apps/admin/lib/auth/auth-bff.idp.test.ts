import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthBffError, startZitadelIDPIntent, zitadelIdpFinish, zitadelIdpComplete } from "./auth-bff";

function stubFetch(status: number, body: unknown, headers: Record<string, string> = {}) {
  const res = new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });
  const fn = vi.fn().mockResolvedValue(res);
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe("startZitadelIDPIntent", () => {
  it("sends {return_url} to /auth/zitadel/idp/start and returns auth_url", async () => {
    const fetchMock = stubFetch(200, { auth_url: "https://zitadel.example/idp/authorize" });

    const url = await startZitadelIDPIntent("https://admin.mark8ly.com/auth/idp/finish?auth_request_id=ar-1");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [reqUrl, init] = fetchMock.mock.calls[0];
    expect(reqUrl).toContain("/auth/zitadel/idp/start");
    expect(JSON.parse(init.body as string)).toEqual({
      return_url: "https://admin.mark8ly.com/auth/idp/finish?auth_request_id=ar-1",
    });
    expect(url).toBe("https://zitadel.example/idp/authorize");
  });

  it("sends X-Internal-Auth from server config, never a NEXT_PUBLIC_ variable", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, { auth_url: "https://zitadel.example/idp/authorize" });

    await startZitadelIDPIntent("https://admin.mark8ly.com/auth/idp/finish");

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
  });

  it("throws AuthBffError on a non-2xx response", async () => {
    stubFetch(400, { error: "invalid_return_url" });

    await expect(startZitadelIDPIntent("https://evil.example/finish")).rejects.toMatchObject({
      status: 400,
      code: "invalid_return_url",
    });
  });

  it("throws when a 2xx body carries no auth_url", async () => {
    stubFetch(200, {});

    await expect(startZitadelIDPIntent("https://admin.mark8ly.com/auth/idp/finish")).rejects.toBeInstanceOf(
      AuthBffError,
    );
  });
});

const finishReq = {
  authRequestId: "ar-1",
  intentId: "intent-1",
  intentToken: "intent-token-1",
};

// This is the EXACT body auth-bff's idpFinish sends when no workspace_tenant
// is supplied — services/auth-bff/internal/zitadellogin/handler.go's idpFinish:
// `writeJSON(w, 200, map[string]any{"tenant_required": true, "session_id":
// sess.ID, "session_token": sess.Token, "login_name": identity.Email})`. This
// is the ONLY shape idpFinish answers with on success — it never completes a
// login itself, unlike zitadelLogin/zitadelTotp.
const TENANT_REQUIRED_BODY = {
  tenant_required: true,
  session_id: "sess-1",
  session_token: "sess-token-1",
  login_name: "merchant@example.com",
};

describe("zitadelIdpFinish", () => {
  it("sends the exact snake_case field names auth-bff expects — WITHOUT workspace_tenant", async () => {
    const fetchMock = stubFetch(200, TENANT_REQUIRED_BODY);

    await zitadelIdpFinish(finishReq);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/auth/zitadel/idp/finish");
    expect(JSON.parse(init.body as string)).toEqual({
      auth_request_id: "ar-1",
      intent_id: "intent-1",
      intent_token: "intent-token-1",
    });
  });

  it("never sends a `user` field or a `workspace_tenant` field — there is no parameter for either", async () => {
    const fetchMock = stubFetch(200, TENANT_REQUIRED_BODY);

    await zitadelIdpFinish(finishReq);

    const [, init] = fetchMock.mock.calls[0];
    const body = JSON.parse(init.body as string);
    expect(body.user).toBeUndefined();
    expect(body.workspace_tenant).toBeUndefined();
    expect(Object.keys(body)).not.toContain("user");
    expect(Object.keys(body)).not.toContain("workspace_tenant");
  });

  it("forwards User-Agent and X-Forwarded-For when supplied", async () => {
    const fetchMock = stubFetch(200, TENANT_REQUIRED_BODY);

    await zitadelIdpFinish({ ...finishReq, userAgent: "Mozilla/5.0 test-agent", forwardedFor: "203.0.113.5" });

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["User-Agent"]).toBe("Mozilla/5.0 test-agent");
    expect(headers["X-Forwarded-For"]).toBe("203.0.113.5");
  });

  it("returns the session id/token and login_name from the tenant_required response", async () => {
    stubFetch(200, TENANT_REQUIRED_BODY);

    const result = await zitadelIdpFinish(finishReq);

    expect(result).toEqual({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
  });

  it("throws when a 2xx response is not shaped as tenant_required", async () => {
    // A stray success body that doesn't carry tenant_required — must never
    // be silently treated as anything else. There is no code path in
    // auth-bff that returns a completed login from THIS call, so this
    // function has no business accepting one.
    stubFetch(200, { callback_url: "https://admin.mark8ly.com/auth/callback?code=c" });

    await expect(zitadelIdpFinish(finishReq)).rejects.toMatchObject({
      code: "unrecognised_response_shape",
    });
  });

  it.each([
    ["no_admin_account", 403],
    ["unexpected_idp", 401],
    ["email_not_verified", 401],
    ["email_ambiguous", 409],
    ["invalid_intent", 401],
    ["zitadel_unavailable", 503],
  ])("preserves the %s outcome code on a %i response", async (code, status) => {
    stubFetch(status, { error: code });

    await expect(zitadelIdpFinish(finishReq)).rejects.toMatchObject({ status, code });
  });

  it("sends X-Internal-Auth from server config", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, TENANT_REQUIRED_BODY);

    await zitadelIdpFinish(finishReq);

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
  });
});

const completeReq = {
  authRequestId: "ar-1",
  loginName: "merchant@example.com",
  sessionId: "sess-1",
  sessionToken: "sess-token-1",
  workspaceTenant: "tenant-1",
};

// This is the EXACT body auth-bff sends on a genuine completed Zitadel
// sign-in (password, Google idp/finish, or idp/complete) —
// services/auth-bff/internal/zitadellogin/handler.go's finishComplete:
// `writeJSON(w, 200, map[string]any{"callback_url": res.CallbackURL})`,
// pinned server-side by handler_test.go's
// TestLoginCompleteCallsCompleteAndReturnsCallbackURL. No `data`, no `uid`,
// no `tenant_id` — see login-response.ts's file header for why
// parseLoginResponse must accept this shape as "complete" rather than
// throwing on it.
const REAL_COMPLETE_BODY = {
  callback_url: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
};

describe("zitadelIdpComplete", () => {
  it("sends the exact snake_case field names auth-bff expects", async () => {
    const fetchMock = stubFetch(200, REAL_COMPLETE_BODY);

    await zitadelIdpComplete(completeReq);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/auth/zitadel/idp/complete");
    expect(JSON.parse(init.body as string)).toEqual({
      auth_request_id: "ar-1",
      login_name: "merchant@example.com",
      session_id: "sess-1",
      session_token: "sess-token-1",
      workspace_tenant: "tenant-1",
    });
  });

  it("forwards User-Agent and X-Forwarded-For when supplied", async () => {
    const fetchMock = stubFetch(200, REAL_COMPLETE_BODY);

    await zitadelIdpComplete({ ...completeReq, userAgent: "Mozilla/5.0 test-agent", forwardedFor: "203.0.113.5" });

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["User-Agent"]).toBe("Mozilla/5.0 test-agent");
    expect(headers["X-Forwarded-For"]).toBe("203.0.113.5");
  });

  it("parses the REAL auth-bff wire shape (callback_url alone) as a complete outcome", async () => {
    stubFetch(200, REAL_COMPLETE_BODY);

    const result = await zitadelIdpComplete(completeReq);

    expect(result).toMatchObject({
      kind: "complete",
      callbackUrl: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
    });
  });

  it("collects every Set-Cookie header on a completed sign-in", async () => {
    const res = new Response(JSON.stringify(REAL_COMPLETE_BODY), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    res.headers.append("set-cookie", "m8_session=abc; Path=/; HttpOnly");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res));

    const result = await zitadelIdpComplete(completeReq);

    expect(result.setCookies).toEqual(["m8_session=abc; Path=/; HttpOnly"]);
  });

  it("recognises a step-up outcome (e.g. auth-bff's own mfa gate) with no session cookie minted", async () => {
    stubFetch(200, { mfa_required: true });

    const result = await zitadelIdpComplete(completeReq);

    expect(result.kind).toBe("mfa_required");
    expect(result.setCookies).toEqual([]);
  });

  it.each([
    ["invalid_request", 400],
    ["zitadel_unavailable", 503],
    ["internal_error", 500],
  ])("preserves the %s outcome code on a %i response", async (code, status) => {
    stubFetch(status, { error: code });

    await expect(zitadelIdpComplete(completeReq)).rejects.toMatchObject({ status, code });
  });

  it("sends X-Internal-Auth from server config", async () => {
    vi.stubEnv("MARKETPLACE_INTERNAL_AUTH_SECRET", "s3cret-internal");
    const fetchMock = stubFetch(200, REAL_COMPLETE_BODY);

    await zitadelIdpComplete(completeReq);

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
  });
});
