import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

export async function signInWithGoogleCredential(
  idToken: string,
  accessToken?: string,
): Promise<FirebaseAuthTypes.UserCredential> {
  const cred = auth.GoogleAuthProvider.credential(idToken, accessToken);
  return auth().signInWithCredential(cred);
}

export interface AppleFullName {
  givenName?: string | null;
  familyName?: string | null;
}

export async function signInWithAppleCredential(
  idToken: string,
  rawNonce: string,
  fullName?: AppleFullName | null,
): Promise<FirebaseAuthTypes.UserCredential> {
  const cred = auth.AppleAuthProvider.credential(idToken, rawNonce);
  const result = await auth().signInWithCredential(cred);
  const displayName = buildDisplayName(fullName);
  if (displayName && !result.user.displayName) {
    try {
      await result.user.updateProfile({ displayName });
    } catch {
      // Best-effort: name capture is non-fatal; the user stays signed in.
    }
  }
  return result;
}

function buildDisplayName(fullName?: AppleFullName | null): string {
  if (!fullName) return "";
  return [fullName.givenName, fullName.familyName]
    .map((p) => (p ?? "").trim())
    .filter((p) => p.length > 0)
    .join(" ");
}
