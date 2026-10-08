import { describe, expect, it, vi } from "vitest";

import type { UploadApi } from "./personalisation-upload";
import {
  UploadError,
  deletePersonalisationUpload,
  uploadPersonalisationImage,
} from "./personalisation-upload";

const file = {
  uri: "file:///tmp/photo.jpg",
  name: "photo.jpg",
  type: "image/jpeg",
  width: 2000,
  height: 1500,
};
const input = { productId: "p1", fieldId: "f1", cartToken: "cart-1", file };

function fakeApi(
  handlers: { post?: (path: string, body: unknown) => unknown } = {},
) {
  const calls: { method: string; path: string; body?: unknown }[] = [];
  const api: UploadApi = {
    post: vi.fn(async (path: string, body?: unknown) => {
      calls.push({ method: "POST", path, body });
      if (handlers.post) return handlers.post(path, body);
      if (path.endsWith("/upload-url"))
        return { upload_id: "u1", url: "https://gcs.example/signed" };
      return { id: "u1", state: "verified" };
    }) as UploadApi["post"],
    delete: vi.fn(async (path: string) => {
      calls.push({ method: "DELETE", path });
      return undefined;
    }) as UploadApi["delete"],
  };
  return { api, calls };
}

describe("uploadPersonalisationImage", () => {
  it("signs, PUTs to the signed URL, then confirms — with the cart token on both calls", async () => {
    const { api, calls } = fakeApi();
    const putFile = vi.fn(async () => ({ ok: true, status: 200 }));

    const res = await uploadPersonalisationImage(api, input, { putFile });

    expect(res.uploadId).toBe("u1");
    expect(res.dimensions).toEqual({ width: 2000, height: 1500 });
    expect(putFile).toHaveBeenCalledWith("https://gcs.example/signed", file);
    expect(calls.map((c) => c.path)).toEqual([
      "/personalisation/upload-url",
      "/personalisation/uploads/u1/confirm",
    ]);
    // No cookie on a native client: the token is the body, or there is no cart.
    expect(calls[0]!.body).toEqual({
      product_id: "p1",
      field_id: "f1",
      filename: "photo.jpg",
      content_type: "image/jpeg",
      cart_token: "cart-1",
    });
    expect(calls[1]!.body).toEqual({
      cart_token: "cart-1",
      width_px: 2000,
      height_px: 1500,
    });
  });

  it("omits dimensions the picker could not supply, and still uploads", async () => {
    const { api, calls } = fakeApi();
    const res = await uploadPersonalisationImage(
      api,
      { ...input, file: { ...file, width: 0, height: 0 } },
      { putFile: async () => ({ ok: true, status: 200 }) },
    );
    expect(res.dimensions).toBeNull();
    expect(calls[1]!.body).toEqual({ cart_token: "cart-1" });
  });

  it("surfaces the server's own message when signing is refused", async () => {
    const { api } = fakeApi({
      post: () => {
        throw new Error("HEIC photos need converting first");
      },
    });
    await expect(
      uploadPersonalisationImage(api, input, {
        putFile: async () => ({ ok: true, status: 200 }),
      }),
    ).rejects.toMatchObject({
      name: "UploadError",
      step: "sign",
      message: "HEIC photos need converting first",
    });
  });

  it("fails on a bad PUT without confirming or cleaning up", async () => {
    const { api, calls } = fakeApi();
    const err = await uploadPersonalisationImage(api, input, {
      putFile: async () => ({ ok: false, status: 403 }),
    }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(UploadError);
    expect((err as UploadError).step).toBe("put");
    // One call: the sign. No confirm, and no DELETE — the sweeper owns orphans.
    expect(calls).toHaveLength(1);
  });

  it("carries the confirm step's message, where size and type are actually enforced", async () => {
    const { api } = fakeApi({
      post: (path) => {
        if (path.endsWith("/upload-url"))
          return { upload_id: "u1", url: "https://gcs.example/s" };
        throw new Error("that image is too large, the limit is 15 MB");
      },
    });
    await expect(
      uploadPersonalisationImage(api, input, {
        putFile: async () => ({ ok: true, status: 200 }),
      }),
    ).rejects.toMatchObject({
      step: "confirm",
      message: "that image is too large, the limit is 15 MB",
    });
  });

  it("treats a malformed sign response as a sign failure", async () => {
    const { api } = fakeApi({ post: () => ({}) });
    await expect(
      uploadPersonalisationImage(api, input, {
        putFile: async () => ({ ok: true, status: 200 }),
      }),
    ).rejects.toMatchObject({ step: "sign" });
  });
});

describe("deletePersonalisationUpload", () => {
  it("sends the cart token in the query, as the server reads it there", async () => {
    const { api, calls } = fakeApi();
    await deletePersonalisationUpload(api, "u 1", "cart-1");
    expect(calls[0]).toEqual({
      method: "DELETE",
      path: "/personalisation/uploads/u%201?cart_token=cart-1",
    });
  });

  it("never throws", async () => {
    const api: UploadApi = {
      post: async () => undefined as never,
      delete: async () => {
        throw new Error("network");
      },
    };
    await expect(
      deletePersonalisationUpload(api, "u1", "c"),
    ).resolves.toBeUndefined();
  });
});
