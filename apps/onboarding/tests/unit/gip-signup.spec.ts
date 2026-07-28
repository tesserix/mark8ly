import { test, expect } from "@playwright/test";

// publicConfig freezes NEXT_PUBLIC_GIP_* at module-load time; the stand-in
// values are set in playwright.unit.config.ts, which every worker evaluates
// before it loads this file.
import { signUpWithName, signInWithGoogle } from "../../lib/gip/signup";

type RecordedCall = { url: string; body: Record<string, unknown> };

interface StubbedResponse {
  ok: boolean;
  status?: number;
  json?: unknown;
}

/**
 * Replace global fetch with a queue of canned responses, recording every
 * request. Returns the recording array.
 */
function stubFetch(responses: StubbedResponse[]): RecordedCall[] {
  const calls: RecordedCall[] = [];
  let i = 0;

  globalThis.fetch = (async (url: string, init: RequestInit) => {
    const next = responses[i++];
    if (!next) throw new Error(`unexpected fetch call #${i}: ${url}`);
    calls.push({
      url: String(url),
      body: JSON.parse(String(init.body ?? "{}")) as Record<string, unknown>,
    });
    return {
      ok: next.ok,
      status: next.status ?? (next.ok ? 200 : 500),
      json: async () => next.json ?? {},
    };
  }) as unknown as typeof fetch;

  return calls;
}

const SIGNUP_OK: StubbedResponse = {
  ok: true,
  json: {
    localId: "uid-1",
    idToken: "id-token-1",
    refreshToken: "refresh-1",
    expiresIn: "3600",
  },
};

test("password signup writes the merchant's name onto the GIP account", async () => {
  const calls = stubFetch([SIGNUP_OK, { ok: true, json: {} }]);

  const result = await signUpWithName(
    "merchant@example.com",
    "correct-horse-battery",
    "  Ada Lovelace  ",
  );

  expect(calls).toHaveLength(2);
  expect(calls[1]!.url).toContain("accounts:update");
  expect(calls[1]!.body).toMatchObject({
    idToken: "id-token-1",
    displayName: "Ada Lovelace",
  });
  expect(result.uid).toBe("uid-1");
  expect(result.displayName).toBe("Ada Lovelace");
});

test("signup still succeeds when the GIP name write fails", async () => {
  const calls = stubFetch([
    SIGNUP_OK,
    { ok: false, status: 500, json: { error: { message: "BOOM" } } },
  ]);

  const reported: string[] = [];
  const result = await signUpWithName(
    "merchant@example.com",
    "correct-horse-battery",
    "Ada Lovelace",
    (code) => reported.push(code),
  );

  // The account exists, so onboarding must be allowed to continue.
  expect(result.uid).toBe("uid-1");
  expect(result.idToken).toBe("id-token-1");
  expect(result.displayName).toBeUndefined();
  // ...but the failure is surfaced, not lost — and carries no PII.
  expect(reported).toEqual(["gip_profile_update_failed"]);
  expect(reported.join()).not.toContain("Ada");
  expect(calls).toHaveLength(2);
});

test("a blank name skips the profile write entirely", async () => {
  const calls = stubFetch([SIGNUP_OK]);

  const result = await signUpWithName(
    "merchant@example.com",
    "correct-horse-battery",
    "   ",
  );

  expect(calls).toHaveLength(1);
  expect(calls[0]!.url).toContain("accounts:signUp");
  expect(result.displayName).toBeUndefined();
});

test("the name never reaches the accounts:signUp request body", async () => {
  const calls = stubFetch([SIGNUP_OK, { ok: true, json: {} }]);

  await signUpWithName(
    "merchant@example.com",
    "correct-horse-battery",
    "Ada Lovelace",
  );

  expect(JSON.stringify(calls[0]!.body)).not.toContain("Ada");
});

test("Google sign-in carries the IdP display name through instead of discarding it", async () => {
  stubFetch([
    {
      ok: true,
      json: {
        localId: "uid-2",
        idToken: "id-token-2",
        refreshToken: "refresh-2",
        expiresIn: "3600",
        displayName: "Grace Hopper",
      },
    },
  ]);

  const result = await signInWithGoogle("google-credential");

  expect(result.displayName).toBe("Grace Hopper");
});

test("Google sign-in without a display name reports none", async () => {
  stubFetch([
    {
      ok: true,
      json: {
        localId: "uid-3",
        idToken: "id-token-3",
        refreshToken: "refresh-3",
        expiresIn: "3600",
        displayName: "   ",
      },
    },
  ]);

  const result = await signInWithGoogle("google-credential");

  expect(result.displayName).toBeUndefined();
});
