// Firebase-free error types for the auth layer. Kept separate from
// `link.ts` (which does a value import of `@react-native-firebase/auth`)
// so app-layer code that only needs to catch `LastSignInMethodError` never
// pulls the native module chain into its import graph — important for
// expo-router route files, which are required at boot even in Expo Go.

/** Thrown when unlinking would remove the user's only remaining sign-in method. */
export class LastSignInMethodError extends Error {
  constructor() {
    super("Cannot remove the only sign-in method");
    this.name = "LastSignInMethodError";
  }
}
