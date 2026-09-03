import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthBffError, startZitadelIDPIntent, zitadelIdpFinish } from "./auth-bff";

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
  workspaceTenant: "tenant-1",
};

describe("zitadelIdpFinish", () => {
  it("sends the exact snake_case field names auth-bff expects", async () => {
    const fetchMock = stubFetch(200, {
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelIdpFinish(finishReq);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/auth/zitadel/idp/finish");
    expect(JSON.parse(init.body as string)).toEqual({
      auth_request_id: "ar-1",
      intent_id: "intent-1",
      intent_token: "intent-token-1",
      workspace_tenant: "tenant-1",
    });
  });

  it("never sends a `user` field — there is no parameter for it", async () => {
    const fetchMock = stubFetch(200, {
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelIdpFinish(finishReq);

    const [, init] = fetchMock.mock.calls[0];
    const body = JSON.parse(init.body as string);
    expect(body.user).toBeUndefined();
    expect(Object.keys(body)).not.toContain("user");
  });

  it("forwards User-Agent and X-Forwarded-For when supplied", async () => {
    const fetchMock = stubFetch(200, {
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelIdpFinish({ ...finishReq, userAgent: "Mozilla/5.0 test-agent", forwardedFor: "203.0.113.5" });

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["User-Agent"]).toBe("Mozilla/5.0 test-agent");
    expect(headers["X-Forwarded-For"]).toBe("203.0.113.5");
  });

  it("collects every Set-Cookie header on a completed sign-in", async () => {
    const res = new Response(
      JSON.stringify({
        callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
        data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
      }),
      {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    res.headers.append("set-cookie", "m8_session=abc; Path=/; HttpOnly");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res));

    const result = await zitadelIdpFinish(finishReq);

    expect(result.setCookies).toEqual(["m8_session=abc; Path=/; HttpOnly"]);
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
    const fetchMock = stubFetch(200, {
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelIdpFinish(finishReq);

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Internal-Auth"]).toBe("s3cret-internal");
  });
});
