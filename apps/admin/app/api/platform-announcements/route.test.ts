import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api/platform-token", () => ({
  getPlatformToken: vi.fn(async () => "machine-token"),
}));

import { GET } from "./route";

// The banner's proxy, repointed from apps/web's shared-secret internal route
// to the platform API (tesserix-home#152).
//
// The BROWSER contract is deliberately unchanged — the banner still reads
// `{ rows }` — so the repoint is invisible on that side. What changes is where
// the rows come from and how this pod authenticates for them.

function ok(body: unknown) {
  return { ok: true, status: 200, json: async () => body } as unknown as Response;
}

beforeEach(() => {
  process.env.TESSERIX_PLATFORM_API_URL = "http://platform-api.tesserix.svc.cluster.local";
});
afterEach(() => {
  vi.unstubAllGlobals();
  delete process.env.TESSERIX_PLATFORM_API_URL;
});

it("calls the platform API with a machine token", async () => {
  const f = vi.fn().mockResolvedValue(ok({ data: { announcements: [] } }));
  vi.stubGlobal("fetch", f);

  await GET(new Request("http://localhost/api/platform-announcements") as never);

  const [url, init] = f.mock.calls[0];
  const u = new URL(String(url));
  expect(u.host).toBe("platform-api.tesserix.svc.cluster.local");
  expect(u.pathname).toBe("/v1/announcements");
  expect(new Headers((init as RequestInit).headers).get("authorization")).toBe("Bearer machine-token");
  // The shared secret must not travel to a service that ignores it.
  expect(new Headers((init as RequestInit).headers).get("x-internal-token")).toBeNull();
});

it("sends no product, because the API takes it from the token's scope", async () => {
  const f = vi.fn().mockResolvedValue(ok({ data: { announcements: [] } }));
  vi.stubGlobal("fetch", f);

  await GET(new Request("http://localhost/api/platform-announcements") as never);

  const u = new URL(String(f.mock.calls[0][0]));
  // A product parameter is now a 400 at the API — it would break the banner
  // rather than be ignored.
  expect(u.searchParams.get("product")).toBeNull();
  expect(u.searchParams.get("tenant_status")).toBe("active");
});

it("keeps the browser contract: data.announcements is returned as rows", async () => {
  const f = vi.fn().mockResolvedValue(
    ok({ data: { announcements: [{ id: "a1", title: "t", body: "b", severity: "info" }] } }),
  );
  vi.stubGlobal("fetch", f);

  const res = await GET(new Request("http://localhost/api/platform-announcements") as never);
  const body = (await res.json()) as { rows: unknown[] };

  // The banner reads `rows`. Returning the envelope through unchanged would
  // render as "no announcements" rather than as an error.
  expect(body.rows).toHaveLength(1);
});

it("degrades to an empty banner rather than failing the page", async () => {
  // A banner is decoration on someone's admin dashboard. It must never be the
  // reason the page errors — which is why every failure path answers [].
  for (const outcome of [
    { ok: false, status: 503, json: async () => ({}) } as unknown as Response,
    Promise.reject(new Error("network")) as unknown as Response,
  ]) {
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => Promise.resolve(outcome).catch(() => Promise.reject(new Error("network")))));
    const res = await GET(new Request("http://localhost/api/platform-announcements") as never);
    expect((await res.json()).rows).toEqual([]);
  }
});

it("answers [] when the token cannot be minted", async () => {
  const { getPlatformToken } = await import("@/lib/api/platform-token");
  vi.mocked(getPlatformToken).mockRejectedValueOnce(new Error("no creds"));
  vi.stubGlobal("fetch", vi.fn());

  const res = await GET(new Request("http://localhost/api/platform-announcements") as never);
  expect((await res.json()).rows).toEqual([]);
});
