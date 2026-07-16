import { computeContentHash, inferContentType } from "@/lib/media-upload";

describe("computeContentHash", () => {
  it("is at least 16 chars and at most 128 (backend: required,min=16,max=128)", () => {
    const hash = computeContentHash({ fileName: "photo.jpg", fileSize: 12345 }, 1_700_000_000_000);
    expect(hash.length).toBeGreaterThanOrEqual(16);
    expect(hash.length).toBeLessThanOrEqual(128);
  });

  it("is alphanumeric-only", () => {
    const hash = computeContentHash({ fileName: "my photo (1).jpg", fileSize: 999 }, 42);
    expect(hash).toMatch(/^[a-zA-Z0-9]+$/);
  });

  it("is stable for the same input", () => {
    const asset = { fileName: "a.png", fileSize: 100 };
    const first = computeContentHash(asset, 123);
    const second = computeContentHash(asset, 123);
    expect(first).toBe(second);
  });

  it("differs when the timestamp differs", () => {
    const asset = { fileName: "a.png", fileSize: 100 };
    expect(computeContentHash(asset, 1)).not.toBe(computeContentHash(asset, 2));
  });

  it("tolerates a missing fileName", () => {
    const hash = computeContentHash({ fileSize: 100 }, 1);
    expect(hash.length).toBeGreaterThanOrEqual(16);
  });

  it("tolerates a missing fileSize", () => {
    const hash = computeContentHash({ fileName: "a.png" }, 1);
    expect(hash.length).toBeGreaterThanOrEqual(16);
  });

  it("tolerates both fileName and fileSize missing", () => {
    const hash = computeContentHash({}, 1);
    expect(hash.length).toBeGreaterThanOrEqual(16);
    expect(hash).toMatch(/^[a-zA-Z0-9]+$/);
  });

  it("tolerates a null fileName (ImagePicker's actual type)", () => {
    const hash = computeContentHash({ fileName: null, fileSize: 100 }, 1);
    expect(hash.length).toBeGreaterThanOrEqual(16);
  });
});

describe("inferContentType", () => {
  it("recognizes image/png", () => {
    expect(inferContentType("image/png")).toBe("image/png");
  });

  it("recognizes image/webp", () => {
    expect(inferContentType("image/webp")).toBe("image/webp");
  });

  it("recognizes image/jpeg", () => {
    expect(inferContentType("image/jpeg")).toBe("image/jpeg");
  });

  it("falls back to image/jpeg for an unrecognized mime type", () => {
    expect(inferContentType("image/heic")).toBe("image/jpeg");
  });

  it("falls back to image/jpeg when mimeType is missing", () => {
    expect(inferContentType(undefined)).toBe("image/jpeg");
  });

  it("falls back to image/jpeg when mimeType is null", () => {
    expect(inferContentType(null)).toBe("image/jpeg");
  });
});
