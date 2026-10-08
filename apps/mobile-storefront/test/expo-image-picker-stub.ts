// Import-resolution stub for vitest. Not reached by any test: the picker
// module takes `launch` as an injected dependency.
export async function launchImageLibraryAsync(): Promise<never> {
  throw new Error(
    "expo-image-picker is not available under vitest; inject `launch`",
  );
}
