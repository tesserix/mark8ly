import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createSupportClient, SupportError } from "../client";

interface Captured {
  url: string;
  method: string;
  headers: Record<string, string>;
  body?: string;
}

let calls: Captured[] = [];

function mockOnce(status: number, json: unknown) {
  (globalThis.fetch as any).mockImplementationOnce(async (url: string, init: RequestInit) => {
    calls.push({
      url,
      method: init.method ?? "GET",
      headers: (init.headers as Record<string, string>) ?? {},
      body: init.body as string | undefined,
    });
    return new Response(JSON.stringify(json), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  });
}

function newClient(overrides: Partial<Parameters<typeof createSupportClient>[0]> = {}) {
  return createSupportClient({
    baseUrl: "https://api.test",
    basePath: "/api/v1/mobile/storefront/stores/acme/support",
    getToken: async () => "tok-1",
    ...overrides,
  });
}

beforeEach(() => {
  calls = [];
  vi.stubGlobal("fetch", vi.fn());
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createSupportClient", () => {
  it("creates a conversation, sends bearer + body, captures session_token", async () => {
    const saved: string[] = [];
    const client = newClient({ saveSessionToken: (t) => void saved.push(t) });
    mockOnce(201, {
      conversation: { id: "c1", status: "pending" },
      first_message: { id: "m1", sender_type: "customer", body: "hi", created_at: "2026-06-21T10:00:00Z" },
      session_token: "sess-xyz",
    });

    const out = await client.createConversation({ message: "hi", reason: "order_issue", statusInfo: "late" });

    expect(out.conversation.id).toBe("c1");
    expect(out.firstMessage.id).toBe("m1");
    expect(calls[0].method).toBe("POST");
    expect(calls[0].url).toBe("https://api.test/api/v1/mobile/storefront/stores/acme/support/conversations");
    expect(calls[0].headers["Authorization"]).toBe("Bearer tok-1");
    expect(JSON.parse(calls[0].body!)).toMatchObject({ message: "hi", reason: "order_issue", status_info: "late" });
    expect(client.currentSessionToken()).toBe("sess-xyz");
    expect(saved).toEqual(["sess-xyz"]);
  });

  it("sends X-Otto-Session on calls after the session is established", async () => {
    const client = newClient();
    mockOnce(201, { conversation: { id: "c1", status: "pending" }, first_message: { id: "m1", sender_type: "customer", body: "hi", created_at: "2026-06-21T10:00:00Z" }, session_token: "sess-xyz" });
    await client.createConversation({ message: "hi", reason: "other", statusInfo: "x" });

    mockOnce(201, { message: { id: "m2", sender_type: "customer", body: "again", created_at: "2026-06-21T10:01:00Z" } });
    await client.postMessage("c1", "again");

    expect(calls[1].headers["X-Otto-Session"]).toBe("sess-xyz");
    expect(calls[1].url).toBe("https://api.test/api/v1/mobile/storefront/stores/acme/support/conversations/c1/messages");
  });

  it("refreshes the token once on 401 then retries", async () => {
    const refreshToken = vi.fn(async () => "tok-2");
    const client = newClient({ refreshToken });
    mockOnce(401, { error: "unauthorized" });
    mockOnce(200, { messages: [] });

    const msgs = await client.listMessages("c1");
    expect(msgs).toEqual([]);
    expect(refreshToken).toHaveBeenCalledTimes(1);
    expect(calls[1].headers["Authorization"]).toBe("Bearer tok-2");
  });

  it("throws SupportError + calls onUnauthorized when refresh still 401s", async () => {
    const onUnauthorized = vi.fn();
    const client = newClient({ refreshToken: async () => "tok-2", onUnauthorized });
    mockOnce(401, { error: "unauthorized" });
    mockOnce(401, { error: "unauthorized" });

    await expect(client.listMessages("c1")).rejects.toBeInstanceOf(SupportError);
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
  });

  it("maps a non-OK response to SupportError with otto's code", async () => {
    const client = newClient();
    mockOnce(409, { error: "thread_closed", message: "closed" });
    await expect(client.postMessage("c1", "hi")).rejects.toMatchObject({ status: 409, code: "thread_closed" });
  });

  it("resume returns null when there is no open thread", async () => {
    const client = newClient();
    mockOnce(200, { conversation: null });
    expect(await client.resume()).toBeNull();
  });

  it("loads a persisted session token and sends it on the first call", async () => {
    const client = newClient({ loadSessionToken: () => "persisted-sess" });
    mockOnce(200, { conversation: { id: "c1", status: "active" }, messages: [] });
    await client.resume();
    expect(calls[0].headers["X-Otto-Session"]).toBe("persisted-sess");
  });

  it("builds the ws url with the ticket query param", () => {
    const client = newClient();
    expect(
      client.buildWsUrl({ ticket: "tk t/1", ws_url: "wss://api.test/api/v1/storefront/otto/conversations/c1/ws" }),
    ).toBe("wss://api.test/api/v1/storefront/otto/conversations/c1/ws?ticket=tk%20t%2F1");
  });
});
