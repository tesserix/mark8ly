import { afterEach, describe, expect, it, vi } from "vitest";

import { autoLogin } from "./auth-bff";

function stubFetch(body: unknown) {
  const res = new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const req = {
  idToken: "id-token",
  expectedTenantId: "MP-Internal-e986p",
  workspaceTenant: "tenant-1",
};

describe("autoLogin", () => {
  // Regression: auth-bff returns email_otp_required alongside
  // mfa_required, and dropping it made a new-device sign-in look like a
  // completed one. The caller then redirected on a PENDING cookie and
  // middleware bounced the user back to /login showing no error at all.
  it("surfaces email_otp_required as emailOtpRequired", async () => {
    stubFetch({
      data: {
        uid: "u1",
        email: "merchant@example.com",
        tenant_id: "tenant-1",
        email_otp_required: true,
      },
    });

    const result = await autoLogin(req);

    expect(result.emailOtpRequired).toBe(true);
    expect(result.mfaRequired).toBe(false);
  });

  it("surfaces mfa_required independently", async () => {
    stubFetch({
      data: {
        uid: "u1",
        email: "merchant@example.com",
        tenant_id: "tenant-1",
        mfa_required: true,
      },
    });

    const result = await autoLogin(req);

    expect(result.mfaRequired).toBe(true);
    expect(result.emailOtpRequired).toBe(false);
  });

  it("treats an unchallenged sign-in as neither", async () => {
    stubFetch({
      data: { uid: "u1", email: "merchant@example.com", tenant_id: "tenant-1" },
    });

    const result = await autoLogin(req);

    expect(result.mfaRequired).toBe(false);
    expect(result.emailOtpRequired).toBe(false);
  });
});
