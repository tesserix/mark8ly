import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";
import { toByteArray } from "base64-js";

export interface AppleFullName {
  givenName?: string | null;
  familyName?: string | null;
}

/**
 * Result of a social sign-in. `needs-link` means the tenant is
 * one-account-per-email and an account already exists for this email under a
 * different provider — the caller must have the user re-authenticate with
 * their existing method, then link `pendingCredential` onto it.
 */
export type SocialSignInOutcome =
  | { status: "signed-in" }
  | {
      status: "needs-link";
      email: string;
      provider: "google.com" | "apple.com";
      pendingCredential: FirebaseAuthTypes.AuthCredential;
    };

const ACCOUNT_EXISTS = "auth/account-exists-with-different-credential";

function isAccountExistsConflict(e: unknown): e is { code: string } {
  return (
    typeof e === "object" &&
    e !== null &&
    (e as { code?: unknown }).code === ACCOUNT_EXISTS
  );
}

/**
 * Best-effort read of the `email` claim from a provider id_token, used only to
 * decide which account the user must re-authenticate as. This is a UX hint, not
 * a trust decision — Firebase validates the token's signature server-side — so
 * the payload is decoded without verification. Returns "" for any malformed token.
 */
function emailFromIdToken(idToken: string): string {
  try {
    const payload = idToken.split(".")[1];
    if (!payload) return "";
    const base64 = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=");
    const bytes = toByteArray(padded);
    // Bytes → UTF-8 string without atob/TextDecoder (neither exists in this RN runtime).
    const json = decodeURIComponent(
      Array.from(bytes)
        .map((b) => "%" + b.toString(16).padStart(2, "0"))
        .join(""),
    );
    const claims: unknown = JSON.parse(json);
    const email = (claims as { email?: unknown }).email;
    return typeof email === "string" ? email : "";
  } catch {
    return "";
  }
}

function needsLinkEmail(idToken: string, e: unknown): string {
  const fromToken = emailFromIdToken(idToken);
  if (fromToken) return fromToken;
  const fromUserInfo = (e as { userInfo?: { email?: unknown } }).userInfo?.email;
  return typeof fromUserInfo === "string" ? fromUserInfo : "";
}

export async function signInWithGoogleCredential(
  idToken: string,
  accessToken?: string,
): Promise<SocialSignInOutcome> {
  const cred = auth.GoogleAuthProvider.credential(idToken, accessToken);
  try {
    await auth().signInWithCredential(cred);
    return { status: "signed-in" };
  } catch (e: unknown) {
    if (isAccountExistsConflict(e)) {
      return {
        status: "needs-link",
        email: needsLinkEmail(idToken, e),
        provider: "google.com",
        pendingCredential: cred,
      };
    }
    throw e;
  }
}

export async function signInWithAppleCredential(
  idToken: string,
  rawNonce: string,
  fullName?: AppleFullName | null,
): Promise<SocialSignInOutcome> {
  const cred = auth.AppleAuthProvider.credential(idToken, rawNonce);
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await auth().signInWithCredential(cred);
  } catch (e: unknown) {
    if (isAccountExistsConflict(e)) {
      return {
        status: "needs-link",
        email: needsLinkEmail(idToken, e),
        provider: "apple.com",
        pendingCredential: cred,
      };
    }
    throw e;
  }
  const displayName = buildDisplayName(fullName);
  if (displayName && !result.user.displayName) {
    try {
      await result.user.updateProfile({ displayName });
    } catch {
      // Best-effort: name capture is non-fatal; the user stays signed in.
    }
  }
  return { status: "signed-in" };
}

function buildDisplayName(fullName?: AppleFullName | null): string {
  if (!fullName) return "";
  return [fullName.givenName, fullName.familyName]
    .map((p) => (p ?? "").trim())
    .filter((p) => p.length > 0)
    .join(" ");
}
