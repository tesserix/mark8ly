import { test, expect } from "@playwright/test";

// Issue #685 — the onboarding completion server actions, asserted
// against the bytes they actually put on the wire.
//
// The bug being fixed was precisely a payload/endpoint mismatch: a GIP
// uid written where the Zitadel admin login path reads an email. A test
// that stopped at "the action returned ok" would not have caught it, and
// neither would one that mocked the platform-api client module — the
// mismatch lives in the JSON body.

import {
  completeOnboarding,
  completeOnboardingWithZitadel,
} from "../../app/onboarding/actions";

interface RecordedCall {
  url: string;
  body: Record<string, unknown>;
}

const SESSION_ID = "sess-685";
const SESSION_URL = `http://localhost:8086/api/v1/onboarding/sessions/${SESSION_ID}`;
const COMPLETE_URL = `${SESSION_URL}/complete`;

// A password that satisfies the live Zitadel policy (12+ characters,
// upper, lower, number, symbol) while being unmistakably not a real
// credential.
const PASSWORD = "Not-A-Real-Password-1!";

const SESSION_BODY = {
  data: {
    id: SESSION_ID,
    // Mixed case on purpose: every email-keyed FGA tuple is lowercase.
    email: "Founder@Example.test",
    status: "in_progress",
    email_verified_at: "2026-09-05T00:00:00Z",
    draft: {
      business_name: "Bondi Surf Co",
      slug: "bondi-surf",
      country_code: "AU",
      currency_code: "AUD",
      timezone: "Australia/Sydney",
    },
  },
};

const COMPLETE_BODY = {
  data: { tenant_id: "tenant-685", slug: "bondi-surf" },
};

/**
 * Replaces global fetch with a router over exact URLs, recording every
 * request. Any URL without a handler fails the test from inside the
 * action — which is how "the Zitadel path must never call auth-bff" is
 * asserted below rather than merely hoped for.
 */
function installFetch(
  handlers: Record<string, () => { status?: number; json: unknown }>,
): RecordedCall[] {
  const calls: RecordedCall[] = [];
  globalThis.fetch = (async (input: unknown, init?: RequestInit) => {
    const url = String(input);
    calls.push({
      url,
      body: JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>,
    });
    const handler = handlers[url];
    if (!handler) throw new Error(`unexpected request to ${url}`);
    const { status = 200, json } = handler();
    return {
      ok: status >= 200 && status < 300,
      status,
      json: async () => json,
      headers: { get: () => null },
    };
  }) as unknown as typeof fetch;
  return calls;
}

test("the Zitadel path sends a password and no owner_user_id", async () => {
  const calls = installFetch({
    [SESSION_URL]: () => ({ json: SESSION_BODY }),
    [COMPLETE_URL]: () => ({ json: COMPLETE_BODY }),
  });

  const result = await completeOnboardingWithZitadel({
    sessionId: SESSION_ID,
    password: PASSWORD,
    name: "Ada Lovelace",
  });

  expect(result.ok).toBe(true);

  const complete = calls.find((c) => c.url === COMPLETE_URL);
  expect(complete, "the complete endpoint was never called").toBeTruthy();
  // The whole bug in one assertion: no GIP uid may be sent as the owner.
  expect(complete!.body).not.toHaveProperty("owner_user_id");
  expect(complete!.body.password).toBe(PASSWORD);
  expect(complete!.body.first_name).toBe("Ada");
  expect(complete!.body.last_name).toBe("Lovelace");
});

test("the Zitadel path lowercases the owner email it sends", async () => {
  const calls = installFetch({
    [SESSION_URL]: () => ({ json: SESSION_BODY }),
    [COMPLETE_URL]: () => ({ json: COMPLETE_BODY }),
  });

  await completeOnboardingWithZitadel({
    sessionId: SESSION_ID,
    password: PASSWORD,
    name: "Ada Lovelace",
  });

  // The session carries Founder@Example.test. An email-keyed FGA tuple
  // written verbatim would never match the login path's lowercased
  // lookup, and the merchant would be told "We couldn't find a store for
  // this account" seconds after finishing onboarding.
  const complete = calls.find((c) => c.url === COMPLETE_URL)!;
  expect(complete.body.owner_email).toBe("founder@example.test");
});

test("the Zitadel path never calls auth-bff auto-login", async () => {
  // Only the two platform-api URLs are routed. auth-bff's /auth/auto-login
  // verifies a GIP id_token; there is none on this path, and a call to it
  // would throw here rather than silently 401ing in production.
  const calls = installFetch({
    [SESSION_URL]: () => ({ json: SESSION_BODY }),
    [COMPLETE_URL]: () => ({ json: COMPLETE_BODY }),
  });

  const result = await completeOnboardingWithZitadel({
    sessionId: SESSION_ID,
    password: PASSWORD,
    name: "Ada Lovelace",
  });

  expect(result.ok).toBe(true);
  expect(calls.map((c) => c.url)).toEqual([SESSION_URL, COMPLETE_URL]);
});

test("a single-word name still fills both Zitadel profile fields", async () => {
  const calls = installFetch({
    [SESSION_URL]: () => ({ json: SESSION_BODY }),
    [COMPLETE_URL]: () => ({ json: COMPLETE_BODY }),
  });

  await completeOnboardingWithZitadel({
    sessionId: SESSION_ID,
    password: PASSWORD,
    name: "Ada",
  });

  // Zitadel rejects an empty givenName/familyName outright.
  const complete = calls.find((c) => c.url === COMPLETE_URL)!;
  expect(complete.body.first_name).toBe("Ada");
  expect(complete.body.last_name).toBe("Ada");
});

test("a password_policy rejection reaches the caller with its own code", async () => {
  installFetch({
    [SESSION_URL]: () => ({ json: SESSION_BODY }),
    [COMPLETE_URL]: () => ({
      status: 400,
      json: {
        error: "password_policy",
        message:
          "Password must be at least 12 characters, with an uppercase letter, a lowercase letter, a number, and a symbol.",
      },
    }),
  });

  const result = await completeOnboardingWithZitadel({
    sessionId: SESSION_ID,
    password: "short",
    name: "Ada Lovelace",
  });

  expect(result.ok).toBe(false);
  if (result.ok) return;
  // The form puts this on the password field, not in a generic alert —
  // so the code has to survive the round trip intact.
  expect(result.code).toBe("password_policy");
  expect(result.message).toContain("12 characters");
});

test("the GIP path still sends owner_user_id and still calls auth-bff", async () => {
  const MEMBERSHIPS_URL =
    "http://localhost:8086/api/v1/users/me/tenants?uid=gip-uid-1";
  const AUTO_LOGIN_URL = "http://localhost:8087/auth/auto-login";

  const calls = installFetch({
    [SESSION_URL]: () => ({ json: SESSION_BODY }),
    [MEMBERSHIPS_URL]: () => ({ json: { data: [] } }),
    [COMPLETE_URL]: () => ({ json: COMPLETE_BODY }),
    [AUTO_LOGIN_URL]: () => ({ json: { data: {} } }),
  });

  const result = await completeOnboarding({
    sessionId: SESSION_ID,
    gipUid: "gip-uid-1",
    gipIdToken: "gip-id-token",
  });

  expect(result.ok).toBe(true);

  const complete = calls.find((c) => c.url === COMPLETE_URL)!;
  expect(complete.body.owner_user_id).toBe("gip-uid-1");
  // And nothing from the Zitadel path leaked into it.
  expect(complete.body).not.toHaveProperty("password");
  expect(complete.body).not.toHaveProperty("first_name");
  expect(calls.some((c) => c.url === AUTO_LOGIN_URL)).toBe(true);
});
