import { createZitadelAuthClient } from "@repo/mobile-shared/auth/zitadel-client";

function jsonResponse(status: number, body: unknown) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response);
}

type FetchMock = jest.Mock<Promise<Response>, [string, RequestInit]>;

function clientWith(fetchImpl: FetchMock) {
  global.fetch = fetchImpl as unknown as typeof fetch;
  return createZitadelAuthClient({ baseUrl: "https://api.mark8ly.com" });
}

it("signs in and reports the tokens", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
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
  expect(init.method).toBe("POST");
  // No bearer: this call is how one is obtained.
  expect(init.headers).not.toHaveProperty("Authorization");
});

// THE common first-login path: a fresh install is always a new device.
it("reports an OTP challenge with the token needed to resume it", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, {
      data: {
        uid: "u1", email: "a@b.test", tenant_id: "t-1",
        email_otp_required: true, mfa_required: false, totp_required: false,
        pending_token: "sealed-value",
      },
    }),
  );
  const res = await clientWith(f).signIn("a@b.test", "pw");

  expect(res.status).toBe("step_up_required");
  if (res.status !== "step_up_required") throw new Error("unreachable");
  expect(res.challenge).toBe("email_otp");
  expect(res.pendingToken).toBe("sealed-value");
  expect(res.email).toBe("a@b.test");
});

// #686 item 2. The TOTP gate answers OUTSIDE the data envelope and carries
// its own sealed token; it must surface as a step-up the caller can tell
// APART from an emailed one, since the screens and the copy differ.
it("reports a TOTP challenge distinctly, with the token needed to resume it", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, {
      data: {
        tenant_id: "t-1",
        email_otp_required: false, mfa_required: false, totp_required: true,
        pending_token: "sealed-value",
      },
    }),
  );
  const res = await clientWith(f).signIn("a@b.test", "pw");

  expect(res.status).toBe("step_up_required");
  if (res.status !== "step_up_required") throw new Error("unreachable");
  expect(res.challenge).toBe("totp");
  expect(res.pendingToken).toBe("sealed-value");
});

// The regression this branch exists for: a TOTP enrolment used to produce
// `challenge_unresumable`, which the login screen renders as "this app
// version needs an update" — advice no update could ever satisfy.
it("does not report challenge_unresumable for a TOTP step-up", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, { data: { totp_required: true, pending_token: "sealed-value" } }),
  );
  await expect(clientWith(f).signIn("a@b.test", "pw")).resolves.toMatchObject({
    status: "step_up_required",
    challenge: "totp",
  });
});

it("verifies a TOTP code on its own route and returns tokens", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, {
      data: {
        uid: "u1", email: "a@b.test", tenant_id: "t-1",
        access_token: "AT", refresh_token: "RT", token_type: "Bearer", expires_in: 3599,
      },
    }),
  );
  const res = await clientWith(f).verifyTotp("sealed-value", "123456");

  expect(res.tokens.accessToken).toBe("AT");
  const [url, init] = f.mock.calls[0];
  // NOT the OTP route: the server checks this code against a Zitadel
  // session, not an emailed value.
  expect(url).toBe("https://api.mark8ly.com/api/v1/mobile/admin/auth/totp/verify");
  expect(JSON.parse(init.body as string)).toEqual({
    pending_token: "sealed-value",
    code: "123456",
  });
});

// A wrong TOTP code must keep its own code so the screen can show
// authenticator copy rather than password copy.
it("maps a rejected TOTP code to invalid_totp", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(401, { error: "invalid_totp" }),
  );
  await expect(clientWith(f).verifyTotp("sealed-value", "000000")).rejects.toMatchObject({
    code: "invalid_totp",
  });
});

// A challenge with no pending_token cannot be completed. Treating it as a
// normal OTP prompt would strand the user on a screen whose submit can
// never succeed, so it fails loudly instead.
it("rejects an OTP challenge that carries no pending token", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, { data: { email_otp_required: true, email: "a@b.test" } }),
  );
  // Asserted on the stable code, not the copy: screens map from the code,
  // and pinning wording makes the test fail on a copy edit.
  await expect(clientWith(f).signIn("a@b.test", "pw")).rejects.toMatchObject({
    code: "challenge_unresumable",
  });
});

it("maps 401 to invalid credentials", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) => jsonResponse(401, { error: "invalid_credentials" }));
  await expect(clientWith(f).signIn("a@b.test", "bad")).rejects.toMatchObject({
    code: "invalid_credentials",
  });
});

// 404 no_store is a distinct, actionable state — the account exists but has
// no store — and must not be flattened into "wrong password".
it("maps 404 no_store distinctly", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) => jsonResponse(404, { error: "no_store" }));
  await expect(clientWith(f).signIn("a@b.test", "pw")).rejects.toMatchObject({
    code: "no_store",
  });
});

// A 502 means the credential may have been correct. Reporting it as a bad
// password makes a merchant retype a correct one indefinitely.
it("maps 502 to an availability error, not a credential error", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) => jsonResponse(502, { error: "auth_unavailable" }));
  await expect(clientWith(f).signIn("a@b.test", "pw")).rejects.toMatchObject({
    code: "auth_unavailable",
  });
});

it("completes the OTP challenge and returns tokens", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
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
  expect(JSON.parse(init.body as string)).toEqual({
    pending_token: "sealed-value",
    code: "123456",
  });
});

it("maps a rejected OTP code to invalid_code", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) => jsonResponse(401, { error: "invalid_code" }));
  await expect(clientWith(f).verifyOtp("sealed", "000000")).rejects.toMatchObject({
    code: "invalid_code",
  });
});

// #686 item 3. The RETURN VALUE is the point: the server re-seals the
// challenge because the emailed code and the pending token expire
// together, so a caller keeping its original token would submit the stale
// half of the pair and be told a correct code was wrong.
it("resends the code and returns the FRESH pending token", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, { data: { sent: true, pending_token: "resealed-2" } }),
  );
  const fresh = await clientWith(f).resendOtp("original");

  expect(fresh).toBe("resealed-2");

  const [url, init] = f.mock.calls[0];
  expect(url).toBe("https://api.mark8ly.com/api/v1/mobile/admin/auth/otp/resend");
  // No address on the wire: the server reads it from the sealed token, and
  // offering one would make this a way to mail a code anywhere.
  expect(JSON.parse(init.body as string)).toEqual({ pending_token: "original" });
});

// A spent code budget must keep its own code all the way to the screen —
// "wait a few minutes" is the only advice that works here.
it("maps a spent code budget to rate_limited", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(429, { error: "rate_limited", message: "too many" }),
  );
  await expect(clientWith(f).resendOtp("original")).rejects.toMatchObject({
    code: "rate_limited",
  });
});

// A 200 with no fresh token would leave the caller verifying against the
// old challenge under a brand new code. Fail loudly instead.
it("refuses a resend that returns no fresh pending token", async () => {
  const f: FetchMock = jest.fn((_u: string, _i: RequestInit) =>
    jsonResponse(200, { data: { sent: true } }),
  );
  await expect(clientWith(f).resendOtp("original")).rejects.toMatchObject({
    code: "challenge_unresumable",
  });
});
