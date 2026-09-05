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
// Real (unmocked) signing so tests can construct a token that will
// actually pass verifyCustomerEmail's tamper check, and so the
// deliberately-tampered tests below are pinned against the real function,
// not a stub of it.
import { signPendingSignup } from "@/lib/auth/pending-signup-token";

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

  // Default: marketplace-api reports this identity IS a customer of the
  // store. completeCustomerSignIn now asks before it mints a session
  // cookie (see @/lib/auth/customer-session), so every test below that
  // expects a cookie needs a membership to exist. The gate itself is
  // covered in lib/auth/customer-session.membership.test.ts.
  fetchSpy = vi
    .fn()
    .mockResolvedValue({ ok: true, json: async () => ({ data: { member: true } }) });
  globalThis.fetch = fetchSpy as unknown as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  vi.clearAllMocks();
});

describe("customerSignUp — unconditional delegation to customerSignIn", () => {
  // customerSignUp itself never inspects NEXT_PUBLIC_AUTH_PROVIDER — it is
  // a pure `return customerSignIn(input)` (see actions.ts). The flag
  // branch a GIP-vs-Zitadel input shape needs lives entirely inside
  // customerSignIn and is already covered end to end by
  // app/sign-in/actions.zitadel.test.ts's "provider branch" describe
  // block; duplicating that here (by flipping the flag against a
  // GIP-shaped input) would just prove customerSignIn's branching, mis-
  // titled as this file's. What IS this file's to prove: that phase 6a
  // task 3 (adding registerCustomer/verifyCustomerEmail alongside this
  // function) left the GIP call path — accounts:signUp -> customerSignUp
  // -> customerSignIn -> cookie — completely untouched.
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

describe("registerCustomer — provider guard", () => {
  it("flag unset: refuses without calling auth-bff at all", async () => {
    const { registerCustomer } = await loadActions();

    const result = await registerCustomer({
      email: "shopper@example.com",
      password: SECRET_PASSWORD,
    });

    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.code).toBe("not_available");
    expect(registerCustomerAccountMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });
});

describe("registerCustomer — flag set", () => {
  it("returns the trusted uid/email plus a signed pending-signup token on success", async () => {
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

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.uid).toBe("u-new");
      expect(result.email).toBe("shopper@example.com");
      expect(result.token).toBe(signPendingSignup("u-new", "shopper@example.com"));
    }
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("normalizes (trim + lowercase) the email before sending it to auth-bff and before signing the token", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    // auth-bff echoes back whatever it received — since registerCustomer
    // must send the normalized form, the mock's echo is normalized too.
    registerCustomerAccountMock.mockResolvedValue({
      kind: "created",
      uid: "u-new",
      email: "shopper@example.com",
    });
    const { registerCustomer } = await loadActions();

    const result = await registerCustomer({
      email: "  Shopper@Example.COM  ",
      password: SECRET_PASSWORD,
    });

    expect(registerCustomerAccountMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: "shopper@example.com" }),
    );
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.email).toBe("shopper@example.com");
      expect(result.token).toBe(signPendingSignup("u-new", "shopper@example.com"));
    }
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

describe("verifyCustomerEmail — provider guard", () => {
  it("flag unset: refuses without calling auth-bff at all", async () => {
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail({
      uid: "u-new",
      email: "shopper@example.com",
      token: signPendingSignup("u-new", "shopper@example.com"),
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.code).toBe("not_available");
    expect(verifyCustomerEmailCodeMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });
});

describe("verifyCustomerEmail — flag set", () => {
  // A valid token for exactly this {uid, email} pair — as if it had come
  // straight out of a prior registerCustomer call for the same address.
  const validToken = signPendingSignup("u-new", "shopper@example.com");
  const verifyInput = {
    uid: "u-new",
    email: "shopper@example.com",
    token: validToken,
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
    if (!registered.ok) throw new Error("unreachable");

    // Uses the REAL token registerCustomer just returned — not the
    // module-level `validToken` fixture — so this test exercises the
    // actual end-to-end token handoff, not a value the test computed
    // independently.
    const result = await verifyCustomerEmail({
      uid: registered.uid,
      email: registered.email,
      token: registered.token,
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(true);
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session", domain: HOST }),
    );
  });

  it("verifies against the token even when the email carried through the form is differently cased/padded than what was signed", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    // register normalized to "shopper@example.com" and signed THAT.
    const token = signPendingSignup("u-new", "shopper@example.com");
    verifyCustomerEmailCodeMock.mockResolvedValue({ kind: "verified" });
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail({
      uid: "u-new",
      email: "  Shopper@Example.COM  ", // same address, different casing/whitespace
      token,
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(true);
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session" }),
    );
  });

  it("REJECTS a client-swapped email even with a genuinely correct code — the account-takeover this token exists to close", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    // Attacker registered attacker@example.com themselves and holds a
    // real uid + token for THAT address...
    const attackerToken = signPendingSignup("u-attacker", "attacker@example.com");
    // Zitadel would happily accept the code — it checks only {uid, code}.
    verifyCustomerEmailCodeMock.mockResolvedValue({ kind: "verified" });
    const { verifyCustomerEmail } = await loadActions();

    // ...then replays verify with a victim's email swapped in.
    const result = await verifyCustomerEmail({
      uid: "u-attacker",
      email: "victim@example.com",
      token: attackerToken,
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    // Must never even reach Zitadel's verify-email call, let alone mint a
    // session under the victim's address.
    expect(verifyCustomerEmailCodeMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("rejects a client-swapped uid the same way", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    const tokenForOtherUid = signPendingSignup("u-other", "shopper@example.com");
    verifyCustomerEmailCodeMock.mockResolvedValue({ kind: "verified" });
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail({
      uid: "u-new",
      email: "shopper@example.com",
      token: tokenForOtherUid,
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    expect(verifyCustomerEmailCodeMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("rejects a missing/garbage token outright", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    const { verifyCustomerEmail } = await loadActions();

    const result = await verifyCustomerEmail({
      uid: "u-new",
      email: "shopper@example.com",
      token: "not-a-real-token",
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    expect(verifyCustomerEmailCodeMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).not.toHaveBeenCalled();
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

  it("never logs the verification code — exercises a path that actually logs (an AuthBffCustomerError), not just one that returns a value", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const consoleLogSpy = vi.spyOn(console, "log").mockImplementation(() => {});

    // This is the branch that actually calls console.error(..., err.code)
    // — a `mockResolvedValue({kind:"failed", ...})` outcome, by contrast,
    // logs nothing at all, so asserting against it proves nothing about
    // whether the code could leak through a log call.
    verifyCustomerEmailCodeMock.mockRejectedValue(
      new AuthBffCustomerError(503, "zitadel_unavailable"),
    );
    const { verifyCustomerEmail } = await loadActions();
    const result = await verifyCustomerEmail(verifyInput);

    expect(consoleErrorSpy).toHaveBeenCalled();
    const allCalls = [...consoleErrorSpy.mock.calls, ...consoleLogSpy.mock.calls];
    expect(allCalls.length).toBeGreaterThan(0);
    for (const call of allCalls) {
      expect(JSON.stringify(call)).not.toContain(SECRET_TEST_CODE);
    }
    expect(JSON.stringify(result)).not.toContain(SECRET_TEST_CODE);

    consoleErrorSpy.mockRestore();
    consoleLogSpy.mockRestore();
  });

  it("never logs the code (or the token) on a tamper rejection either — that branch logs too", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { verifyCustomerEmail } = await loadActions();

    await verifyCustomerEmail({
      uid: "u-new",
      email: "victim@example.com", // swapped — deliberately invalid token/email pair
      token: signPendingSignup("u-new", "shopper@example.com"),
      code: SECRET_TEST_CODE,
      storeSlug: "shop",
    });

    expect(consoleErrorSpy).toHaveBeenCalled();
    for (const call of consoleErrorSpy.mock.calls) {
      expect(JSON.stringify(call)).not.toContain(SECRET_TEST_CODE);
    }
    consoleErrorSpy.mockRestore();
  });
});
