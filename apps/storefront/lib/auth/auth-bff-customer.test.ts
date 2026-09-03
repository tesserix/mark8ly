import { afterEach, describe, expect, it, vi } from "vitest";

import {
  AuthBffCustomerError,
  verifyCustomerCredential,
  verifyCustomerTotp,
} from "./auth-bff-customer";

const SECRET_PASSWORD = "correct-horse-battery-staple";

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
    authRequestId: "ar-1",
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
      auth_request_id: "ar-1",
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
      handoff_url: "https://zitadel.example.com/ui/v2/login/login?authRequestID=ar-1",
      auth_request_id: "ar-1",
    });

    const outcome = await verifyCustomerCredential(loginArgs);

    expect(outcome).toEqual({
      kind: "handoff",
      handoffUrl: "https://zitadel.example.com/ui/v2/login/login?authRequestID=ar-1",
      authRequestId: "ar-1",
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
    authRequestId: "ar-1",
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
      auth_request_id: "ar-1",
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
