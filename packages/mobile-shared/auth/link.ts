// Completes the one-account-per-email merge on mobile: the user re-authenticates
// with the method their account already has, then the pending provider credential
// is linked onto that account. Mirrors the web admin's link handshake
// (apps/admin/lib/gip/link.ts) using the native SDK instead of REST.

import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

/** Re-auth with the account's existing password, then attach `pending`. */
export async function completeLinkWithPassword(
  email: string,
  password: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const result = await auth().signInWithEmailAndPassword(email, password);
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Google identity, then attach `pending`. */
export async function completeLinkWithGoogle(
  googleIdToken: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = auth.GoogleAuthProvider.credential(googleIdToken);
  const result = await auth().signInWithCredential(existing);
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Apple identity, then attach `pending`. */
export async function completeLinkWithApple(
  appleIdToken: string,
  rawNonce: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = auth.AppleAuthProvider.credential(appleIdToken, rawNonce);
  const result = await auth().signInWithCredential(existing);
  await result.user.linkWithCredential(pending);
}

/**
 * Sign-in methods already registered for `email` — e.g. ["password"],
 * ["google.com"]. Returns [] when the tenant has email-enumeration protection
 * enabled, in which case the caller must ask the user which method they used.
 */
export async function existingSignInMethods(email: string): Promise<string[]> {
  return auth().fetchSignInMethodsForEmail(email);
}
