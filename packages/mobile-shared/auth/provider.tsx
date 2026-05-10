import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import Constants, { ExecutionEnvironment } from "expo-constants";
import { tokenStorage } from "./token-storage";
import { getEnv } from "../config/env";

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
  signOut: () => Promise<void>;
  /** Returns the cached GIP id_token. Cheap, safe to call per request. */
  getToken: () => Promise<string | null>;
  /**
   * Forces GIP to mint a fresh id_token, refreshing custom claims and
   * extending the expiry. Use after a 401 from the API before treating
   * the session as terminated.
   */
  refreshToken: () => Promise<string | null>;
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
  signOut: () => Promise<void>;
  getIdToken: () => Promise<string | null>;
  getIdTokenForced: () => Promise<string | null>;
  onAuthStateChanged: (cb: (user: AuthUser | null) => void) => () => void;
}

/** True when running inside the public Expo Go shell (no custom native modules). */
function isExpoGo(): boolean {
  return Constants.executionEnvironment === ExecutionEnvironment.StoreClient;
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
  };
}

export function AuthProvider({ tenantId, children }: AuthProviderProps) {
  const resolvedTenantId = tenantId ?? getEnv().gipTenantId;
  const [backend] = useState<AuthBackend>(() =>
    isExpoGo() ? createDemoBackend() : createFirebaseBackend(resolvedTenantId),
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

  const signOut = async () => {
    await tokenStorage.clearAll();
    await backend.signOut();
  };

  const getToken = () => backend.getIdToken();
  const refreshToken = () => backend.getIdTokenForced();

  return (
    <AuthContext.Provider
      value={{ user, loading, signIn, signOut, getToken, refreshToken }}
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
