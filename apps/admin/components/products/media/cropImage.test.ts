import { describe, it, expect, vi, beforeEach } from "vitest";
import { cropToBlob, loadImage, type CropBox } from "./cropImage";

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

  it("applies rotation transforms when rotationDeg != 0", async () => {
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

    await cropToBlob({} as HTMLImageElement, { x: 0, y: 0, width: 10, height: 10 }, 90);
    expect(rotate).toHaveBeenCalledTimes(1);
    expect(translate).toHaveBeenCalledTimes(2);
  });

  it("throws when 2d context is unavailable", async () => {
    const fakeCanvas = {
      getContext: vi.fn(() => null),
    } as unknown as HTMLCanvasElement;
    vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
      if (tag === "canvas") return fakeCanvas;
      return {} as HTMLElement;
    }) as typeof document.createElement);
    await expect(
      cropToBlob({} as HTMLImageElement, { x: 0, y: 0, width: 1, height: 1 }, 0),
    ).rejects.toThrow(/2d context/);
  });

  it("rejects when canvas.toBlob returns null", async () => {
    const fakeCtx = { drawImage: vi.fn(), translate: vi.fn(), rotate: vi.fn() } as unknown as CanvasRenderingContext2D;
    const fakeCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => fakeCtx),
      toBlob: vi.fn((cb: (b: Blob | null) => void) => cb(null)),
    } as unknown as HTMLCanvasElement;
    vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
      if (tag === "canvas") return fakeCanvas;
      return {} as HTMLElement;
    }) as typeof document.createElement);
    await expect(
      cropToBlob({} as HTMLImageElement, { x: 0, y: 0, width: 1, height: 1 }, 0),
    ).rejects.toThrow(/toBlob returned null/);
  });

  it("loadImage resolves on load and rejects on error", async () => {
    // Success
    const okImg = { crossOrigin: "", src: "", onload: null as null | (() => void), onerror: null as null | (() => void) };
    const errImg = { crossOrigin: "", src: "", onload: null as null | (() => void), onerror: null as null | (() => void) };
    const origImage = globalThis.Image;
    let calls = 0;
    (globalThis as unknown as { Image: unknown }).Image = function () {
      calls += 1;
      return calls === 1 ? okImg : errImg;
    };
    const p1 = loadImage("a.jpg");
    okImg.onload?.();
    await expect(p1).resolves.toBe(okImg);

    const p2 = loadImage("b.jpg");
    errImg.onerror?.();
    await expect(p2).rejects.toThrow(/failed to load b.jpg/);
    (globalThis as unknown as { Image: unknown }).Image = origImage;
  });
});
