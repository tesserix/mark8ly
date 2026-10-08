// Import-resolution stub for vitest. Not reached by any test: the picker
// module takes `convert` as an injected dependency.
export enum SaveFormat {
  JPEG = "jpeg",
  PNG = "png",
  WEBP = "webp",
}
export async function manipulateAsync(): Promise<never> {
  throw new Error(
    "expo-image-manipulator is not available under vitest; inject `convert`",
  );
}
