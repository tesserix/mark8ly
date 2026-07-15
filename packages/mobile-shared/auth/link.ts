// Completes the one-account-per-email merge on mobile: the user re-authenticates
// with the method their account already has, then the pending provider credential
// is linked onto that account. Mirrors the web admin's link handshake
// (apps/admin/lib/gip/link.ts) using the native SDK instead of REST.

import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

// Imported (not just re-exported) so `unlinkProvider` below can throw it —
// `export { X } from "./mod"` alone creates no local binding. Re-exported so
// existing importers of `LastSignInMethodError` from this module keep
// working. The canonical definition lives in `./errors` (firebase-free) so
// app-layer code can catch it without pulling in the native module chain —
// see `./errors` for why that matters.
import { LastSignInMethodError } from "./errors";
export { LastSignInMethodError };

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

function requireCurrentUser(): FirebaseAuthTypes.User {
  const user = auth().currentUser;
  if (!user) throw new Error("Not signed in");
  return user;
}

/** Provider ids on the signed-in user: "password" | "google.com" | "apple.com". */
export async function linkedProviderIds(): Promise<string[]> {
  const user = auth().currentUser;
  if (!user) return [];
  return user.providerData.map((p) => p.providerId);
}

/** Attach Google to the CURRENT user — no re-auth, no email matching. */
export async function linkGoogleToCurrentUser(idToken: string): Promise<void> {
  const user = requireCurrentUser();
  await user.linkWithCredential(auth.GoogleAuthProvider.credential(idToken));
}

/**
 * Attach Apple to the CURRENT user. This is the Apple "Hide My Email" path:
 * because we link to the signed-in user, the relay address Apple may return
 * never has to match anything.
 */
export async function linkAppleToCurrentUser(
  idToken: string,
  rawNonce: string,
): Promise<void> {
  const user = requireCurrentUser();
  await user.linkWithCredential(auth.AppleAuthProvider.credential(idToken, rawNonce));
}

/** Detach a provider. Refuses to remove the last one (would lock the user out). */
export async function unlinkProvider(providerId: string): Promise<void> {
  const user = requireCurrentUser();
  if (user.providerData.length <= 1) throw new LastSignInMethodError();
  await user.unlink(providerId);
}
