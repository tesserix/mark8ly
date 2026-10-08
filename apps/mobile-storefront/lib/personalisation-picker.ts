// Choosing a photo for a personalisation field (#969).
//
// HEIC is the whole reason this file exists. Every photo an iPhone takes
// is HEIC unless something converts it, the server's v1 allow-list is
// JPEG/PNG/WebP only, and web REJECTS a HEIC with a message. A native app
// can do better: it has the image in hand and an encoder, so it converts
// on pick and the buyer never learns the word.
//
// The picker is the system one (PHPicker on iOS). It runs out of process
// and needs no library permission, so none is requested — asking anyway
// opts into the legacy flow where "Limited Access" drops the buyer into a
// management sheet the real picker never opens from (observed on
// mobile-admin, pinned there by a regression test).
//
// Dependencies are injected so the decision logic is testable without
// either native module.

import * as ImagePicker from "expo-image-picker";
import { manipulateAsync, SaveFormat } from "expo-image-manipulator";

import type { PickedImage } from "@/lib/personalisation-upload";

/** What the server will sign for. Mirrors personalisationupload.Service. */
export const ACCEPTED_TYPES: readonly string[] = [
  "image/jpeg",
  "image/png",
  "image/webp",
];

/** JPEG quality for a converted photo. High: this is the print source. */
export const CONVERT_QUALITY = 0.92;

/** The slice of an ImagePickerAsset the decision needs. */
export interface PickedAsset {
  uri: string;
  width: number;
  height: number;
  mimeType?: string | null;
  fileName?: string | null;
}

export interface PickerDeps {
  /** Opens the library. Null when the buyer cancelled. */
  launch?: () => Promise<PickedAsset | null>;
  /** Re-encodes the image at `uri` as JPEG. */
  convert?: (
    uri: string,
  ) => Promise<{ uri: string; width: number; height: number }>;
}

const EXT_TYPES: Record<string, string> = {
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  webp: "image/webp",
};

/**
 * The type the server would see, or null when the asset is not something
 * it accepts. Trusts the picker's mimeType first; falls back to the
 * extension, because older Android pickers report no type at all and a
 * missing type is not evidence of a HEIC.
 */
export function acceptedType(
  asset: Pick<PickedAsset, "mimeType" | "fileName">,
): string | null {
  const mime = asset.mimeType?.toLowerCase();
  if (mime) return ACCEPTED_TYPES.includes(mime) ? mime : null;
  const ext = asset.fileName?.split(".").pop()?.toLowerCase();
  return (ext && EXT_TYPES[ext]) ?? null;
}

/** True when the asset must be re-encoded before the server will take it. */
export function needsConversion(
  asset: Pick<PickedAsset, "mimeType" | "fileName">,
): boolean {
  return acceptedType(asset) === null;
}

/**
 * The filename sent on upload-url. The buyer's own name where there is
 * one — it prints on the packing slip, and "IMG_4021.jpg" is how a
 * merchant matches a file to a conversation. A converted photo keeps the
 * stem and takes .jpg, because the bytes are now JPEG whatever the name
 * said.
 */
export function filenameFor(
  asset: Pick<PickedAsset, "fileName">,
  type: string,
): string {
  const original = asset.fileName?.trim();
  const stem = original ? original.replace(/\.[^.]+$/, "") : "photo";
  const ext =
    type === "image/png" ? "png" : type === "image/webp" ? "webp" : "jpg";
  return `${stem || "photo"}.${ext}`;
}

async function defaultLaunch(): Promise<PickedAsset | null> {
  const result = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ["images"],
    quality: 1,
    exif: false,
    allowsMultipleSelection: false,
  });
  if (result.canceled) return null;
  const asset = result.assets[0];
  if (!asset) return null;
  return {
    uri: asset.uri,
    width: asset.width,
    height: asset.height,
    mimeType: asset.mimeType,
    fileName: asset.fileName,
  };
}

async function defaultConvert(uri: string) {
  // No actions: a straight re-encode at the image's own size. Cropping
  // and resizing are the server's crop step (#966) and the merchant's
  // print pipeline respectively; this must not lose pixels.
  const out = await manipulateAsync(uri, [], {
    compress: CONVERT_QUALITY,
    format: SaveFormat.JPEG,
  });
  return { uri: out.uri, width: out.width, height: out.height };
}

/**
 * Opens the library and returns an image the server will accept, or null
 * when the buyer cancelled. Throws only when the picker or the encoder
 * does — a caller must surface that, because a swallowed error here is
 * a picker that "does nothing".
 */
export async function pickPersonalisationImage(
  deps: PickerDeps = {},
): Promise<PickedImage | null> {
  const launch = deps.launch ?? defaultLaunch;
  const convert = deps.convert ?? defaultConvert;

  const asset = await launch();
  if (!asset) return null;

  const type = acceptedType(asset);
  if (type) {
    return {
      uri: asset.uri,
      name: filenameFor(asset, type),
      type,
      width: asset.width,
      height: asset.height,
    };
  }

  const converted = await convert(asset.uri);
  return {
    uri: converted.uri,
    name: filenameFor(asset, "image/jpeg"),
    type: "image/jpeg",
    width: converted.width,
    height: converted.height,
  };
}
