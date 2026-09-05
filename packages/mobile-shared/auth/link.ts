// Completes the one-account-per-email merge on mobile: the user re-authenticates
// with the method their account already has, then the pending provider credential
// is linked onto that account. Mirrors the web admin's link handshake
// (apps/admin/lib/gip/link.ts) using the native SDK instead of REST.

import {
  getAuth,
  GoogleAuthProvider,
  AppleAuthProvider,
  FirebaseAuthTypes,
} from "@react-native-firebase/auth";

// Imported (not just re-exported) so `unlinkProvider` below can throw it —
// `export { X } from "./mod"` alone creates no local binding. Re-exported so
// existing importers of `LastSignInMethodError` from this module keep
// working. The canonical definition lives in `./errors` (firebase-free) so
// app-layer code can catch it without pulling in the native module chain —
// see `./errors` for why that matters.
import { LastSignInMethodError } from "./errors";
export { LastSignInMethodError };

/**
 * Tags a failure of the RE-AUTH step. GIP tenants run email-enumeration
 * protection, which collapses "wrong password" and "no such user" into the
 * single ambiguous `auth/invalid-credential` — and that same code also means
 * "expired credential" when it comes from the LINK step. Tagging by which
 * call threw is the only way to tell them apart; never branch on the code.
 */
function reauthFailed(cause: unknown): Error {
  return Object.assign(new Error("Re-authentication failed"), {
    code: "auth/reauth-failed",
    cause,
  });
}

/** Re-auth with the account's existing password, then attach `pending`. */
export async function completeLinkWithPassword(
  email: string,
  password: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await getAuth().signInWithEmailAndPassword(email, password);
  } catch (e: unknown) {
    throw reauthFailed(e);
  }
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Google identity, then attach `pending`. */
export async function completeLinkWithGoogle(
  googleIdToken: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = GoogleAuthProvider.credential(googleIdToken);
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await getAuth().signInWithCredential(existing);
  } catch (e: unknown) {
    throw reauthFailed(e);
  }
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Apple identity, then attach `pending`. */
export async function completeLinkWithApple(
  appleIdToken: string,
  rawNonce: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = AppleAuthProvider.credential(appleIdToken, rawNonce);
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await getAuth().signInWithCredential(existing);
  } catch (e: unknown) {
    throw reauthFailed(e);
  }
  await result.user.linkWithCredential(pending);
}

/**
 * Sign-in methods already registered for `email` — e.g. ["password"],
 * ["google.com"]. Returns [] when the tenant has email-enumeration protection
 * enabled, in which case the caller must ask the user which method they used.
 */
export async function existingSignInMethods(email: string): Promise<string[]> {
  return getAuth().fetchSignInMethodsForEmail(email);
}

function requireCurrentUser(): FirebaseAuthTypes.User {
  const user = getAuth().currentUser;
  if (!user) throw new Error("Not signed in");
  return user;
}

/** Provider ids on the signed-in user: "password" | "google.com" | "apple.com". */
export async function linkedProviderIds(): Promise<string[]> {
  const user = getAuth().currentUser;
  if (!user) return [];
  return user.providerData.map((p) => p.providerId);
}

/** Attach Google to the CURRENT user — no re-auth, no email matching. */
export async function linkGoogleToCurrentUser(idToken: string): Promise<void> {
  const user = requireCurrentUser();
  await user.linkWithCredential(GoogleAuthProvider.credential(idToken));
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
  await user.linkWithCredential(AppleAuthProvider.credential(idToken, rawNonce));
}

/** Detach a provider. Refuses to remove the last one (would lock the user out). */
export async function unlinkProvider(providerId: string): Promise<void> {
  const user = requireCurrentUser();
  if (user.providerData.length <= 1) throw new LastSignInMethodError();
  await user.unlink(providerId);
}
