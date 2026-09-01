import { test, expect } from "@playwright/test";

import { subscribeToJournal } from "../../lib/api/journal-signup";

interface StubbedResponse {
  ok: boolean;
  status?: number;
  json?: unknown;
}

/**
 * Replace global fetch with a queue of canned responses, recording every
 * request. Mirrors the stubFetch helper in gip-signup.spec.ts.
 */
function stubFetch(responses: StubbedResponse[]): { url: string; body: unknown }[] {
  const calls: { url: string; body: unknown }[] = [];
  let i = 0;

  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const next = responses[i++];
    if (!next) throw new Error(`unexpected fetch call #${i}: ${url}`);
    calls.push({ url: String(url), body: JSON.parse(String(init?.body ?? "{}")) });
    return {
      ok: next.ok,
      status: next.status ?? (next.ok ? 200 : 500),
      json: async () => next.json ?? {},
    };
  }) as unknown as typeof fetch;

  return calls;
}

test("posts to the app's own /api/journal-subscribe route, never marketplace-api directly", async () => {
  const calls = stubFetch([{ ok: true, json: { ok: true } }]);

  const result = await subscribeToJournal("ada@example.com");

  expect(result).toEqual({ ok: true });
  expect(calls).toHaveLength(1);
  const [call] = calls;
  expect(call?.url).toBe("/api/journal-subscribe");
  expect(call?.body).toEqual({ email: "ada@example.com" });
});

test("success path: resolves ok:true on a 200 response", async () => {
  stubFetch([{ ok: true, json: { ok: true } }]);

  const result = await subscribeToJournal("ada@example.com");

  expect(result.ok).toBe(true);
});

test("error path: surfaces the server's message on a non-2xx response", async () => {
  stubFetch([
    { ok: false, status: 400, json: { ok: false, message: "That doesn't look like a valid email address." } },
  ]);

  const result = await subscribeToJournal("not-an-email");

  expect(result).toEqual({
    ok: false,
    message: "That doesn't look like a valid email address.",
  });
});

test("error path: falls back to a generic message when the response body has none", async () => {
  stubFetch([{ ok: false, status: 500, json: {} }]);

  const result = await subscribeToJournal("ada@example.com");

  expect(result.ok).toBe(false);
  if (!result.ok) {
    expect(result.message).toBe("Something went wrong. Please try again.");
  }
});

test("error path: a network failure (fetch rejects) never throws, and returns a human message", async () => {
  globalThis.fetch = (async () => {
    throw new Error("network down");
  }) as unknown as typeof fetch;

  const result = await subscribeToJournal("ada@example.com");

  expect(result.ok).toBe(false);
  if (!result.ok) {
    expect(result.message).toContain("couldn't reach the server");
  }
});
