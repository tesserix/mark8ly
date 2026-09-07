import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// This file is the trace, not a per-file review. Phase 3a's six tasks each
// passed their own review and the feature was still unreachable end to
// end — a flag computed and never passed, a route built and never called.
// None of that shows up in a diff of any single file; it only shows up by
// walking the seams a real request walks.
//
// So unlike app/sign-in/actions.zitadel.test.ts (which mocks
// verifyCustomerCredential directly to drive customerSignIn in
// isolation), this file mocks only:
//   - the network boundary (global fetch)
//   - next/headers (a Next.js server-action runtime seam vitest has no
//     substitute for — there is no request to read headers/cookies from
//     outside a real Next.js server)
//
// customerSignIn, verifyCustomerCredential and encodeSession all run for
// real. A password submitted by the caller really goes over an HTTP
// request body to auth-bff.

const HOST = "shop.mark8ly.com";
const AUTH_BFF_URL = "http://localhost:8087";
const PLATFORM_API_URL = "http://localhost:8086";
const MARKETPLACE_API_URL = "http://localhost:8088";
/** Google's Identity Toolkit cert endpoint. Nothing in this flow should
 *  ever request it any more — asserted as an absence below. */
const CERTS_URL =
  "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com";

// --- next/headers: the one module boundary that is genuinely unavoidable
// here (no real Next.js request exists under vitest to read headers/
// cookies from). Backed by a plain in-memory store so cookies.set / .get /
// .delete behave like the real API customerSignIn calls.
const cookieStore: Record<string, string> = {};
const cookiesSetSpy = vi.fn(
  (opts: { name: string; value: string; domain?: string }) => {
    cookieStore[opts.name] = opts.value;
  },
);
let headerMap: Map<string, string>;

vi.mock("next/headers", () => ({
  headers: async () => ({
    get: (key: string) => headerMap.get(key.toLowerCase()) ?? null,
  }),
  cookies: async () => ({
    set: cookiesSetSpy,
    get: (name: string) =>
      cookieStore[name] !== undefined ? { value: cookieStore[name] } : undefined,
    delete: (name: string) => {
      delete cookieStore[name];
    },
  }),
}));

/** Decodes the base64 payload half of an encodeSession cookie value
 *  (`<base64-payload>.<hex-signature>`) without needing the HMAC key. */
function decodeCookiePayload(cookieValue: string): Record<string, unknown> {
  const payload = cookieValue.slice(0, cookieValue.lastIndexOf("."));
  return JSON.parse(Buffer.from(payload, "base64").toString());
}

async function loadActions() {
  return await import("./actions");
}

const originalFetch = globalThis.fetch;
let fetchMock: ReturnType<typeof vi.fn>;
/** URLs requested, in order — used to assert both presence and absence
 *  of specific network hops (e.g. "the certs URL was never requested"). */
let requestedUrls: string[];

/** A fetch stand-in that routes each known URL to its real-service shape
 *  and throws on anything unexpected, so a wrong-branch call surfaces
 *  loudly instead of silently returning `undefined`-shaped JSON. */
function installFetchRouter(handlers: {
  customerLogin?: (body: {
    login_name: string;
    password: string;
  }) => { status: number; body: unknown };
}) {
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    requestedUrls.push(url);

    if (url === `${AUTH_BFF_URL}/auth/customer/login`) {
      const submitted = JSON.parse(String(init?.body)) as {
        login_name: string;
        password: string;
      };
      const { status, body } = handlers.customerLogin
        ? handlers.customerLogin(submitted)
        : { status: 500, body: {} };
      return jsonResponse(status, body);
    }

    if (
      url === `${PLATFORM_API_URL}/internal/stores/by-slug/shop` ||
      url.startsWith(`${PLATFORM_API_URL}/internal/stores/by-slug/`)
    ) {
      return jsonResponse(200, {
        data: { tenant_id: "tenant-1", id: "store-1" },
      });
    }

    // The membership probe completeCustomerSignIn runs BEFORE it hands a
    // session cookie to the browser. These traces are about the
    // credential seams, so the customer is a member throughout; the gate
    // itself is covered in lib/auth/customer-session.membership.test.ts.
    if (
      url.startsWith(`${MARKETPLACE_API_URL}`) &&
      url.includes("/account/membership")
    ) {
      return jsonResponse(200, { data: { member: true } });
    }

    if (url.startsWith(`${MARKETPLACE_API_URL}`) && url.includes("/account")) {
      return jsonResponse(200, {});
    }

    if (
      url.startsWith(`${MARKETPLACE_API_URL}`) &&
      url.includes("/loyalty/enroll")
    ) {
      return jsonResponse(200, {});
    }

    throw new Error(`unexpected fetch to unmocked URL: ${url}`);
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
}

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: () => null } as unknown as Headers,
    json: async () => body,
  } as Response;
}

beforeEach(() => {
  vi.resetModules();
  process.env.AUTH_BFF_URL = AUTH_BFF_URL;
  process.env.PLATFORM_API_URL = PLATFORM_API_URL;
  process.env.MARKETPLACE_API_URL = MARKETPLACE_API_URL;

  headerMap = new Map([["host", HOST]]);
  for (const key of Object.keys(cookieStore)) delete cookieStore[key];
  cookiesSetSpy.mockClear();
  requestedUrls = [];
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.clearAllMocks();
});

describe("customer login flow — every seam traversed", () => {
  it("password sign-in reaches verifyCustomerCredential, mints a session from auth-bff's identity, scopes the cookie to the host, and fires profile/loyalty side effects", async () => {
    installFetchRouter({
      customerLogin: (submitted) => {
        // Seam 1: the credential that reaches auth-bff over the wire is
        // exactly what the caller submitted — proof verifyCustomerCredential
        // was actually invoked with it, not a mock standing in for it.
        expect(submitted).toEqual({
          login_name: "customer@example.com",
          password: "correct horse battery staple",
        });
        return {
          status: 200,
          body: { data: { uid: "zit-uid-1", email: "trusted@example.com" } },
        };
      },
    });

    const { customerSignIn } = await loadActions();
    const result = await customerSignIn({
      storeSlug: "shop",
      loginName: "customer@example.com",
      password: "correct horse battery staple",
      // A client-supplied uid/email must never win — proven by seam 3 below.
      uid: "attacker-supplied-uid",
      email: "attacker@evil.com",
    });

    expect(result.ok).toBe(true);

    // Seam 1 (call-shape): the auth-bff customer login endpoint was hit
    // exactly once — this is verifyCustomerCredential actually running.
    const loginCalls = requestedUrls.filter(
      (u) => u === `${AUTH_BFF_URL}/auth/customer/login`,
    );
    expect(loginCalls).toHaveLength(1);

    // Seam 2: nothing reached out to Google's cert endpoint — no
    // Identity Toolkit token verification remains in this path.
    expect(requestedUrls).not.toContain(CERTS_URL);

    // Seam 3: encodeSession was fed auth-bff's identity, not the
    // client-supplied uid/email above.
    expect(cookiesSetSpy).toHaveBeenCalledTimes(1);
    const cookieCall = cookiesSetSpy.mock.calls[0]![0] as {
      name: string;
      value: string;
      domain?: string;
    };
    expect(cookieCall.name).toBe("mp_customer_session");
    const decoded = decodeCookiePayload(cookieCall.value);
    expect(decoded.uid).toBe("zit-uid-1");
    expect(decoded.email).toBe("trusted@example.com");

    // Seam 4: the cookie is scoped to the resolved request host.
    expect(cookieCall.domain).toBe(HOST);

    // Seam 5: profile + loyalty side effects fired.
    expect(requestedUrls.some((u) => u.includes("/account"))).toBe(true);
    expect(requestedUrls.some((u) => u.includes("/loyalty/enroll"))).toBe(
      true,
    );
  });
});

