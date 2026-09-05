import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createPublicKey,
  createSign,
  generateKeyPairSync,
} from "node:crypto";

// This file is the trace, not a per-file review. Phase 3a's six tasks each
// passed their own review and the feature was still unreachable end to
// end — a flag computed and never passed, a route built and never called.
// None of that shows up in a diff of any single file; it only shows up by
// walking the seams a real request walks.
//
// So unlike app/sign-in/actions.zitadel.test.ts (which mocks
// verifyGIPIdToken and verifyCustomerCredential directly to drive
// customerSignIn's branches in isolation), this file mocks only:
//   - the network boundary (global fetch)
//   - next/headers (a Next.js server-action runtime seam vitest has no
//     substitute for — there is no request to read headers/cookies from
//     outside a real Next.js server)
//
// customerSignIn, verifyCustomerCredential, verifyGIPIdToken, and
// encodeSession all run for real. A password submitted by the caller
// really goes over an HTTP request body to auth-bff; a GIP id_token is a
// real RS256-signed JWT verified against a real RSA keypair served back
// through the mocked fetch as if it were Google's certs endpoint.

const HOST = "shop.mark8ly.com";
const AUTH_BFF_URL = "http://localhost:8087";
const PLATFORM_API_URL = "http://localhost:8086";
const MARKETPLACE_API_URL = "http://localhost:8088";
const CERTS_URL =
  "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com";

const GIP_PROJECT_ID = "test-project";
const GIP_CUSTOMER_TENANT_ID = "test-customer-tenant";

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

/** Generates a real RSA keypair and a real RS256-signed GIP-shaped id_token,
 *  plus the "Google certs" document verifyGIPIdToken would fetch to check
 *  it. Nothing here is a stub of the verification logic — it is the same
 *  signature math verify-id-token.ts performs, run backwards. */
function makeSignedGIPToken(claims: {
  uid: string;
  email: string;
  projectId: string;
  tenantId: string;
}): { idToken: string; certsBody: Record<string, string> } {
  const { publicKey, privateKey } = generateKeyPairSync("rsa", {
    modulusLength: 2048,
  });
  const kid = "test-kid";
  const header = { alg: "RS256", kid };
  const now = Math.floor(Date.now() / 1000);
  const payload = {
    aud: claims.projectId,
    iss: `https://securetoken.google.com/${claims.projectId}`,
    sub: claims.uid,
    email: claims.email,
    email_verified: true,
    iat: now,
    exp: now + 3600,
    firebase: { tenant: claims.tenantId },
  };
  const b64url = (o: unknown) =>
    Buffer.from(JSON.stringify(o)).toString("base64url");
  const signingInput = `${b64url(header)}.${b64url(payload)}`;
  const signer = createSign("RSA-SHA256");
  signer.update(signingInput);
  signer.end();
  const signature = signer.sign(privateKey).toString("base64url");

  const pubPem = publicKey.export({ type: "spki", format: "pem" }) as string;
  return {
    idToken: `${signingInput}.${signature}`,
    certsBody: { [kid]: pubPem },
  };
}

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
  certs?: () => { status: number; body: unknown };
  customerLogin?: (body: {
    login_name: string;
    password: string;
  }) => { status: number; body: unknown };
}) {
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    requestedUrls.push(url);

    if (url === CERTS_URL) {
      const { status, body } = handlers.certs
        ? handlers.certs()
        : { status: 500, body: {} };
      return jsonResponse(status, body);
    }

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
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  process.env.GIP_PROJECT_ID = GIP_PROJECT_ID;
  process.env.GIP_CUSTOMER_TENANT_ID = GIP_CUSTOMER_TENANT_ID;
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
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  vi.clearAllMocks();
});

describe("customer login flow — Zitadel provider, every seam traversed", () => {
  it("password sign-in reaches verifyCustomerCredential, skips verifyGIPIdToken, mints a session from auth-bff's identity, scopes the cookie to the host, and fires profile/loyalty side effects", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
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

    // Seam 2: verifyGIPIdToken was never reached. If it had been, it would
    // have needed to fetch Google's certs to verify a signature — that
    // fetch never happened.
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

describe("customer login flow — GIP provider (flag unset), mirror trace", () => {
  it("id_token sign-in reaches verifyGIPIdToken, never touches the Zitadel client, mints a session from the verified token, and still scopes the cookie to the host", async () => {
    // AUTH_PROVIDER stays "gip" — NEXT_PUBLIC_AUTH_PROVIDER unset, per
    // beforeEach.
    const { idToken, certsBody } = makeSignedGIPToken({
      uid: "gip-uid-1",
      email: "gip-trusted@example.com",
      projectId: GIP_PROJECT_ID,
      tenantId: GIP_CUSTOMER_TENANT_ID,
    });
    installFetchRouter({
      certs: () => ({ status: 200, body: certsBody }),
    });

    const { customerSignIn } = await loadActions();
    const result = await customerSignIn({
      storeSlug: "shop",
      idToken,
      uid: "attacker-supplied-uid",
      email: "attacker@evil.com",
    });

    expect(result.ok).toBe(true);

    // The certs fetch is verifyGIPIdToken's own real network hop — proof
    // it, not a stand-in, actually verified the signature.
    expect(requestedUrls).toContain(CERTS_URL);

    // The Zitadel client's endpoint was never called.
    expect(requestedUrls).not.toContain(`${AUTH_BFF_URL}/auth/customer/login`);

    expect(cookiesSetSpy).toHaveBeenCalledTimes(1);
    const cookieCall = cookiesSetSpy.mock.calls[0]![0] as {
      name: string;
      value: string;
      domain?: string;
    };
    const decoded = decodeCookiePayload(cookieCall.value);
    expect(decoded.uid).toBe("gip-uid-1");
    expect(decoded.email).toBe("gip-trusted@example.com");
    expect(cookieCall.domain).toBe(HOST);
  });
});

describe("customer login flow — the GIP branch is genuinely unreachable under the Zitadel flag", () => {
  // The previous phase's failure would NOT have been caught by an assertion
  // that merely reads "verifyGIPIdToken was not called this time" — that is
  // also true if the branch were reached but happened to no-op, or if the
  // call recorder itself were wired to nothing. What distinguishes "did not
  // run" from "could not have produced this outcome" is a poison pill: rig
  // the GIP path so that if it executes at all, it fails in a specific,
  // detectable way that could never coincidentally match the Zitadel
  // path's success. No idToken is supplied, and GIP_PROJECT_ID /
  // GIP_CUSTOMER_TENANT_ID are deliberately left as they are — verifyGIPIdToken
  // rejects an empty idToken before doing anything else (see
  // verify-id-token.ts's very first check), and customerSignIn's catch
  // block turns that into a specific, distinguishable error shape
  // (ok:false, code:"invalid_token") that a successful Zitadel result can
  // never equal. A passing Zitadel result here is proof the GIP branch's
  // code never ran — not merely proof a spy sitting on it wasn't tripped.
  it("succeeds via the Zitadel branch even though the GIP branch would fail immediately and distinctly if entered", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    installFetchRouter({
      customerLogin: () => ({
        status: 200,
        body: { data: { uid: "zit-uid-2", email: "zit2@example.com" } },
      }),
    });

    const { customerSignIn } = await loadActions();
    const result = await customerSignIn({
      storeSlug: "shop",
      loginName: "zit2@example.com",
      password: "another-correct-password",
      // No idToken at all — if the GIP branch ran, verifyGIPIdToken would
      // throw GIPTokenVerificationError("missing verification
      // configuration") synchronously, before any network call, and
      // customerSignIn's catch block maps that specific error type to
      // { ok: false, code: "invalid_token" }.
    });

    // This is the trap springing (or not): ok:true, with the Zitadel
    // identity, is reachable ONLY if the GIP branch's throw never
    // happened — i.e. the GIP branch's code was never entered, not just
    // "not observed to have been entered".
    expect(result).toMatchObject({ ok: true });
    if (result.ok) {
      // no message/code fields on success — nothing further to assert
      // beyond ok:true itself, which is already the discriminant.
    }
    expect(requestedUrls).not.toContain(CERTS_URL);

    const cookieCall = cookiesSetSpy.mock.calls[0]![0] as { value: string };
    const decoded = decodeCookiePayload(cookieCall.value);
    expect(decoded.uid).toBe("zit-uid-2");
  });
});
