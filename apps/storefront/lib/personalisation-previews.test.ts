import { describe, it, expect, vi } from "vitest";

import {
  collectUploadIds,
  emptyPreviewLookup,
  fetchPersonalisationPreviews,
  isExpired,
} from "./personalisation-previews";

// #966. The property that matters most here is a negative one: a
// NETWORK failure must not look like an expired upload.
//
// "Your photo has expired — upload it again" is a destructive thing to
// tell someone. If a transport error marked every id unavailable, a
// flaky connection would tell a buyer their photos were gone and invite
// them to re-upload images that are sitting there perfectly fine.

function okResponse(body: unknown) {
  return { ok: true, status: 200, json: async () => body } as unknown as Response;
}

describe("fetchPersonalisationPreviews", () => {
  it("makes no request for an empty list", async () => {
    const fetchJson = vi.fn();
    const out = await fetchPersonalisationPreviews("acme", [], { fetchJson });
    expect(fetchJson).not.toHaveBeenCalled();
    expect(out).toBe(emptyPreviewLookup);
  });

  it("sends one request for the whole set, deduplicated", async () => {
    const fetchJson = vi.fn().mockResolvedValue(okResponse({ previews: {}, unavailable: [] }));
    await fetchPersonalisationPreviews("acme", ["a", "b", "a", "", "b"], { fetchJson });

    expect(fetchJson).toHaveBeenCalledTimes(1);
    const [, init] = fetchJson.mock.calls[0]!;
    expect(JSON.parse(String(init.body))).toEqual({ upload_ids: ["a", "b"] });
  });

  it("maps signed urls and unavailable ids apart", async () => {
    const fetchJson = vi.fn().mockResolvedValue(
      okResponse({
        previews: { a: { url: "https://signed/a", expires_at: "2026-10-05T00:00:00Z" } },
        unavailable: ["b"],
      }),
    );
    const out = await fetchPersonalisationPreviews("acme", ["a", "b"], { fetchJson });

    expect(out.previews.a?.url).toBe("https://signed/a");
    expect(isExpired(out, "b")).toBe(true);
    expect(isExpired(out, "a")).toBe(false);
  });

  it("drops a preview entry with no url rather than rendering a broken image", async () => {
    const fetchJson = vi
      .fn()
      .mockResolvedValue(okResponse({ previews: { a: { expires_at: "x" } }, unavailable: [] }));
    const out = await fetchPersonalisationPreviews("acme", ["a"], { fetchJson });
    expect(out.previews.a).toBeUndefined();
    expect(isExpired(out, "a")).toBe(false);
  });

  it("does NOT report ids as expired when the request fails", async () => {
    // The whole point. A thrown fetch and a non-ok response both mean
    // "we could not ask", which is not the same as "they are gone".
    const thrown = await fetchPersonalisationPreviews("acme", ["a"], {
      fetchJson: vi.fn().mockRejectedValue(new Error("offline")),
    });
    expect(isExpired(thrown, "a")).toBe(false);
    expect(thrown.previews.a).toBeUndefined();

    const notOk = await fetchPersonalisationPreviews("acme", ["a"], {
      fetchJson: vi.fn().mockResolvedValue({ ok: false, status: 500 } as unknown as Response),
    });
    expect(isExpired(notOk, "a")).toBe(false);
  });

  it("does not cache — the url outlives neither its signature nor the render", async () => {
    const fetchJson = vi.fn().mockResolvedValue(okResponse({ previews: {}, unavailable: [] }));
    await fetchPersonalisationPreviews("acme", ["a"], { fetchJson });
    const [, init] = fetchJson.mock.calls[0]!;
    expect(init.cache).toBe("no-store");
  });
});

describe("collectUploadIds", () => {
  it("gathers every id across lines, once each", () => {
    expect(
      collectUploadIds([
        { personalisation: [{ uploadId: "a" }, { uploadId: "b" }] },
        { personalisation: [{ uploadId: "a" }] },
        { personalisation: [{}] },
        {},
      ]),
    ).toEqual(["a", "b"]);
  });

  it("returns nothing for a cart with no personalisation", () => {
    expect(collectUploadIds([{}, { personalisation: [] }])).toEqual([]);
  });
});
