import { describe, it, expect, vi, beforeEach } from "vitest";
import { cropToBlob, type CropBox } from "./cropImage";

describe("cropToBlob", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("rounds fractional crop box coords and forwards them to drawImage", async () => {
    const drawImage = vi.fn();
    const translate = vi.fn();
    const rotate = vi.fn();
    const toBlob = vi.fn((cb: (b: Blob) => void) => {
      cb(new Blob(["x"], { type: "image/jpeg" }));
    });

    const fakeCtx = { drawImage, translate, rotate } as unknown as CanvasRenderingContext2D;
    const fakeCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => fakeCtx),
      toBlob,
    } as unknown as HTMLCanvasElement;

    vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
      if (tag === "canvas") return fakeCanvas;
      return {} as HTMLElement;
    }) as typeof document.createElement);

    const box: CropBox = { x: 100.4, y: 200.6, width: 300.5, height: 400.5 };
    const fakeImage = {} as HTMLImageElement;

    const blob = await cropToBlob(fakeImage, box, 0);

    expect(blob).toBeInstanceOf(Blob);
    expect(drawImage).toHaveBeenCalledTimes(1);
    expect(drawImage).toHaveBeenCalledWith(fakeImage, 100, 201, 301, 401, 0, 0, 301, 401);
    expect(fakeCanvas.width).toBe(301);
    expect(fakeCanvas.height).toBe(401);
  });
});
