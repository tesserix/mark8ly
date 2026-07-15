import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

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

function isAccountExistsConflict(
  e: unknown,
): e is { code: string; email?: string } {
  return (
    typeof e === "object" &&
    e !== null &&
    (e as { code?: unknown }).code === ACCOUNT_EXISTS
  );
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
        email: e.email ?? "",
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
        email: e.email ?? "",
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
