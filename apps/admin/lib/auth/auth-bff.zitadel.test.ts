import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthBffError, zitadelLogin, zitadelTotp } from "./auth-bff";

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
});

const loginReq = {
  authRequestId: "ar-1",
  loginName: "merchant@example.com",
  password: "super-secret-pw",
  workspaceTenant: "tenant-1",
};

const totpReq = {
  authRequestId: "ar-1",
  sessionId: "sess-1",
  sessionToken: "sess-token-1",
  code: "123456",
  workspaceTenant: "tenant-1",
};

describe("zitadelLogin", () => {
  it("sends the exact snake_case field names auth-bff expects", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelLogin(loginReq);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/auth/zitadel/login");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      auth_request_id: "ar-1",
      login_name: "merchant@example.com",
      password: "super-secret-pw",
      workspace_tenant: "tenant-1",
    });
  });

  it("forwards User-Agent and X-Forwarded-For when supplied", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelLogin({
      ...loginReq,
      userAgent: "Mozilla/5.0 test-agent",
      forwardedFor: "203.0.113.5",
    });

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["User-Agent"]).toBe("Mozilla/5.0 test-agent");
    expect(headers["X-Forwarded-For"]).toBe("203.0.113.5");
  });

  it("does not set User-Agent / X-Forwarded-For when not supplied", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelLogin(loginReq);

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["User-Agent"]).toBeUndefined();
    expect(headers["X-Forwarded-For"]).toBeUndefined();
  });

  it("hands a 2xx body to parseLoginResponse and returns its outcome", async () => {
    stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1", mfa_required: true },
    });

    const result = await zitadelLogin(loginReq);

    expect(result.kind).toBe("mfa_required");
  });

  it("returns a totp_required outcome from parseLoginResponse", async () => {
    stubFetch(200, { totp_required: true, session_id: "s1", session_token: "tok" });

    const result = await zitadelLogin(loginReq);

    expect(result).toMatchObject({ kind: "totp_required", sessionId: "s1", sessionToken: "tok" });
  });

  it("collects every Set-Cookie header", async () => {
    stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });
    // Response's getSetCookie() only reflects headers actually set via the
    // headers init, and jsdom/undici Response supports multiple Set-Cookie
    // values appended individually.
    const res = new Response(
      JSON.stringify({ data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" } }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
    res.headers.append("set-cookie", "m8_session=abc; Path=/; HttpOnly");
    res.headers.append("set-cookie", "m8_mfa_pending=; Path=/; Max-Age=0");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res));

    const result = await zitadelLogin(loginReq);

    expect(result.setCookies).toEqual([
      "m8_session=abc; Path=/; HttpOnly",
      "m8_mfa_pending=; Path=/; Max-Age=0",
    ]);
  });

  it("raises AuthBffError with the flat error/message body on a 401", async () => {
    stubFetch(401, { error: "invalid_credentials", message: "Wrong login name or password" });

    await expect(zitadelLogin(loginReq)).rejects.toMatchObject({
      status: 401,
      code: "invalid_credentials",
      message: "Wrong login name or password",
    });
  });

  it("never leaks the submitted password into a thrown error", async () => {
    stubFetch(401, { error: "invalid_credentials", message: "Wrong login name or password" });

    let caught: unknown;
    try {
      await zitadelLogin(loginReq);
    } catch (err) {
      caught = err;
    }

    expect(caught).toBeInstanceOf(AuthBffError);
    const err = caught as AuthBffError;
    expect(err.message).not.toContain(loginReq.password);
    expect(err.code).not.toContain(loginReq.password);
    expect(JSON.stringify(err)).not.toContain(loginReq.password);
    expect(String(err)).not.toContain(loginReq.password);
  });
});

describe("zitadelTotp", () => {
  it("sends the exact snake_case field names auth-bff expects", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelTotp(totpReq);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/auth/zitadel/totp");
    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody).toEqual({
      auth_request_id: "ar-1",
      session_id: "sess-1",
      session_token: "sess-token-1",
      code: "123456",
      workspace_tenant: "tenant-1",
    });
  });

  it("forwards User-Agent and X-Forwarded-For when supplied", async () => {
    const fetchMock = stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    await zitadelTotp({
      ...totpReq,
      userAgent: "Mozilla/5.0 test-agent",
      forwardedFor: "203.0.113.5",
    });

    const [, init] = fetchMock.mock.calls[0];
    const headers = init.headers as Record<string, string>;
    expect(headers["User-Agent"]).toBe("Mozilla/5.0 test-agent");
    expect(headers["X-Forwarded-For"]).toBe("203.0.113.5");
  });

  it("hands a 2xx body to parseLoginResponse and returns its outcome", async () => {
    stubFetch(200, {
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    const result = await zitadelTotp(totpReq);

    expect(result).toMatchObject({ kind: "complete", uid: "u1", tenantId: "tenant-1" });
  });

  it("collects every Set-Cookie header", async () => {
    const res = new Response(
      JSON.stringify({ data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" } }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
    res.headers.append("set-cookie", "m8_session=xyz; Path=/; HttpOnly");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res));

    const result = await zitadelTotp(totpReq);

    expect(result.setCookies).toEqual(["m8_session=xyz; Path=/; HttpOnly"]);
  });

  it("raises AuthBffError with the flat error/message body on a 401", async () => {
    stubFetch(401, { error: "invalid_code", message: "Incorrect verification code" });

    await expect(zitadelTotp(totpReq)).rejects.toMatchObject({
      status: 401,
      code: "invalid_code",
      message: "Incorrect verification code",
    });
  });

  it("never leaks the submitted password into a thrown error (totp request carries no password, but must not echo any secret field)", async () => {
    stubFetch(401, { error: "invalid_code", message: "Incorrect verification code" });

    let caught: unknown;
    try {
      await zitadelTotp(totpReq);
    } catch (err) {
      caught = err;
    }

    expect(caught).toBeInstanceOf(AuthBffError);
    const err = caught as AuthBffError;
    expect(err.message).not.toContain(loginReq.password);
    expect(JSON.stringify(err)).not.toContain(loginReq.password);
  });
});
