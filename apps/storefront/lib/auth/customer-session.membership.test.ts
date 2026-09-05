import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The store-membership gate, end to end through the real
// completeCustomerSignIn / completeCustomerStoreJoin tail.
//
// The bug these guard: a customer who signed up at store1 could sign in
// at store2 and silently acquire an account there, because a valid
// credential was treated as permission to be at any store. See
// docs/superpowers/specs/2026-09-05-customer-store-membership-design.md.
//
// The invariant asserted below is not "a row is absent" but "no session
// cookie is ever handed to the browser for a store the customer has not
// joined, and the only call that creates a membership is the one the
// customer explicitly asked for".

const cookieStore: Record<string, string> = {};
const cookiesSetSpy = vi.fn(
  (opts: { name: string; value: string; domain?: string; maxAge?: number }) => {
    cookieStore[opts.name] = opts.value;
  },
);
const cookiesDeleteSpy = vi.fn((name: string) => {
  delete cookieStore[name];
});

const HOST = "store-two.mark8ly.com";
let headerMap: Map<string, string>;

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

vi.mock("@/lib/api/server/platformInternal", () => ({
  platformInternalFetch: vi.fn(),
}));

vi.mock("@/lib/slug", () => ({ resolveStoreSlug: vi.fn() }));

import { resolveStoreSlug } from "@/lib/slug";
import { JOIN_GRANT_COOKIE, verifyJoinGrant } from "@/lib/auth/join-grant";

const resolveStoreSlugMock = vi.mocked(resolveStoreSlug);

const STORE = { tenant_id: "tenant-2", store_id: "store-2" };
const STORE_SLUG = "store-two";
const VERIFIED = { uid: "uid-1", email: "shopper@example.com" };

const originalFetch = globalThis.fetch;
let calls: { url: string; method: string }[];

/** Routes the marketplace-api calls completeCustomerSignIn makes.
 *  `member` decides what the membership probe reports; `joinStatus`
 *  decides how the explicit join responds. */
function installFetch(opts: {
  member: boolean | "unavailable";
  joinStatus?: number;
  joinError?: string;
}) {
  calls = [];
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, method: init?.method ?? "GET" });

    if (url.includes("/account/membership")) {
      if (opts.member === "unavailable") {
        return jsonResponse(503, {});
      }
      return jsonResponse(200, { data: { member: opts.member } });
    }
    if (url.includes("/account/join")) {
      const status = opts.joinStatus ?? 200;
      return jsonResponse(
        status,
        status === 200 ? { data: {} } : { error: opts.joinError },
      );
    }
    if (url.includes("/loyalty/enroll")) return jsonResponse(200, {});
    throw new Error(`unexpected fetch: ${url}`);
  }) as unknown as typeof fetch;
}

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response;
}

async function loadSession() {
  return await import("@/lib/auth/customer-session");
}

beforeEach(() => {
  vi.resetModules();
  headerMap = new Map([["host", HOST]]);
  for (const key of Object.keys(cookieStore)) delete cookieStore[key];
  cookiesSetSpy.mockClear();
  cookiesDeleteSpy.mockClear();
  resolveStoreSlugMock.mockReset();
  resolveStoreSlugMock.mockResolvedValue(STORE_SLUG);
});

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe("completeCustomerSignIn — a verified credential is not a membership", () => {
  it("mints no session at a store the customer has not joined, and offers the join instead", async () => {
    installFetch({ member: false });
    const { completeCustomerSignIn } = await loadSession();

    const result = await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    expect(result).toMatchObject({ ok: false, code: "membership_required" });
    expect(cookieStore["mp_customer_session"]).toBeUndefined();
    expect(
      cookiesSetSpy.mock.calls.some(
        (c) => c[0].name === "mp_customer_session",
      ),
    ).toBe(false);
  });

  it("never creates the membership on the sign-in path — only the probe is called", async () => {
    installFetch({ member: false });
    const { completeCustomerSignIn } = await loadSession();

    await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    const joinCalls = calls.filter((c) => c.url.includes("/account/join"));
    expect(joinCalls).toEqual([]);
    // Nor may it fall back to the old "poke /account so the middleware
    // upserts a row" trick, which is what made membership incidental.
    const accountPokes = calls.filter(
      (c) => c.url.includes("/account") && !c.url.includes("/account/membership"),
    );
    expect(accountPokes).toEqual([]);
  });

  it("does not run the loyalty side effects for a non-member", async () => {
    installFetch({ member: false });
    const { completeCustomerSignIn } = await loadSession();

    await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    expect(calls.some((c) => c.url.includes("/loyalty/enroll"))).toBe(false);
  });

  it("issues a short-lived, host-scoped join grant carrying the verified identity", async () => {
    installFetch({ member: false });
    const { completeCustomerSignIn } = await loadSession();

    await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    const grantCall = cookiesSetSpy.mock.calls.find(
      (c) => c[0].name === JOIN_GRANT_COOKIE,
    );
    expect(grantCall).toBeDefined();
    expect(grantCall![0]).toMatchObject({ domain: HOST, maxAge: 600 });

    const claims = verifyJoinGrant(grantCall![0].value, STORE_SLUG);
    expect(claims).toMatchObject({
      uid: VERIFIED.uid,
      email: VERIFIED.email,
      store_id: STORE.store_id,
    });
  });

  it("the copy never implies a wrong password", async () => {
    installFetch({ member: false });
    const { completeCustomerSignIn } = await loadSession();

    const result = await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    if (result.ok) throw new Error("expected the gate to refuse");
    const message = result.message.toLowerCase();
    for (const forbidden of ["password", "incorrect", "credentials"]) {
      expect(message).not.toContain(forbidden);
    }
  });

  it("fails closed when the membership check is unavailable — no session, no silent sign-in", async () => {
    installFetch({ member: "unavailable" });
    const { completeCustomerSignIn } = await loadSession();

    const result = await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    expect(result).toMatchObject({ ok: false, code: "membership_unavailable" });
    expect(cookieStore["mp_customer_session"]).toBeUndefined();
    expect(cookieStore[JOIN_GRANT_COOKIE]).toBeUndefined();
  });

  it("signs an existing member in exactly as before", async () => {
    installFetch({ member: true });
    const { completeCustomerSignIn } = await loadSession();

    const result = await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);

    expect(result).toEqual({ ok: true });
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session", domain: HOST }),
    );
    expect(calls.some((c) => c.url.includes("/loyalty/enroll"))).toBe(true);
    // An existing member is looked up, never re-created.
    expect(calls.some((c) => c.url.includes("/account/join"))).toBe(false);
  });
});

describe("joinThisStore — the explicit join", () => {
  async function primeGrant() {
    installFetch({ member: false });
    const { completeCustomerSignIn } = await loadSession();
    await completeCustomerSignIn(STORE, HOST, STORE_SLUG, VERIFIED);
  }

  it("creates the membership and only then mints the session", async () => {
    await primeGrant();
    installFetch({ member: false, joinStatus: 200 });
    const { joinThisStore } = await import("@/app/join/actions");

    const result = await joinThisStore();

    expect(result).toEqual({ ok: true });
    const joinIndex = calls.findIndex((c) => c.url.includes("/account/join"));
    expect(joinIndex).toBeGreaterThanOrEqual(0);
    expect(calls[joinIndex]!.method).toBe("POST");
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "mp_customer_session", domain: HOST }),
    );
    expect(cookiesDeleteSpy).toHaveBeenCalledWith(JOIN_GRANT_COOKIE);
  });

  it("refuses without a grant — a join is never reachable from an unverified request", async () => {
    installFetch({ member: false });
    const { joinThisStore } = await import("@/app/join/actions");

    const result = await joinThisStore();

    expect(result).toMatchObject({ ok: false, code: "join_grant_invalid" });
    expect(calls.some((c) => c.url.includes("/account/join"))).toBe(false);
    expect(cookieStore["mp_customer_session"]).toBeUndefined();
  });

  it("refuses a grant minted for a different store", async () => {
    await primeGrant();
    installFetch({ member: false });
    // Same browser, same grant cookie, but the request now resolves to a
    // third store. Without the store check the grant would join it.
    resolveStoreSlugMock.mockResolvedValue("store-three");
    const { joinThisStore } = await import("@/app/join/actions");

    const result = await joinThisStore();

    expect(result).toMatchObject({ ok: false, code: "join_grant_invalid" });
    expect(calls.some((c) => c.url.includes("/account/join"))).toBe(false);
  });

  it("surfaces a blocked customer without minting a session", async () => {
    await primeGrant();
    installFetch({ member: false, joinStatus: 403, joinError: "account_blocked" });
    const { joinThisStore } = await import("@/app/join/actions");

    const result = await joinThisStore();

    expect(result).toMatchObject({ ok: false, code: "account_blocked" });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message.toLowerCase()).not.toContain("try again");
    }
    expect(
      cookiesSetSpy.mock.calls.some((c) => c[0].name === "mp_customer_session"),
    ).toBe(false);
  });

  it("mints no session when the join call fails", async () => {
    await primeGrant();
    installFetch({ member: false, joinStatus: 500 });
    const { joinThisStore } = await import("@/app/join/actions");

    const result = await joinThisStore();

    expect(result).toMatchObject({ ok: false, code: "join_failed" });
    expect(
      cookiesSetSpy.mock.calls.some((c) => c[0].name === "mp_customer_session"),
    ).toBe(false);
  });
});
