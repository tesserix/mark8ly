import { describe, it, expect, vi } from "vitest";
import {
  MIN_SHORT_EDGE_PX,
  belowMinShortEdge,
  inferMimeFromUrl,
  readFileDimensions,
} from "./mediaResolution";

describe("belowMinShortEdge", () => {
  it("is false when the short edge meets the threshold", () => {
    expect(belowMinShortEdge(4000, MIN_SHORT_EDGE_PX)).toBe(false);
    expect(belowMinShortEdge(MIN_SHORT_EDGE_PX, MIN_SHORT_EDGE_PX)).toBe(false);
  });

  it("is true when the short edge is under the threshold", () => {
    expect(belowMinShortEdge(4000, 800)).toBe(true);
    expect(belowMinShortEdge(600, 3000)).toBe(true);
  });
});

describe("inferMimeFromUrl", () => {
  it("maps known extensions and defaults to jpeg", () => {
    expect(inferMimeFromUrl("https://cdn/x.png")).toBe("image/png");
    expect(inferMimeFromUrl("https://cdn/x.PNG?v=2")).toBe("image/png");
    expect(inferMimeFromUrl("https://cdn/x.webp")).toBe("image/webp");
    expect(inferMimeFromUrl("https://cdn/x.jpg")).toBe("image/jpeg");
    expect(inferMimeFromUrl("https://cdn/x")).toBe("image/jpeg");
  });
});

describe("readFileDimensions", () => {
  it("resolves natural dimensions from a loaded image", async () => {
    const created: { onload?: () => void; onerror?: () => void; naturalWidth: number; naturalHeight: number; src: string } = {
      naturalWidth: 2400,
      naturalHeight: 1600,
      src: "",
    };
    const origImage = globalThis.Image;
    (globalThis as unknown as { Image: unknown }).Image = function () {
      return created;
    };
    const origCreate = URL.createObjectURL;
    const origRevoke = URL.revokeObjectURL;
    URL.createObjectURL = vi.fn(() => "blob:x");
    URL.revokeObjectURL = vi.fn();

    const file = new File(["x"], "a.jpg", { type: "image/jpeg" });
    const p = readFileDimensions(file);
    created.onload?.();
    await expect(p).resolves.toEqual({ width: 2400, height: 1600 });

    (globalThis as unknown as { Image: unknown }).Image = origImage;
    URL.createObjectURL = origCreate;
    URL.revokeObjectURL = origRevoke;
  });
});
