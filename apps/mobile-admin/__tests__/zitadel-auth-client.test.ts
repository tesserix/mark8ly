import { createZitadelAuthClient } from "@repo/mobile-shared/auth/zitadel-client";

function jsonResponse(status: number, body: unknown) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response);
}

function clientWith(fetchImpl: jest.Mock) {
  global.fetch = fetchImpl as unknown as typeof fetch;
  return createZitadelAuthClient({ baseUrl: "https://api.mark8ly.com" });
}

it("signs in and reports the tokens", async () => {
  const f = jest.fn(() =>
    jsonResponse(200, {
      data: {
        uid: "u1", email: "a@b.test", tenant_id: "t-1",
        tenants: [{ tenant_id: "t-1", name: "Mumbai Spice Co", role: "owner" }],
        access_token: "AT", refresh_token: "RT", token_type: "Bearer", expires_in: 3599,
        email_otp_required: false, mfa_required: false, totp_required: false,
      },
    }),
  );
  const res = await clientWith(f).signIn("a@b.test", "pw");

  expect(res.status).toBe("complete");
  if (res.status !== "complete") throw new Error("unreachable");
  expect(res.tokens.accessToken).toBe("AT");
  expect(res.tenantId).toBe("t-1");
  expect(res.tenants).toHaveLength(1);

  const [url, init] = f.mock.calls[0];
  expect(url).toBe("https://api.mark8ly.com/api/v1/mobile/admin/auth/login");
  expect((init as RequestInit).method).toBe("POST");
  // No bearer: this call is how one is obtained.
  expect((init as RequestInit).headers).not.toHaveProperty("Authorization");
});

// THE common first-login path: a fresh install is always a new device.
it("reports an OTP challenge with the token needed to resume it", async () => {
  const f = jest.fn(() =>
    jsonResponse(200, {
      data: {
        uid: "u1", email: "a@b.test", tenant_id: "t-1",
        email_otp_required: true, mfa_required: false, totp_required: false,
        pending_token: "sealed-value",
      },
    }),
  );
  const res = await clientWith(f).signIn("a@b.test", "pw");

  expect(res.status).toBe("otp_required");
  if (res.status !== "otp_required") throw new Error("unreachable");
  expect(res.pendingToken).toBe("sealed-value");
  expect(res.email).toBe("a@b.test");
});

// A challenge with no pending_token cannot be completed. Treating it as a
// normal OTP prompt would strand the user on a screen whose submit can
// never succeed, so it fails loudly instead.
it("rejects an OTP challenge that carries no pending token", async () => {
  const f = jest.fn(() =>
    jsonResponse(200, { data: { email_otp_required: true, email: "a@b.test" } }),
  );
  // Asserted on the stable code, not the copy: screens map from the code,
  // and pinning wording makes the test fail on a copy edit.
  await expect(clientWith(f).signIn("a@b.test", "pw")).rejects.toMatchObject({
    code: "challenge_unresumable",
  });
});

it("maps 401 to invalid credentials", async () => {
  const f = jest.fn(() => jsonResponse(401, { error: "invalid_credentials" }));
  await expect(clientWith(f).signIn("a@b.test", "bad")).rejects.toMatchObject({
    code: "invalid_credentials",
  });
});

// 404 no_store is a distinct, actionable state — the account exists but has
// no store — and must not be flattened into "wrong password".
it("maps 404 no_store distinctly", async () => {
  const f = jest.fn(() => jsonResponse(404, { error: "no_store" }));
  await expect(clientWith(f).signIn("a@b.test", "pw")).rejects.toMatchObject({
    code: "no_store",
  });
});

// A 502 means the credential may have been correct. Reporting it as a bad
// password makes a merchant retype a correct one indefinitely.
it("maps 502 to an availability error, not a credential error", async () => {
  const f = jest.fn(() => jsonResponse(502, { error: "auth_unavailable" }));
  await expect(clientWith(f).signIn("a@b.test", "pw")).rejects.toMatchObject({
    code: "auth_unavailable",
  });
});

it("completes the OTP challenge and returns tokens", async () => {
  const f = jest.fn(() =>
    jsonResponse(200, {
      data: {
        uid: "u1", email: "a@b.test", tenant_id: "t-1",
        access_token: "AT2", refresh_token: "RT2", token_type: "Bearer", expires_in: 3599,
      },
    }),
  );
  const res = await clientWith(f).verifyOtp("sealed-value", "123456");

  expect(res.tokens.accessToken).toBe("AT2");
  expect(res.tenantId).toBe("t-1");

  const [url, init] = f.mock.calls[0];
  expect(url).toBe("https://api.mark8ly.com/api/v1/mobile/admin/auth/otp/verify");
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({
    pending_token: "sealed-value",
    code: "123456",
  });
});

it("maps a rejected OTP code to invalid_code", async () => {
  const f = jest.fn(() => jsonResponse(401, { error: "invalid_code" }));
  await expect(clientWith(f).verifyOtp("sealed", "000000")).rejects.toMatchObject({
    code: "invalid_code",
  });
});
