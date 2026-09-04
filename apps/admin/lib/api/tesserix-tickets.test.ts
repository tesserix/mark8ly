import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./platform-token", () => ({
  getPlatformToken: vi.fn(async () => "machine-token"),
  __resetPlatformTokenCache: () => {},
}));

import {
  filePlatformTicket,
  getMyPlatformTicket,
  listMyPlatformTickets,
  replyToPlatformTicket,
} from "./tesserix";

// The four calls repointed from apps/web's shared-secret /api/internal/* to
// tesserix-home's platform API (tesserix-home#152).
//
// These assert the REQUEST as much as the response, because the failures that
// matter here are silent on this side: a missing tenant reads another
// merchant's queue on the old API and is refused on the new one, and a reply
// with no author is recorded as the support team rather than the merchant.

const TENANT = "8c302556-b647-4824-8ce4-73f547ca456e";

function ok(body: unknown) {
  return { ok: true, status: 200, json: async () => body } as unknown as Response;
}

function lastCall(m: ReturnType<typeof vi.fn>) {
  const [url, init] = m.mock.calls[m.mock.calls.length - 1];
  return {
    url: new URL(String(url)),
    init: init as RequestInit,
    body: init && (init as RequestInit).body ? JSON.parse(String((init as RequestInit).body)) : null,
    headers: new Headers((init as RequestInit)?.headers),
  };
}

beforeEach(() => {
  process.env.TESSERIX_PLATFORM_API_URL = "http://platform-api.tesserix.svc.cluster.local";
});
afterEach(() => {
  vi.unstubAllGlobals();
  delete process.env.TESSERIX_PLATFORM_API_URL;
});

describe("every ticket call", () => {
  it("goes to the platform API with a Bearer token, not the shared secret", async () => {
    const f = vi.fn().mockResolvedValue(ok({ data: { tickets: [] } }));
    vi.stubGlobal("fetch", f);

    await listMyPlatformTickets(TENANT);

    const { url, headers } = lastCall(f);
    expect(url.host).toBe("platform-api.tesserix.svc.cluster.local");
    expect(url.pathname).toBe("/v1/tickets");
    expect(headers.get("authorization")).toBe("Bearer machine-token");
    // The old channel's header must be gone: sending both would leave the
    // shared secret in flight to a service that ignores it.
    expect(headers.get("x-internal-token")).toBeNull();
  });
});

describe("listing", () => {
  it("names the tenant, without which the API refuses", async () => {
    const f = vi.fn().mockResolvedValue(ok({ data: { tickets: [{ id: "t1" }] } }));
    vi.stubGlobal("fetch", f);

    const rows = await listMyPlatformTickets(TENANT);

    expect(lastCall(f).url.searchParams.get("tenant")).toBe(TENANT);
    expect(rows).toHaveLength(1);
  });

  it("reads data.tickets, not the old rows key", async () => {
    // The envelope changed shape. Reading the old key returns [] on every
    // call, which renders as "no tickets" rather than as an error.
    const f = vi.fn().mockResolvedValue(ok({ data: { tickets: [{ id: "t1" }, { id: "t2" }] } }));
    vi.stubGlobal("fetch", f);

    expect(await listMyPlatformTickets(TENANT)).toHaveLength(2);
  });

  it("returns nothing when no tenant is known, without calling out", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);

    expect(await listMyPlatformTickets("")).toEqual([]);
    expect(f).not.toHaveBeenCalled();
  });
});

describe("detail", () => {
  it("names the tenant and reads the envelope's data", async () => {
    const f = vi.fn().mockResolvedValue(
      ok({ data: { ticket: { id: "t1" }, replies: [{ id: "r1" }] } }),
    );
    vi.stubGlobal("fetch", f);

    const got = await getMyPlatformTicket(TENANT, "t1");

    expect(lastCall(f).url.pathname).toBe("/v1/tickets/t1");
    expect(lastCall(f).url.searchParams.get("tenant")).toBe(TENANT);
    expect(got?.ticket.id).toBe("t1");
    expect(got?.replies).toHaveLength(1);
  });
});

describe("filing", () => {
  it("sends the platform API's snake_case contract and no product", async () => {
    const f = vi.fn().mockResolvedValue(ok({ data: { ticket: { id: "new" } } }));
    vi.stubGlobal("fetch", f);

    await filePlatformTicket({
      tenantId: TENANT,
      subject: "Payouts delayed",
      description: "Three pending",
      priority: "high",
      submittedByName: "Priya",
      submittedByEmail: "p@example.com",
      submittedByUserId: "uid-1",
    });

    const { url, body } = lastCall(f);
    expect(url.pathname).toBe("/v1/tickets");
    expect(body).toMatchObject({
      tenant_id: TENANT,
      subject: "Payouts delayed",
      priority: "high",
      submitted_by_name: "Priya",
      submitted_by_email: "p@example.com",
      submitted_by_user_id: "uid-1",
    });
    // The product comes from the caller's SCOPE on the server. Sending one
    // would be a field the API does not read and a claim it does not honour.
    expect(body).not.toHaveProperty("productId");
    expect(body).not.toHaveProperty("product_id");
  });
});

describe("replying", () => {
  it("carries the merchant's identity, which is what stops it being filed as the platform", async () => {
    const f = vi.fn().mockResolvedValue(ok({ data: { reply: { id: "r1" } } }));
    vi.stubGlobal("fetch", f);

    await replyToPlatformTicket({
      tenantId: TENANT,
      ticketId: "t1",
      content: "Any update?",
      authorName: "Priya",
      authorEmail: "p@example.com",
      authorUserId: "uid-1",
    });

    const { url, body } = lastCall(f);
    expect(url.pathname).toBe("/v1/tickets/t1/replies");
    expect(body).toMatchObject({
      tenant_id: TENANT,
      content: "Any update?",
      author_name: "Priya",
      author_email: "p@example.com",
      author_user_id: "uid-1",
    });
  });

  it("surfaces the API's own refusal message rather than a bare status", async () => {
    const f = vi.fn().mockResolvedValue({
      ok: false,
      status: 422,
      json: async () => ({ error: { code: "VALIDATION", message: "a merchant author needs a name" } }),
    } as unknown as Response);
    vi.stubGlobal("fetch", f);

    const res = await replyToPlatformTicket({
      tenantId: TENANT, ticketId: "t1", content: "hi", authorName: "",
    });

    expect(res.ok).toBe(false);
    if (!res.ok) {
      // The envelope nests error as an OBJECT; the old API returned a string.
      // Reading the old shape loses every message the API took care to write.
      expect(res.error.message).toContain("merchant author needs a name");
    }
  });
});
