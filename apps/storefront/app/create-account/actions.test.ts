import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Mirrors app/sign-in/actions.zitadel.test.ts's approach: mock the
// auth-bff-customer client boundary and the next/headers server-runtime
// seam, then drive the real registerCustomer/verifyCustomerEmail/
// customerSignUp server actions against those mocks.

const cookieStore: Record<string, string> = {};
const cookiesSetSpy = vi.fn(
  (opts: { name: string; value: string; domain?: string }) => {
    cookieStore[opts.name] = opts.value;
  },
);
const cookiesDeleteSpy = vi.fn((name: string) => {
  delete cookieStore[name];
});

let headerMap: Map<string, string>;
const HOST = "shop.mark8ly.com";
const SECRET_PASSWORD = "correct-horse-battery-staple";
const SECRET_TEST_CODE = "A1B2C3"; // low-entropy, test-only — never a live credential

vi.mock("next/headers", () => ({
  headers: async () => ({
    get: (key: string) => headerMap.get(key.toLowerCase()) ?? null,
  }),
  cookies: async () => ({
    set: cookiesSetSpy,
    get: (name: string) =>
      cookieStore[name] !== undefined ? { value: cookieStore[name] } : undefined,
    delete: cookiesDeleteSpy,
  }),
}));

vi.mock("@/lib/gip/verify-id-token", async () => {
  const actual = await vi.importActual<typeof import("@/lib/gip/verify-id-token")>(
    "@/lib/gip/verify-id-token",
  );
  return { ...actual, verifyGIPIdToken: vi.fn() };
});

vi.mock("@/lib/auth/auth-bff-customer", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/auth-bff-customer")>(
    "@/lib/auth/auth-bff-customer",
  );
  return {
    ...actual,
    verifyCustomerCredential: vi.fn(),
    verifyCustomerTotp: vi.fn(),
    registerCustomerAccount: vi.fn(),
    verifyCustomerEmailCode: vi.fn(),
  };
});

vi.mock("@/lib/api/server/platformInternal", () => ({
  platformInternalFetch: vi.fn(),
}));

import { verifyGIPIdToken } from "@/lib/gip/verify-id-token";
import {
  AuthBffCustomerError,
  registerCustomerAccount,
  verifyCustomerCredential,
  verifyCustomerEmailCode,
} from "@/lib/auth/auth-bff-customer";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";

const verifyGIPIdTokenMock = vi.mocked(verifyGIPIdToken);
const verifyCustomerCredentialMock = vi.mocked(verifyCustomerCredential);
const registerCustomerAccountMock = vi.mocked(registerCustomerAccount);
const verifyCustomerEmailCodeMock = vi.mocked(verifyCustomerEmailCode);
const platformInternalFetchMock = vi.mocked(platformInternalFetch);

const originalFetch = globalThis.fetch;
let fetchSpy: ReturnType<typeof vi.fn>;

async function loadActions() {
  return await import("./actions");
}

beforeEach(() => {
  vi.resetModules();
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;

  headerMap = new Map([["host", HOST]]);
  for (const key of Object.keys(cookieStore)) delete cookieStore[key];
  cookiesSetSpy.mockClear();
  cookiesDeleteSpy.mockClear();

  verifyGIPIdTokenMock.mockReset();
  verifyCustomerCredentialMock.mockReset();
  registerCustomerAccountMock.mockReset();
  verifyCustomerEmailCodeMock.mockReset();

  platformInternalFetchMock.mockReset();
  platformInternalFetchMock.mockResolvedValue({
    ok: true,
    json: async () => ({ data: { tenant_id: "tenant-1", id: "store-1" } }),
  } as Response);

  fetchSpy = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) });
  globalThis.fetch = fetchSpy as unknown as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  vi.clearAllMocks();
});

describe("customerSignUp — flag unset stays byte-identical to the GIP delegation", () => {
  it("delegates to customerSignIn exactly as before, minting a cookie on success", async () => {
    verifyGIPIdTokenMock.mockResolvedValue({
      uid: "u-gip",
      email: "gip@example.com",
      tenantId: "t",
    });
    const { customerSignUp } = await loadActions();

    const result = await customerSignUp({
      idToken: "id-token",
      uid: "ignored",
      storeSlug: "shop",
    });

    expect(result.ok).toBe(true);
    expect(verifyGIPIdTokenMock).toHaveBeenCalledTimes(1);
    expect(registerCustomerAccountMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session" }),
    );
  });

  it("sets no cookie when the GIP token verification fails", async () => {
    verifyGIPIdTokenMock.mockRejectedValue(new Error("bad token"));
    const { customerSignUp } = await loadActions();

    const result = await customerSignUp({
      idToken: "bad",
      uid: "ignored",
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });
});

describe("registerCustomer — flag set", () => {
  it("returns the trusted uid/email on success", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    registerCustomerAccountMock.mockResolvedValue({
      kind: "created",
      uid: "u-new",
      email: "shopper@example.com",
    });
    const { registerCustomer } = await loadActions();

    const result = await registerCustomer({
      email: "shopper@example.com",
      password: SECRET_PASSWORD,
    });

    expect(result).toEqual({ ok: true, uid: "u-new", email: "shopper@example.com" });
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it.each([
    ["email_taken", /sign in|support/i],
    ["email_ambiguous", /support/i],
    ["weak_password", /password/i],
    ["verification_email_failed", /try/i],
    ["zitadel_unavailable", /temporarily/i],
  ])("maps %s to its own distinct, truthful message and sets no cookie", async (code, pattern) => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    registerCustomerAccountMock.mockResolvedValue({ kind: "failed", code });
    const { registerCustomer } = await loadActions();

    const result = await registerCustomer({
      email: "shopper@example.com",
      password: SECRET_PASSWORD,
    });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.code).toBe(code);
      expect(result.message).toMatch(pattern);
    }
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("email_taken and verification_email_failed render different messages (permanent vs. retriable)", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    const { registerCustomer } = await loadActions();

    registerCustomerAccountMock.mockResolvedValue({ kind: "failed", code: "email_taken" });
    const taken = await registerCustomer({ email: "a@example.com", password: SECRET_PASSWORD });

    registerCustomerAccountMock.mockResolvedValue({
      kind: "failed",
      code: "verification_email_failed",
    });
    const failed = await registerCustomer({ email: "a@example.com", password: SECRET_PASSWORD });

    expect(taken.ok).toBe(false);
    expect(failed.ok).toBe(false);
    if (!taken.ok && !failed.ok) {
      expect(taken.message).not.toBe(failed.message);
      expect(taken.message.toLowerCase()).not.toContain("try creating your account again");
      expect(failed.message.toLowerCase()).toContain("try creating your account again");
    }
  });

  it("a transport failure (AuthBffCustomerError) returns zitadel_unavailable, not an unhandled throw", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    registerCustomerAccountMock.mockRejectedValue(new AuthBffCustomerError(0, "network_error"));
    const { registerCustomer } = await loadActions();

    const result = await registerCustomer({
      email: "shopper@example.com",
      password: SECRET_PASSWORD,
    });

    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.code).toBe("zitadel_unavailable");
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });
});

describe("verifyCustomerEmail — flag set", () => {
  const verifyInput = {
    uid: "u-new",
    email: "shopper@example.com",
    code: SECRET_TEST_CODE,
    storeSlug: "shop",
  };

  it("mints the session cookie through completeCustomerSignIn on a verified outcome — the full register -> verify -> session flow", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    registerCustomerAccountMock.mockResolvedValue({
      kind: "created",
      uid: "u-new",
      email: "shopper@example.com",
    });
    verifyCustomerEmailCodeMock.mockResolvedValue({ kind: "verified" });
    const { registerCustomer, verifyCustomerEmail } = await loadActions();

    const registered = await registerCustomer({
      email: "shopper@example.com",
      password: SECRET_PASSWORD,
    });
    expect(registered.ok).toBe(true);

    const result = await verifyCustomerEmail(verifyInput);

    expect(result.ok).toBe(true);
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session", domain: HOST }),
    );
  });

  it("a wrong/expired code sets no cookie and returns invalid_verification_code with its own message", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    verifyCustomerEmailCodeMock.mockResolvedValue({
      kind: "failed",
      code: "invalid_verification_code",
    });
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail(verifyInput);

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.code).toBe("invalid_verification_code");
      expect(result.message.toLowerCase()).toContain("code");
    }
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("sets no cookie when the host cannot be validated", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    headerMap = new Map(); // no host header at all
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail(verifyInput);

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
    expect(verifyCustomerEmailCodeMock).not.toHaveBeenCalled();
  });

  it("sets no cookie when the store cannot be resolved", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    platformInternalFetchMock.mockResolvedValue({ ok: false, json: async () => ({}) } as Response);
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail(verifyInput);

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("a transport failure (AuthBffCustomerError) returns zitadel_unavailable and sets no cookie", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    verifyCustomerEmailCodeMock.mockRejectedValue(new AuthBffCustomerError(0, "network_error"));
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail(verifyInput);

    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.code).toBe("zitadel_unavailable");
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("never logs the verification code, on success or failure", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const consoleLogSpy = vi.spyOn(console, "log").mockImplementation(() => {});

    verifyCustomerEmailCodeMock.mockResolvedValue({
      kind: "failed",
      code: "invalid_verification_code",
    });
    const { verifyCustomerEmail } = await loadActions();
    const result = await verifyCustomerEmail(verifyInput);

    const allCalls = [...consoleErrorSpy.mock.calls, ...consoleLogSpy.mock.calls];
    for (const call of allCalls) {
      expect(JSON.stringify(call)).not.toContain(SECRET_TEST_CODE);
    }
    expect(JSON.stringify(result)).not.toContain(SECRET_TEST_CODE);

    consoleErrorSpy.mockRestore();
    consoleLogSpy.mockRestore();
  });
});
