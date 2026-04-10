import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { uploadMediaFile, type PutFn } from "./mediaUploadClient";

describe("uploadMediaFile", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;
  let putSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    putSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("happy path: request signed url -> PUT -> finalize", async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/abc",
          storage_key: "stores/s1/products/p1/abc.jpg",
          expires_at: new Date(Date.now() + 3600 * 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          id: "media-1",
          url: "https://cdn.example/abc.jpg",
          alt: "",
          position: 0,
          variant_id: null,
          storage_key: "stores/s1/products/p1/abc.jpg",
          media_type: "image",
          width: null,
          height: null,
          bytes: null,
        }),
      });
    putSpy.mockResolvedValueOnce({ ok: true, status: 200 });

    const file = new File(["hello"], "hello.jpg", { type: "image/jpeg" });
    const result = await uploadMediaFile({
      storeId: "s1",
      productId: "p1",
      file,
      position: 0,
      onProgress: vi.fn(),
      putFn: putSpy as unknown as PutFn,
    });

    expect(result.id).toBe("media-1");
    expect(fetchSpy).toHaveBeenCalledTimes(2);
    expect(putSpy).toHaveBeenCalledTimes(1);
  });

  it("retries PUT once on 403 with a fresh signed URL", async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/stale",
          storage_key: "stores/s1/products/p1/stale.jpg",
          expires_at: new Date(Date.now() - 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/fresh",
          storage_key: "stores/s1/products/p1/fresh.jpg",
          expires_at: new Date(Date.now() + 3600 * 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          id: "media-2",
          url: "https://cdn.example/fresh.jpg",
          alt: "",
          position: 0,
          variant_id: null,
          storage_key: "stores/s1/products/p1/fresh.jpg",
          media_type: "image",
          width: null,
          height: null,
          bytes: null,
        }),
      });
    putSpy
      .mockResolvedValueOnce({ ok: false, status: 403 })
      .mockResolvedValueOnce({ ok: true, status: 200 });

    const file = new File(["x"], "x.jpg", { type: "image/jpeg" });
    const result = await uploadMediaFile({
      storeId: "s1",
      productId: "p1",
      file,
      position: 0,
      onProgress: vi.fn(),
      putFn: putSpy as unknown as PutFn,
    });

    expect(result.id).toBe("media-2");
    expect(putSpy).toHaveBeenCalledTimes(2);
    expect(fetchSpy).toHaveBeenCalledTimes(3);
  });

  it("surfaces error after second PUT also fails", async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/a",
          storage_key: "a",
          expires_at: new Date(Date.now() + 1000).toISOString(),
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          upload_url: "https://gcs.example/upload/b",
          storage_key: "b",
          expires_at: new Date(Date.now() + 1000).toISOString(),
        }),
      });
    putSpy
      .mockResolvedValueOnce({ ok: false, status: 403 })
      .mockResolvedValueOnce({ ok: false, status: 403 });

    const file = new File(["x"], "x.jpg", { type: "image/jpeg" });
    await expect(
      uploadMediaFile({
        storeId: "s1",
        productId: "p1",
        file,
        position: 0,
        onProgress: vi.fn(),
        putFn: putSpy as unknown as PutFn,
      }),
    ).rejects.toThrow(/upload failed/i);
  });

  it("throws if signed url request fails (!res.ok)", async () => {
    fetchSpy.mockResolvedValueOnce({ ok: false, status: 500, json: async () => ({}) });
    const file = new File(["x"], "x.jpg", { type: "image/jpeg" });
    await expect(
      uploadMediaFile({
        storeId: "s1",
        productId: "p1",
        file,
        position: 0,
        putFn: putSpy as unknown as PutFn,
      }),
    ).rejects.toThrow(/signed url request failed: 500/);
  });

  it("throws immediately on non-403 PUT failure", async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        upload_url: "https://gcs.example/upload/a",
        storage_key: "a",
        expires_at: "2099",
      }),
    });
    putSpy.mockResolvedValueOnce({ ok: false, status: 500 });
    const file = new File(["x"], "x.png", { type: "image/png" });
    await expect(
      uploadMediaFile({
        storeId: "s1",
        productId: "p1",
        file,
        position: 0,
        putFn: putSpy as unknown as PutFn,
      }),
    ).rejects.toThrow(/PUT returned 500/);
    expect(putSpy).toHaveBeenCalledTimes(1);
  });

  it("sends alt + variantId and webp type on finalize, and surfaces finalize error", async () => {
    // First: signed-url ok, PUT ok, finalize fails
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ url: "https://gcs/u", storage_key: "sk", expires_at: "2099" }),
      })
      .mockResolvedValueOnce({ ok: false, status: 422, json: async () => ({}) });
    putSpy.mockResolvedValueOnce({ ok: true, status: 200 });
    const file = new File(["x"], "x.webp", { type: "image/webp" });
    await expect(
      uploadMediaFile({
        storeId: "s1",
        productId: "p1",
        file,
        position: 2,
        alt: "hi",
        variantId: "var_1",
        putFn: putSpy as unknown as PutFn,
      }),
    ).rejects.toThrow(/finalize media failed: 422/);
    // Check finalize body contains alt + variant_id
    const finalizeCall = fetchSpy.mock.calls[1]!;
    const body = JSON.parse(finalizeCall[1].body as string);
    expect(body.alt).toBe("hi");
    expect(body.variant_id).toBe("var_1");
    expect(body.position).toBe(2);
    // Signed-url request body should have webp content_type
    const urlCallBody = JSON.parse(fetchSpy.mock.calls[0]![1].body as string);
    expect(urlCallBody.content_type).toBe("image/webp");
  });
});
