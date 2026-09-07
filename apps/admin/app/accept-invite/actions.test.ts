import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Issue #679 — the accept-invite server actions, asserted against the
// bytes they actually put on the wire rather than against a mocked
// module boundary. The bug being fixed was precisely a payload/endpoint
// mismatch (a GIP uid written where the Zitadel login path reads an
// email), so a test that stops at `expect(acceptInvitation).toHaveBeenCalled`
// would not have caught it.

import { acceptInviteWithZitadel } from "./actions";

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
  it("sends password + verified_email and no uid or id_token", async () => {
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

    // Exactly one call, to platform-api's accept endpoint — nothing to
    // any identity provider, and nothing to auth-bff.
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

  it("passes a 400 password_policy through with the rule-specific message", async () => {
    // platform-api now distinguishes "the password broke rule X" (400,
    // the caller's input) from "provisioning failed" (500, our fault).
    // The message names the rule, so it must reach the invitee verbatim.
    installFetch({
      [ACCEPT_URL]: () =>
        jsonResponse(400, {
          error: "password_policy",
          message:
            "That password is too short — it needs at least 12 characters.",
        }),
    });

    const result = await acceptInviteWithZitadel({
      token: "invite-token",
      email: "staff@example.com",
      password: "not-a-real-password",
    });

    if (result.ok) throw new Error("unreachable");
    expect(result.code).toBe("password_policy");
    expect(result.message).toContain("12 characters");
    expect(result.message).not.toMatch(/invitation link again/i);
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
