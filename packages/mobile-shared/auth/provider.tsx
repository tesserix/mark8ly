import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import Constants, { ExecutionEnvironment } from "expo-constants";
import { tokenStorage } from "./token-storage";
import { getEnv } from "../config/env";
import type { AppleFullName, SocialSignInOutcome } from "./social-credentials";
import type { FirebaseAuthTypes } from "@react-native-firebase/auth";

/**
 * Loose user shape used by mobile screens — both the real Firebase user
 * and the Expo Go stub conform to this. We avoid importing
 * FirebaseAuthTypes here so the Firebase native module never has to load
 * inside Expo Go (which doesn't ship RNFBAppModule).
 */
export interface AuthUser {
  uid: string;
  email: string | null;
  displayName: string | null;
}

interface AuthState {
  user: AuthUser | null;
  loading: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  signInWithGoogle: (idToken: string, accessToken?: string) => Promise<SocialSignInOutcome>;
  signInWithApple: (
    idToken: string,
    rawNonce: string,
    fullName?: AppleFullName | null,
  ) => Promise<SocialSignInOutcome>;
  signOut: () => Promise<void>;
  /** Returns the cached GIP id_token. Cheap, safe to call per request. */
  getToken: () => Promise<string | null>;
  /**
   * Forces GIP to mint a fresh id_token, refreshing custom claims and
   * extending the expiry. Use after a 401 from the API before treating
   * the session as terminated.
   */
  refreshToken: () => Promise<string | null>;
  completeLinkWithPassword: (
    email: string,
    password: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  completeLinkWithGoogle: (
    googleIdToken: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  completeLinkWithApple: (
    appleIdToken: string,
    rawNonce: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  existingSignInMethods: (email: string) => Promise<string[]>;
  linkedProviderIds: () => Promise<string[]>;
  linkGoogleToCurrentUser: (idToken: string) => Promise<void>;
  linkAppleToCurrentUser: (idToken: string, rawNonce: string) => Promise<void>;
  unlinkProvider: (providerId: string) => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

interface AuthProviderProps {
  /**
   * GIP/Identity Platform tenant pool id. Optional — defaults to the value
   * configured via `expo.extra.gipTenantId` (set in app.config.js). Pass
   * explicitly only to override per-mount (e.g. in tests).
   */
  tenantId?: string;
  children: ReactNode;
}

interface AuthBackend {
  signIn: (email: string, password: string) => Promise<void>;
  signInWithGoogle: (idToken: string, accessToken?: string) => Promise<SocialSignInOutcome>;
  signInWithApple: (
    idToken: string,
    rawNonce: string,
    fullName?: AppleFullName | null,
  ) => Promise<SocialSignInOutcome>;
  signOut: () => Promise<void>;
  getIdToken: () => Promise<string | null>;
  getIdTokenForced: () => Promise<string | null>;
  onAuthStateChanged: (cb: (user: AuthUser | null) => void) => () => void;
  completeLinkWithPassword: (
    email: string,
    password: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  completeLinkWithGoogle: (
    googleIdToken: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  completeLinkWithApple: (
    appleIdToken: string,
    rawNonce: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  existingSignInMethods: (email: string) => Promise<string[]>;
  linkedProviderIds: () => Promise<string[]>;
  linkGoogleToCurrentUser: (idToken: string) => Promise<void>;
  linkAppleToCurrentUser: (idToken: string, rawNonce: string) => Promise<void>;
  unlinkProvider: (providerId: string) => Promise<void>;
}

/** True when running inside the public Expo Go shell (no custom native modules). */
function isExpoGo(): boolean {
  return Constants.executionEnvironment === ExecutionEnvironment.StoreClient;
}

/**
 * Force the demo (no-Firebase) backend outside Expo Go by setting
 * EXPO_PUBLIC_AUTH_BACKEND=demo. This lets a native dev-client / simulator
 * build boot and render the login screen without a bundled
 * GoogleService-Info.plist — real GIP auth needs that plist and is wired
 * per app in a later phase. Defaults off, so production builds are unaffected.
 */
function isDemoAuthForced(): boolean {
  return process.env.EXPO_PUBLIC_AUTH_BACKEND === "demo";
}

/**
 * Demo backend used in Expo Go. Skips Firebase entirely so the UI can
 * render without RNFBAppModule. Starts signed out so the Login screen
 * shows on launch — exactly like the web admin — and any email/password
 * accepts and routes into the dashboard.
 *
 * Real GIP/Firebase auth lights up in dev-client and production builds
 * (eas build --profile uat / production), where RNFBAppModule is linked.
 */
function createDemoBackend(): AuthBackend {
  let active: AuthUser | null = null;
  const subs = new Set<(u: AuthUser | null) => void>();
  return {
    signIn: async (email) => {
      // Accept anything; reflect the typed email back as the user.
      active = {
        uid: `expo-go-demo:${email}`,
        email,
        displayName: email.split("@")[0] ?? "Demo Admin",
      };
      for (const cb of subs) cb(active);
    },
    signInWithGoogle: async () => {
      active = { uid: "expo-go-demo:google", email: "demo@mark8ly.com", displayName: "Demo Admin" };
      for (const cb of subs) cb(active);
      return { status: "signed-in" };
    },
    signInWithApple: async () => {
      active = { uid: "expo-go-demo:apple", email: "demo@mark8ly.com", displayName: "Demo Admin" };
      for (const cb of subs) cb(active);
      return { status: "signed-in" };
    },
    signOut: async () => {
      active = null;
      for (const cb of subs) cb(active);
    },
    getIdToken: async () => (active ? "expo-go-demo-token" : null),
    getIdTokenForced: async () => (active ? "expo-go-demo-token" : null),
    onAuthStateChanged: (cb) => {
      subs.add(cb);
      cb(active);
      return () => subs.delete(cb);
    },
    completeLinkWithPassword: async () => {},
    completeLinkWithGoogle: async () => {},
    completeLinkWithApple: async () => {},
    existingSignInMethods: async () => [],
    linkedProviderIds: async () => ["password"],
    linkGoogleToCurrentUser: async () => {},
    linkAppleToCurrentUser: async () => {},
    unlinkProvider: async () => {},
  };
}

function createFirebaseBackend(tenantId: string): AuthBackend {
  // Lazy require so the static Firebase imports never resolve inside Expo Go.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { createGIPAuth } = require("./gip") as typeof import("./gip");
  const gip = createGIPAuth({ tenantId });
  return {
    signIn: async (email, password) => {
      await gip.signIn(email, password);
    },
    signInWithGoogle: (idToken, accessToken) => gip.signInWithGoogle(idToken, accessToken),
    signInWithApple: (idToken, rawNonce, fullName) =>
      gip.signInWithApple(idToken, rawNonce, fullName),
    signOut: () => gip.signOut(),
    getIdToken: () => gip.getIdToken(),
    getIdTokenForced: () => gip.getIdTokenForced(),
    onAuthStateChanged: (cb) =>
      gip.onAuthStateChanged((u) =>
        cb(
          u
            ? {
                uid: u.uid,
                email: u.email,
                displayName: u.displayName,
              }
            : null,
        ),
      ),
    completeLinkWithPassword: (email, password, pending) =>
      gip.completeLinkWithPassword(email, password, pending),
    completeLinkWithGoogle: (googleIdToken, pending) =>
      gip.completeLinkWithGoogle(googleIdToken, pending),
    completeLinkWithApple: (appleIdToken, rawNonce, pending) =>
      gip.completeLinkWithApple(appleIdToken, rawNonce, pending),
    existingSignInMethods: (email) => gip.existingSignInMethods(email),
    linkedProviderIds: () => gip.linkedProviderIds(),
    linkGoogleToCurrentUser: (idToken) => gip.linkGoogleToCurrentUser(idToken),
    linkAppleToCurrentUser: (idToken, rawNonce) => gip.linkAppleToCurrentUser(idToken, rawNonce),
    unlinkProvider: (providerId) => gip.unlinkProvider(providerId),
  };
}

export function AuthProvider({ tenantId, children }: AuthProviderProps) {
  const resolvedTenantId = tenantId ?? getEnv().gipTenantId;
  const [backend] = useState<AuthBackend>(() =>
    isExpoGo() || isDemoAuthForced()
      ? createDemoBackend()
      : createFirebaseBackend(resolvedTenantId),
  );
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const unsubscribe = backend.onAuthStateChanged((u) => {
      setUser(u);
      setLoading(false);
    });
    return unsubscribe;
  }, [backend]);

  const signIn = async (email: string, password: string) => {
    await backend.signIn(email, password);
  };

  const signInWithGoogle = (idToken: string, accessToken?: string) =>
    backend.signInWithGoogle(idToken, accessToken);

  const signInWithApple = (
    idToken: string,
    rawNonce: string,
    fullName?: AppleFullName | null,
  ) => backend.signInWithApple(idToken, rawNonce, fullName);

  const signOut = async () => {
    await tokenStorage.clearAll();
    await backend.signOut();
  };

  const getToken = () => backend.getIdToken();
  const refreshToken = () => backend.getIdTokenForced();

  const completeLinkWithPassword = (
    email: string,
    password: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => backend.completeLinkWithPassword(email, password, pending);

  const completeLinkWithGoogle = (
    googleIdToken: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => backend.completeLinkWithGoogle(googleIdToken, pending);

  const completeLinkWithApple = (
    appleIdToken: string,
    rawNonce: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => backend.completeLinkWithApple(appleIdToken, rawNonce, pending);

  const existingSignInMethods = (email: string) => backend.existingSignInMethods(email);

  const linkedProviderIds = () => backend.linkedProviderIds();

  const linkGoogleToCurrentUser = (idToken: string) => backend.linkGoogleToCurrentUser(idToken);

  const linkAppleToCurrentUser = (idToken: string, rawNonce: string) =>
    backend.linkAppleToCurrentUser(idToken, rawNonce);

  const unlinkProvider = (providerId: string) => backend.unlinkProvider(providerId);

  return (
    <AuthContext.Provider
      value={{
        user,
        loading,
        signIn,
        signInWithGoogle,
        signInWithApple,
        signOut,
        getToken,
        refreshToken,
        completeLinkWithPassword,
        completeLinkWithGoogle,
        completeLinkWithApple,
        existingSignInMethods,
        linkedProviderIds,
        linkGoogleToCurrentUser,
        linkAppleToCurrentUser,
        unlinkProvider,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
