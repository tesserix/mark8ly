import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// "./actions" is dynamically re-imported per test after resetting the
// module registry, so module-evaluation-time reads cannot leak state
// between cases.

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
const SUBMITTED_PASSWORD = "correct-horse-battery-staple";

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

vi.mock("@/lib/auth/auth-bff-customer", async () => {
  const actual = await vi.importActual<
    typeof import("@/lib/auth/auth-bff-customer")
  >("@/lib/auth/auth-bff-customer");
  return { ...actual, verifyCustomerCredential: vi.fn(), verifyCustomerTotp: vi.fn() };
});

vi.mock("@/lib/api/server/platformInternal", () => ({
  platformInternalFetch: vi.fn(),
}));

import {
  AuthBffCustomerError,
  verifyCustomerCredential,
  verifyCustomerTotp,
} from "@/lib/auth/auth-bff-customer";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";
import { isTotpRequiredResult } from "@/lib/auth/customer-sign-in-result";

const verifyCustomerCredentialMock = vi.mocked(verifyCustomerCredential);
const verifyCustomerTotpMock = vi.mocked(verifyCustomerTotp);
const platformInternalFetchMock = vi.mocked(platformInternalFetch);

const originalFetch = globalThis.fetch;
let fetchSpy: ReturnType<typeof vi.fn>;

async function loadActions() {
  return await import("./actions");
}

/** Decodes the base64 payload half of an encodeSession cookie value
 *  (`<base64-payload>.<hex-signature>`) without needing the HMAC key. */
function decodeCookiePayload(cookieValue: string): Record<string, unknown> {
  const payload = cookieValue.slice(0, cookieValue.lastIndexOf("."));
  return JSON.parse(Buffer.from(payload, "base64").toString());
}

beforeEach(() => {
  vi.resetModules();
  headerMap = new Map([["host", HOST]]);
  for (const key of Object.keys(cookieStore)) delete cookieStore[key];
  cookiesSetSpy.mockClear();
  cookiesDeleteSpy.mockClear();

  verifyCustomerCredentialMock.mockReset();
  verifyCustomerTotpMock.mockReset();

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
  vi.clearAllMocks();
});

describe("customerSignIn — per-store cookie isolation (domain: cookieHost)", () => {
  it("Zitadel path: the session cookie's domain equals the resolved request host", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "complete",
      uid: "u2",
      email: "e2@example.com",
    });
    const { customerSignIn } = await loadActions();

    await customerSignIn({
      loginName: "e2@example.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session", domain: HOST }),
    );
  });
});

describe("customerSignIn — failed verification sets no cookie", () => {
  it("Zitadel: a rejected credential sets no cookie", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({ kind: "rejected" });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: "wrong-password",
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("Zitadel: a totp_required outcome (uncollected by this form) sets no cookie", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "totp_required",
      sessionId: "s1",
      sessionToken: "tok1",
    });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });
});

describe("customerSignIn — truthful messages for outcomes other than a wrong credential", () => {
  it("a wrong password still produces the credential message (the useful signal isn't flattened away)", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({ kind: "rejected" });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: "wrong-password",
      storeSlug: "shop",
    });

    expect(result).toEqual({
      ok: false,
      code: "invalid_credentials",
      message: "Email or password is incorrect.",
    });
  });

  it('an "email_not_verified" outcome does NOT say the password is incorrect, and points the shopper at create-account for a new code', async () => {
    // The password here was CORRECT — auth-bff only returns this outcome
    // after CreatePasswordSession already succeeded — so telling this
    // shopper "Email or password is incorrect" would be false, and no
    // amount of retrying the password fixes an unverified email.
    verifyCustomerCredentialMock.mockResolvedValue({ kind: "email_not_verified" });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(result).toEqual({
      ok: false,
      code: "email_not_verified",
      message:
        "This email address hasn't been verified yet. Go to Create account to get a new verification code.",
    });
  });

  it('a "totp_required" outcome does NOT say the password is incorrect, and has its own message', async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "totp_required",
      sessionId: "s1",
      sessionToken: "tok1",
    });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message).not.toBe("Email or password is incorrect.");
      expect(result.message.toLowerCase()).toContain("authenticator");
    }
  });

  it('a "handoff" outcome does NOT say the password is incorrect, does not surface the handoff URL, and has its own message', async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "handoff",
      handoffUrl: "https://zitadel.example/ui/v2/login/login",
    });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message).not.toBe("Email or password is incorrect.");
      expect(result.message).not.toContain("zitadel.example");
      expect(result.message.toLowerCase()).toContain("sign-in method");
    }
  });

  it("an AuthBffCustomerError produces a generic message with no internal detail", async () => {
    verifyCustomerCredentialMock.mockRejectedValue(
      new AuthBffCustomerError(503, "zitadel_unavailable"),
    );
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message).toBe(
        "Sign-in is temporarily unavailable. Please try again shortly.",
      );
      expect(result.message).not.toContain("503");
      expect(result.message).not.toContain("zitadel_unavailable");
      expect(result.message.toLowerCase()).not.toContain("auth-bff");
    }
  });
});

describe("customerSignIn — uid/email come from the verification result", () => {
  it("Zitadel: the session is built from verifyCustomerCredential's result, not client-supplied loginName", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "complete",
      uid: "trusted-zit-uid",
      email: "trusted-zit@example.com",
    });
    const { customerSignIn } = await loadActions();

    await customerSignIn({
      loginName: "attacker@evil.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    const setCall = cookiesSetSpy.mock.calls[0]![0] as { value: string };
    const decoded = decodeCookiePayload(setCall.value);
    expect(decoded.uid).toBe("trusted-zit-uid");
    expect(decoded.email).toBe("trusted-zit@example.com");
  });
});

describe("customerSignIn — profile and loyalty side effects", () => {
  it("fire on the Zitadel path", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "complete",
      uid: "u2",
      email: "e2@example.com",
    });
    const { customerSignIn } = await loadActions();

    await customerSignIn({
      loginName: "e2@example.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    const paths = fetchSpy.mock.calls.map((c) => String(c[0]));
    expect(paths.some((p) => p.includes("/account"))).toBe(true);
    expect(paths.some((p) => p.includes("/loyalty/enroll"))).toBe(true);
  });
});

describe("customerSignIn — password never leaks", () => {
  it("a thrown verification error never carries the submitted password", async () => {
    verifyCustomerCredentialMock.mockRejectedValue(
      new Error(
        "auth-bff customer endpoint error: zitadel_unavailable (status 503)",
      ),
    );
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(JSON.stringify(result)).not.toContain(SUBMITTED_PASSWORD);
  });

  it("a rejected outcome's result value never carries the submitted password", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({ kind: "rejected" });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(JSON.stringify(result)).not.toContain(SUBMITTED_PASSWORD);
  });
});

describe("customerSignIn — totp_required carries the data the code-entry step needs", () => {
  it("hands back sessionId/sessionToken alongside the message", async () => {
    verifyCustomerCredentialMock.mockResolvedValue({
      kind: "totp_required",
      sessionId: "s-abc",
      sessionToken: "tok-xyz",
    });
    const { customerSignIn } = await loadActions();

    const result = await customerSignIn({
      loginName: "e@x.com",
      password: SUBMITTED_PASSWORD,
      storeSlug: "shop",
    });

    expect(result.ok).toBe(false);
    if (isTotpRequiredResult(result)) {
      expect(result.sessionId).toBe("s-abc");
      expect(result.sessionToken).toBe("tok-xyz");
    } else {
      throw new Error("expected a totp_required result");
    }
  });
});

describe("confirmCustomerTotp — happy path", () => {
  it("a valid code completes: sets mp_customer_session and runs the same profile/loyalty side effects as the password path", async () => {
    verifyCustomerTotpMock.mockResolvedValue({
      kind: "complete",
      uid: "u-totp",
      email: "totp@example.com",
    });
    const { confirmCustomerTotp } = await loadActions();

    const result = await confirmCustomerTotp({
      storeSlug: "shop",
      sessionId: "s-1",
      sessionToken: "tok-1",
      code: "123456",
    });

    expect(result).toEqual({ ok: true });
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session", domain: HOST }),
    );

    const setCall = cookiesSetSpy.mock.calls[0]![0] as { value: string };
    const decoded = decodeCookiePayload(setCall.value);
    expect(decoded.uid).toBe("u-totp");
    expect(decoded.email).toBe("totp@example.com");

    const paths = fetchSpy.mock.calls.map((c) => String(c[0]));
    expect(paths.some((p) => p.includes("/account"))).toBe(true);
    expect(paths.some((p) => p.includes("/loyalty/enroll"))).toBe(true);
  });
});

describe("confirmCustomerTotp — invalid code", () => {
  it("returns a truthful, non-generic message and sets no cookie", async () => {
    verifyCustomerTotpMock.mockResolvedValue({ kind: "rejected" });
    const { confirmCustomerTotp } = await loadActions();

    const result = await confirmCustomerTotp({
      storeSlug: "shop",
      sessionId: "s-1",
      sessionToken: "tok-1",
      code: "000000",
    });

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
    if (!result.ok) {
      expect(result.message).not.toBe("Email or password is incorrect.");
      expect(result.message.toLowerCase()).toContain("code");
    }
  });
});

describe("confirmCustomerTotp — a repeat totp_required is a FRESH challenge, not a wrong code", () => {
  it("hands back the NEW sessionId/sessionToken, not the caller's original pair", async () => {
    verifyCustomerTotpMock.mockResolvedValue({
      kind: "totp_required",
      sessionId: "s-fresh",
      sessionToken: "tok-fresh",
    });
    const { confirmCustomerTotp } = await loadActions();

    const result = await confirmCustomerTotp({
      storeSlug: "shop",
      sessionId: "s-original",
      sessionToken: "tok-original",
      code: "123456",
    });

    expect(result.ok).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
    if (!isTotpRequiredResult(result)) {
      throw new Error("expected a totp_required result");
    }
    // Pin the credentials, not just the message: a bug that discards
    // outcome.sessionId/sessionToken and echoes the caller's original
    // pair back would still produce a truthful-sounding "enter a new
    // code" message and pass a message-only assertion.
    expect(result.sessionId).toBe("s-fresh");
    expect(result.sessionToken).toBe("tok-fresh");
    expect(result.sessionId).not.toBe("s-original");
    expect(result.sessionToken).not.toBe("tok-original");
    // And it must not be confused with an ordinary wrong code.
    expect(result.message).not.toBe("That code is incorrect. Please try again.");
  });
});

// Serializes a console.error argument for a leak check. Plain
// JSON.stringify is not enough here: Error's message/stack/name are
// non-enumerable, so JSON.stringify(new Error("987654")) is "{}" — a
// leaked code embedded in a logged Error would sail straight through a
// JSON.stringify-only check with the assertion still passing. This pulls
// name/message/stack (and a cause, if present) explicitly for Error
// instances, and falls back to JSON.stringify (including non-enumerable
// own properties, via Object.getOwnPropertyNames as the replacer) for
// everything else.
function serializeForLeakCheck(value: unknown): string {
  if (value instanceof Error) {
    const cause =
      "cause" in value ? ` cause=${serializeForLeakCheck(value.cause)}` : "";
    return `${value.name}: ${value.message}\n${value.stack ?? ""}${cause}`;
  }
  if (typeof value === "string") return value;
  if (value && typeof value === "object") {
    try {
      return JSON.stringify(value, Object.getOwnPropertyNames(value));
    } catch {
      return String(value);
    }
  }
  return String(value);
}

describe("confirmCustomerTotp — the code never leaks", () => {
  it("never appears in a console.error argument", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      verifyCustomerTotpMock.mockRejectedValue(
        new AuthBffCustomerError(503, "zitadel_unavailable"),
      );
      const { confirmCustomerTotp } = await loadActions();

      const SECRET_CODE = "987654";
      await confirmCustomerTotp({
        storeSlug: "shop",
        sessionId: "s-1",
        sessionToken: "tok-1",
        code: SECRET_CODE,
      });

      for (const call of consoleErrorSpy.mock.calls) {
        for (const arg of call) {
          expect(serializeForLeakCheck(arg)).not.toContain(SECRET_CODE);
        }
      }
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  // Proves the assertion above is actually capable of failing — a bare
  // JSON.stringify(new Error(...)) check would pass even with the code
  // embedded in a logged Error's message, since Error's own enumerable
  // properties are empty. This test intentionally logs the code the way
  // a regression might, and expects THIS test to fail if
  // serializeForLeakCheck stops inspecting message/stack.
  it("sanity check: serializeForLeakCheck actually catches a code embedded in a logged Error", () => {
    const SECRET_CODE = "555444";
    const leaky = new Error(`totp confirm failed for code ${SECRET_CODE}`);
    expect(serializeForLeakCheck(leaky)).toContain(SECRET_CODE);
    // And a bare JSON.stringify over the same Error would have missed it
    // entirely — this is the gap the sanity check above closes.
    expect(JSON.stringify(leaky)).not.toContain(SECRET_CODE);
  });
});

describe("confirmCustomerTotp — auth-bff failure never leaks internal detail", () => {
  it("an AuthBffCustomerError produces the generic 'temporarily unavailable' message, not the internal string", async () => {
    verifyCustomerTotpMock.mockRejectedValue(
      new AuthBffCustomerError(503, "zitadel_unavailable"),
    );
    const { confirmCustomerTotp } = await loadActions();

    const result = await confirmCustomerTotp({
      storeSlug: "shop",
      sessionId: "s-1",
      sessionToken: "tok-1",
      code: "123456",
    });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message).toBe(
        "Sign-in is temporarily unavailable. Please try again shortly.",
      );
      expect(result.message).not.toContain("503");
      expect(result.message).not.toContain("zitadel_unavailable");
      expect(result.message.toLowerCase()).not.toContain("auth-bff");
    }
  });
});
