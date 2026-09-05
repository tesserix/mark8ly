// v26 removed the namespaced default export (`import auth from ...`; `auth()`).
// The modular `getAuth()` is the replacement; the instance it returns keeps the
// same methods, so only the entry point changes here.
import { getAuth, type FirebaseAuthTypes } from "@react-native-firebase/auth";

import {
  signInWithGoogleCredential,
  signInWithAppleCredential,
  type AppleFullName,
} from "./social-credentials";
import {
  completeLinkWithPassword,
  completeLinkWithGoogle,
  completeLinkWithApple,
  existingSignInMethods,
  linkedProviderIds,
  linkGoogleToCurrentUser,
  linkAppleToCurrentUser,
  unlinkProvider,
} from "./link";

export type { SocialSignInOutcome } from "./social-credentials";

export interface GIPAuthConfig {
  tenantId: string;
}

export function createGIPAuth(config: GIPAuthConfig) {
  const firebaseAuth = getAuth();
  // @react-native-firebase v22+: `tenantId` is a read-only getter — direct
  // assignment throws "Proxy set returned false". `setTenantId` sets the JS
  // field synchronously and propagates to native asynchronously. Await this
  // before any sign-in so the native session is scoped to the GIP tenant.
  const tenantReady = firebaseAuth.setTenantId(config.tenantId);
  // Passive handler so a startup rejection doesn't surface as an unhandled
  // rejection; sign-in awaiters below still observe any real error.
  tenantReady.catch(() => {});

  return {
    signIn: async (email: string, password: string) => {
      await tenantReady;
      return firebaseAuth.signInWithEmailAndPassword(email, password);
    },
    signInWithGoogle: async (idToken: string, accessToken?: string) => {
      await tenantReady;
      return signInWithGoogleCredential(idToken, accessToken);
    },
    signInWithApple: async (
      idToken: string,
      rawNonce: string,
      fullName?: AppleFullName | null,
    ) => {
      await tenantReady;
      return signInWithAppleCredential(idToken, rawNonce, fullName);
    },
    completeLinkWithPassword: async (
      email: string,
      password: string,
      pending: FirebaseAuthTypes.AuthCredential,
    ) => {
      await tenantReady;
      return completeLinkWithPassword(email, password, pending);
    },
    completeLinkWithGoogle: async (
      googleIdToken: string,
      pending: FirebaseAuthTypes.AuthCredential,
    ) => {
      await tenantReady;
      return completeLinkWithGoogle(googleIdToken, pending);
    },
    completeLinkWithApple: async (
      appleIdToken: string,
      rawNonce: string,
      pending: FirebaseAuthTypes.AuthCredential,
    ) => {
      await tenantReady;
      return completeLinkWithApple(appleIdToken, rawNonce, pending);
    },
    existingSignInMethods: async (email: string) => {
      await tenantReady;
      return existingSignInMethods(email);
    },
    linkedProviderIds: async () => {
      await tenantReady;
      return linkedProviderIds();
    },
    linkGoogleToCurrentUser: async (idToken: string) => {
      await tenantReady;
      return linkGoogleToCurrentUser(idToken);
    },
    linkAppleToCurrentUser: async (idToken: string, rawNonce: string) => {
      await tenantReady;
      return linkAppleToCurrentUser(idToken, rawNonce);
    },
    unlinkProvider: async (providerId: string) => {
      await tenantReady;
      return unlinkProvider(providerId);
    },
    signOut: () => firebaseAuth.signOut(),
    getIdToken: async (): Promise<string | null> => {
      const user = firebaseAuth.currentUser;
      if (!user) return null;
      return user.getIdToken(false);
    },
    getIdTokenForced: async (): Promise<string | null> => {
      const user = firebaseAuth.currentUser;
      if (!user) return null;
      return user.getIdToken(true);
    },
    getCurrentUser: (): FirebaseAuthTypes.User | null => firebaseAuth.currentUser,
    onAuthStateChanged: (callback: (user: FirebaseAuthTypes.User | null) => void) =>
      firebaseAuth.onAuthStateChanged(callback),
    sendPasswordResetEmail: (email: string) =>
      firebaseAuth.sendPasswordResetEmail(email),
  };
}

export type GIPAuth = ReturnType<typeof createGIPAuth>;
