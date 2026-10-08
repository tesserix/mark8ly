import { describe, expect, it, vi } from "vitest";

import {
  acceptedType,
  filenameFor,
  needsConversion,
  pickPersonalisationImage,
} from "./personalisation-picker";

describe("acceptedType / needsConversion", () => {
  it("passes the server's three types through untouched", () => {
    expect(acceptedType({ mimeType: "image/jpeg" })).toBe("image/jpeg");
    expect(acceptedType({ mimeType: "IMAGE/PNG" })).toBe("image/png");
    expect(acceptedType({ mimeType: "image/webp" })).toBe("image/webp");
    expect(needsConversion({ mimeType: "image/jpeg" })).toBe(false);
  });

  it("HEIC and HEIF need converting — the whole reason this phase exists", () => {
    expect(
      needsConversion({ mimeType: "image/heic", fileName: "IMG_4021.HEIC" }),
    ).toBe(true);
    expect(needsConversion({ mimeType: "image/heif" })).toBe(true);
    // A HEIC mislabelled with a .jpg name is still a HEIC: mimeType wins.
    expect(
      needsConversion({ mimeType: "image/heic", fileName: "photo.jpg" }),
    ).toBe(true);
  });

  it("falls back to the extension when the picker reports no type", () => {
    expect(acceptedType({ mimeType: null, fileName: "IMG_1.JPG" })).toBe(
      "image/jpeg",
    );
    expect(acceptedType({ fileName: "scan.png" })).toBe("image/png");
    expect(needsConversion({ fileName: "IMG_1.HEIC" })).toBe(true);
    // Nothing to go on at all: convert rather than guess and be refused.
    expect(needsConversion({})).toBe(true);
  });
});

describe("filenameFor", () => {
  it("keeps the buyer's stem and takes the extension of the bytes actually sent", () => {
    expect(filenameFor({ fileName: "IMG_4021.HEIC" }, "image/jpeg")).toBe(
      "IMG_4021.jpg",
    );
    expect(filenameFor({ fileName: "holiday.png" }, "image/png")).toBe(
      "holiday.png",
    );
    expect(filenameFor({ fileName: "a.b.webp" }, "image/webp")).toBe(
      "a.b.webp",
    );
  });
  it("invents a name when the picker gave none", () => {
    expect(filenameFor({ fileName: null }, "image/jpeg")).toBe("photo.jpg");
    expect(filenameFor({ fileName: "  " }, "image/jpeg")).toBe("photo.jpg");
  });
});

describe("pickPersonalisationImage", () => {
  it("returns null when the buyer cancels, without touching the encoder", async () => {
    const convert = vi.fn();
    expect(
      await pickPersonalisationImage({ launch: async () => null, convert }),
    ).toBeNull();
    expect(convert).not.toHaveBeenCalled();
  });

  it("hands back an accepted asset as-is, measured by the picker", async () => {
    const convert = vi.fn();
    const out = await pickPersonalisationImage({
      launch: async () => ({
        uri: "file:///a.jpg",
        width: 3000,
        height: 2000,
        mimeType: "image/jpeg",
        fileName: "a.jpg",
      }),
      convert,
    });
    expect(out).toEqual({
      uri: "file:///a.jpg",
      name: "a.jpg",
      type: "image/jpeg",
      width: 3000,
      height: 2000,
    });
    expect(convert).not.toHaveBeenCalled();
  });

  it("converts a HEIC to JPEG and reports the converted file's size and URI", async () => {
    const convert = vi.fn(async (uri: string) => {
      expect(uri).toBe("file:///IMG_1.HEIC");
      return { uri: "file:///cache/IMG_1.jpg", width: 4032, height: 3024 };
    });
    const out = await pickPersonalisationImage({
      launch: async () => ({
        uri: "file:///IMG_1.HEIC",
        width: 4032,
        height: 3024,
        mimeType: "image/heic",
        fileName: "IMG_1.HEIC",
      }),
      convert,
    });
    expect(out).toEqual({
      uri: "file:///cache/IMG_1.jpg",
      name: "IMG_1.jpg",
      type: "image/jpeg",
      width: 4032,
      height: 3024,
    });
  });

  it("lets an encoder failure propagate — a swallowed one is a picker that does nothing", async () => {
    await expect(
      pickPersonalisationImage({
        launch: async () => ({
          uri: "file:///x.heic",
          width: 1,
          height: 1,
          mimeType: "image/heic",
        }),
        convert: async () => {
          throw new Error("encoder");
        },
      }),
    ).rejects.toThrow("encoder");
  });
});
