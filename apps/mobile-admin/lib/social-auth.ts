import { Platform } from "react-native";
import { GoogleSignin } from "@react-native-google-signin/google-signin";
import * as AppleAuthentication from "expo-apple-authentication";
import { AuthCancelledError } from "@repo/mobile-shared/auth/errors";
import type { AppleFullName } from "@repo/mobile-shared/auth/social-credentials";

let configured = false;

export function configureGoogleSignin(): void {
  if (configured) return;
  const webClientId = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID;
  if (!webClientId) {
    throw new Error("EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID is not configured");
  }
  // The Android OAuth client itself isn't passed to GoogleSignin.configure() —
  // the SDK matches it automatically via google-services.json's registered
  // client for this package name + signing certificate SHA-1. But that
  // Android client doesn't exist until the Firebase console work lands, so
  // this guard exists purely to fail loudly here instead of letting
  // signIn() reach Play Services and fail with an opaque DEVELOPER_ERROR.
  // Same precedent as the webClientId check above.
  if (Platform.OS === "android" && !process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID) {
    throw new Error("EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID is not configured");
  }
  GoogleSignin.configure({
    webClientId,
    iosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID,
  });
  configured = true;
}

export async function signInWithGoogleNative(): Promise<string> {
  await GoogleSignin.hasPlayServices({ showPlayServicesUpdateDialog: true });
  const result = (await GoogleSignin.signIn()) as {
    type?: string;
    data?: { idToken?: string | null };
    idToken?: string | null;
  };
  // The SDK RESOLVES with {type:"cancelled"} rather than rejecting, so a
  // cancel would otherwise fall through to the "no ID token" throw below and
  // be shown to the user as a failure.
  if (result?.type === "cancelled") throw new AuthCancelledError();
  const idToken = result?.data?.idToken ?? result?.idToken;
  if (!idToken) throw new Error("Google sign-in failed: no ID token");
  return idToken;
}

export async function signInWithAppleNative(): Promise<{
  idToken: string;
  rawNonce: string;
  fullName: AppleFullName | null;
}> {
  let cred: AppleAuthentication.AppleAuthenticationCredential;
  try {
    cred = await AppleAuthentication.signInAsync({
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
    });
  } catch (e: unknown) {
    const code = typeof e === "object" && e !== null ? (e as { code?: unknown }).code : undefined;
    if (code === "ERR_REQUEST_CANCELED") throw new AuthCancelledError();
    throw e;
  }
  if (!cred.identityToken) throw new Error("Apple sign-in failed: no identity token");
  // Home-Chef passes an empty rawNonce (GIP verifies Apple's token without a
  // client nonce in their setup); keep parity. Revisit if GIP rejects it in 1b.
  return { idToken: cred.identityToken, rawNonce: "", fullName: cred.fullName };
}
