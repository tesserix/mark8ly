import { describe, it, expect, vi } from "vitest";

import {
  UploadError,
  uploadPersonalisationImage,
} from "./personalisation-upload";

function file(name = "photo.jpg", type = "image/jpeg"): File {
  return { name, type, size: 1234 } as unknown as File;
}

function jsonResponse(body: unknown, ok = true, status = 200): Response {
  return {
    ok,
    status,
    json: async () => body,
  } as unknown as Response;
}

const input = { productId: "p1", fieldId: "f1", file: file() };

describe("uploadPersonalisationImage", () => {
  it("signs, PUTs to the signed URL, then confirms", async () => {
    const calls: string[] = [];
    const fetchJson = vi.fn(async (url: string) => {
      calls.push(String(url));
      if (String(url).includes("upload-url")) {
        return jsonResponse({ upload_id: "u1", url: "https://gcs.example/signed" });
      }
      return jsonResponse({ id: "u1", state: "verified" });
    }) as unknown as typeof fetch;

    const putBlob = vi.fn(async () => ({ ok: true, status: 200 }));
    const measure = vi.fn(async () => ({ width: 2000, height: 1500 }));

    const res = await uploadPersonalisationImage("shop", input, { fetchJson, putBlob, measure });

    expect(res.uploadId).toBe("u1");
    expect(res.dimensions).toEqual({ width: 2000, height: 1500 });
    // The bytes go straight to Google, not through this origin.
    expect(putBlob).toHaveBeenCalledWith("https://gcs.example/signed", input.file);
    expect(calls[0]).toContain("/api/personalisation/upload-url");
    expect(calls[1]).toContain("/confirm");
  });

  it("sends the measured dimensions to confirm, because they drive the warning", async () => {
    let confirmBody: unknown;
    const fetchJson = vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).includes("upload-url")) {
        return jsonResponse({ upload_id: "u1", url: "https://gcs.example/s" });
      }
      confirmBody = JSON.parse(String(init?.body));
      return jsonResponse({});
    }) as unknown as typeof fetch;

    await uploadPersonalisationImage("shop", input, {
      fetchJson,
      putBlob: async () => ({ ok: true, status: 200 }),
      measure: async () => ({ width: 800, height: 600 }),
    });
    expect(confirmBody).toEqual({ width_px: 800, height_px: 600 });
  });

  it("still uploads when the browser cannot measure the image", async () => {
    // An unreadable image is not evidence of a bad one; the warning just
    // stays quiet.
    let confirmBody: unknown;
    const fetchJson = vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).includes("upload-url")) {
        return jsonResponse({ upload_id: "u1", url: "https://gcs.example/s" });
      }
      confirmBody = JSON.parse(String(init?.body));
      return jsonResponse({});
    }) as unknown as typeof fetch;

    const res = await uploadPersonalisationImage("shop", input, {
      fetchJson,
      putBlob: async () => ({ ok: true, status: 200 }),
      measure: async () => null,
    });
    expect(res.dimensions).toBeNull();
    expect(confirmBody).toEqual({});
  });

  it("surfaces the server's message when confirm rejects the file", async () => {
    // Confirm is where the size cap and content-type check actually live,
    // and "the limit is 15 MB" is the only thing the buyer can act on.
    const fetchJson = vi.fn(async (url: string) => {
      if (String(url).includes("upload-url")) {
        return jsonResponse({ upload_id: "u1", url: "https://gcs.example/s" });
      }
      return jsonResponse(
        { error: "validation_error", message: "that image is too large. The limit is 15 MB." },
        false,
        400,
      );
    }) as unknown as typeof fetch;

    await expect(
      uploadPersonalisationImage("shop", input, {
        fetchJson,
        putBlob: async () => ({ ok: true, status: 200 }),
        measure: async () => null,
      }),
    ).rejects.toThrow(/15 MB/);
  });

  it("names the step that failed", async () => {
    const signFails = vi.fn(async () =>
      jsonResponse({ message: "image uploads are not enabled on this store" }, false, 501),
    ) as unknown as typeof fetch;

    await expect(
      uploadPersonalisationImage("shop", input, { fetchJson: signFails }),
    ).rejects.toMatchObject({ step: "sign" });

    const putFails = vi.fn(async (url: string) =>
      String(url).includes("upload-url")
        ? jsonResponse({ upload_id: "u1", url: "https://gcs.example/s" })
        : jsonResponse({}),
    ) as unknown as typeof fetch;

    await expect(
      uploadPersonalisationImage("shop", input, {
        fetchJson: putFails,
        putBlob: async () => ({ ok: false, status: 403 }),
        measure: async () => null,
      }),
    ).rejects.toMatchObject({ step: "put" });
  });

  it("does not confirm an upload whose PUT failed", async () => {
    // Confirming would ask the server to Verify an object that is not
    // there, and turn a clear "upload failed" into a confusing 404.
    const fetchJson = vi.fn(async (url: string) =>
      String(url).includes("upload-url")
        ? jsonResponse({ upload_id: "u1", url: "https://gcs.example/s" })
        : jsonResponse({}),
    ) as unknown as typeof fetch;

    await expect(
      uploadPersonalisationImage("shop", input, {
        fetchJson,
        putBlob: async () => ({ ok: false, status: 500 }),
        measure: async () => null,
      }),
    ).rejects.toBeInstanceOf(UploadError);

    const confirmed = (fetchJson as unknown as { mock: { calls: unknown[][] } }).mock.calls.filter(
      (c) => String(c[0]).includes("/confirm"),
    );
    expect(confirmed).toHaveLength(0);
  });

  it("refuses a sign response missing the id or the url", async () => {
    const fetchJson = vi.fn(async () => jsonResponse({ url: "https://gcs.example/s" })) as unknown as typeof fetch;
    await expect(
      uploadPersonalisationImage("shop", input, { fetchJson }),
    ).rejects.toMatchObject({ step: "sign" });
  });
});
