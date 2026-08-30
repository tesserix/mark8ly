import { afterEach, describe, expect, it, vi } from "vitest";

import { proxyJson } from "./_proxy";

// A 204/205/304 must not be given a body: `new Response(body, {status: 204})`
// throws, the throw lands in proxyJson's catch, and the caller is told
// `upstream_unreachable` for a request that SUCCEEDED — with the side effect
// already applied upstream.
//
// Found live rather than in review: DELETE /cart/holds (#232) is the first
// endpoint behind this proxy that answers 204. It released the hold in the
// database and answered the browser 502.

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

function upstream(status: number, body = "") {
  globalThis.fetch = vi.fn().mockResolvedValue({
    status,
    headers: new Headers({ "Content-Type": "application/json" }),
    text: async () => body,
  }) as unknown as typeof fetch;
}

describe("proxyJson", () => {
  it("passes a 204 through without inventing a body", async () => {
    upstream(204);

    const res = await proxyJson("https://example.test/thing", { method: "DELETE" });

    expect(res.status).toBe(204);
    expect(await res.text()).toBe("");
  });

  it("does not report a successful 204 as unreachable", async () => {
    upstream(204);

    const res = await proxyJson("https://example.test/thing", { method: "DELETE" });

    expect(res.status).not.toBe(502);
  });

  it("still forwards ordinary JSON responses", async () => {
    upstream(200, '{"data":{"ok":true}}');

    const res = await proxyJson("https://example.test/thing");

    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ data: { ok: true } });
  });

  it("reports a genuine network failure as 502", async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error("boom")) as unknown as typeof fetch;

    const res = await proxyJson("https://example.test/thing");

    expect(res.status).toBe(502);
  });
});
