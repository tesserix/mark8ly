import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Issue #679 — the accept-invite server actions, asserted against the
// bytes they actually put on the wire rather than against a mocked
// module boundary. The bug being fixed was precisely a payload/endpoint
// mismatch (a GIP uid written where the Zitadel login path reads an
// email), so a test that stops at `expect(acceptInvitation).toHaveBeenCalled`
// would not have caught it.

vi.mock("next/headers", () => ({
  cookies: async () => ({
    set: vi.fn(),
    getAll: () => [],
  }),
}));

import {
  acceptInvite,
  acceptInviteWithZitadel,
} from "./actions";

const PLATFORM = "http://localhost:8086";
const ACCEPT_URL = `${PLATFORM}/api/v1/invitations/accept`;

interface Call {
  url: string;
  body: Record<string, unknown>;
}

let calls: Call[] = [];

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Records every request and answers each URL from `handlers`. */
function installFetch(handlers: Record<string, () => Response>) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : String(input);
    calls.push({
      url,
      body: init?.body ? JSON.parse(init.body as string) : {},
    });
    const handler = Object.entries(handlers).find(([u]) => url === u)?.[1];
    if (!handler) throw new Error(`unexpected fetch to ${url}`);
    return handler();
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

beforeEach(() => {
  calls = [];
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("acceptInviteWithZitadel", () => {
  it("sends password + verified_email and no GIP uid or id_token", async () => {
    installFetch({
      [ACCEPT_URL]: () =>
        jsonResponse(200, { data: { tenant_id: "tenant-1", role: "staff" } }),
    });

    const result = await acceptInviteWithZitadel({
      token: "invite-token",
      email: "staff@example.com",
      password: "correct-horse-battery",
    });

    expect(result).toEqual({
      ok: true,
      tenantId: "tenant-1",
      signInUrl: "/login/authorize?returnUrl=%2Fdashboard",
    });

    // Exactly one call, to platform-api's accept endpoint. Nothing to
    // GIP's Identity Toolkit, nothing to auth-bff's /auth/auto-login.
    expect(calls).toHaveLength(1);
    expect(calls[0]!.url).toBe(ACCEPT_URL);
    expect(calls[0]!.body).toEqual({
      token: "invite-token",
      verified_email: "staff@example.com",
      password: "correct-horse-battery",
    });
    // Spelled out separately: the absence of these keys is the fix.
    expect(calls[0]!.body).not.toHaveProperty("uid");
    expect(calls[0]!.body).not.toHaveProperty("id_token");
  });

  it("lowercases the email so the tuple it writes matches the one login reads", async () => {
    installFetch({
      [ACCEPT_URL]: () =>
        jsonResponse(200, { data: { tenant_id: "tenant-1", role: "staff" } }),
    });

    await acceptInviteWithZitadel({
      token: "invite-token",
      email: "  Staff@Example.COM ",
      password: "not-a-real-password",
    });

    expect(calls[0]!.body.verified_email).toBe("staff@example.com");
  });

  it("surfaces provisioning_failed with platform-api's own actionable message", async () => {
    installFetch({
      [ACCEPT_URL]: () =>
        jsonResponse(500, {
          error: "provisioning_failed",
          message:
            "we couldn't finish setting up your account — please try the invitation link again",
        }),
    });

    const result = await acceptInviteWithZitadel({
      token: "invite-token",
      email: "staff@example.com",
      password: "not-a-real-password",
    });

    expect(result.ok).toBe(false);
    if (result.ok) throw new Error("unreachable");
    expect(result.code).toBe("provisioning_failed");
    expect(result.message).toContain("invitation link again");
    expect(result.message).not.toMatch(/something went wrong/i);
  });

  it("falls back to actionable copy when provisioning_failed carries no message", async () => {
    installFetch({
      [ACCEPT_URL]: () => jsonResponse(500, { error: "provisioning_failed" }),
    });

    const result = await acceptInviteWithZitadel({
      token: "invite-token",
      email: "staff@example.com",
      password: "not-a-real-password",
    });

    if (result.ok) throw new Error("unreachable");
    expect(result.code).toBe("provisioning_failed");
    // platform-api sent nothing usable; `HTTP 500` must not be what the
    // invitee reads.
    expect(result.message).not.toMatch(/^HTTP /);
    expect(result.message).toMatch(/invitation link again/i);
  });
});

describe("acceptInvite (GIP path, unchanged)", () => {
  it("still sends token + uid + verified_email, and still auto-logs in against GIP", async () => {
    installFetch({
      [ACCEPT_URL]: () =>
        jsonResponse(200, { data: { tenant_id: "tenant-1", role: "staff" } }),
      "http://localhost:8087/auth/auto-login": () =>
        jsonResponse(200, {
          data: { uid: "gip-uid", email: "staff@example.com", tenant_id: "tenant-1" },
        }),
    });

    const result = await acceptInvite({
      token: "invite-token",
      idToken: "gip-id-token",
      uid: "gip-uid",
      verifiedEmail: "staff@example.com",
    });

    expect(result).toEqual({ ok: true, tenantId: "tenant-1" });

    expect(calls).toHaveLength(2);
    expect(calls[0]!.url).toBe(ACCEPT_URL);
    // Byte-identical to the pre-#679 payload: no password, no name fields.
    expect(calls[0]!.body).toEqual({
      token: "invite-token",
      uid: "gip-uid",
      verified_email: "staff@example.com",
    });
    expect(calls[1]!.url).toBe("http://localhost:8087/auth/auto-login");
    expect(calls[1]!.body).toMatchObject({
      id_token: "gip-id-token",
      workspace_tenant: "tenant-1",
    });
  });
});
