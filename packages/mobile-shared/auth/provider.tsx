import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import Constants, { ExecutionEnvironment } from "expo-constants";
import { tokenStorage } from "./token-storage";
import { zitadelSession } from "./zitadel-session";

/**
 * Loose user shape used by mobile screens. Kept as a plain structural type
 * rather than a provider SDK type so no native auth module has to load just
 * to describe a signed-in person.
 *
 * On the Zitadel path this is ALWAYS null: sign-in happens through
 * `zitadel-signin.ts` and the resulting bearer lives in `zitadel-session`,
 * so there is no SDK holding a user object. Callers that need
 * "is somebody signed in?" must read the session, not this field.
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
  /** Returns the cached bearer token. Cheap, safe to call per request. */
  getToken: () => Promise<string | null>;
  /**
   * Asks for the freshest token the backend can produce. Use after a 401
   * from the API before treating the session as terminated.
   */
  refreshToken: () => Promise<string | null>;
}

const AuthContext = createContext<AuthState | null>(null);

interface AuthProviderProps {
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
 * Force the demo backend outside Expo Go by setting
 * EXPO_PUBLIC_AUTH_BACKEND=demo. This lets a native dev-client / simulator
 * build boot and render the login screen without reaching a real auth
 * server. Defaults off, so production builds are unaffected.
 */
function isDemoAuthForced(): boolean {
  return process.env.EXPO_PUBLIC_AUTH_BACKEND === "demo";
}

/**
 * Demo backend used in Expo Go and in simulator builds. Starts signed out so
 * the Login screen shows on launch, and any email/password accepts.
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

/**
 * The real backend (#786). Zitadel is now the only auth provider, and its
 * sign-in is NOT an SDK call: `zitadel-signin.ts` posts our own form to
 * marketplace-api and persists the returned bearer in `zitadel-session`.
 * So this backend owns no user and mints no token — it only surfaces what
 * that session already holds.
 *
 * `signIn` throws rather than silently doing nothing: a screen that calls it
 * has not been wired to the Zitadel flow, and a no-op would look like a
 * successful sign-in that never signs anybody in.
 */
function createZitadelBackend(): AuthBackend {
  return {
    signIn: async () => {
      throw new Error(
        "Password sign-in is not routed through AuthProvider on the Zitadel path — use createZitadelSignIn().",
      );
    },
    signOut: async () => {
      // The tokens live in SecureStore and are cleared by `signOut` below.
    },
    getIdToken: () => zitadelSession.accessTokenIfFresh(),
    // There is no separate force-refresh: the persisted token is the only
    // one there is, so a 401 retry re-reads it and, if it has lapsed,
    // correctly gets null and terminates the session.
    getIdTokenForced: () => zitadelSession.accessTokenIfFresh(),
    onAuthStateChanged: (cb) => {
      // Resolve immediately so `loading` clears; there is never a user here.
      cb(null);
      return () => {};
    },
  };
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [backend] = useState<AuthBackend>(() =>
    isExpoGo() || isDemoAuthForced() ? createDemoBackend() : createZitadelBackend(),
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
    // The Zitadel bearer tokens live in SecureStore, owned by this app rather
    // than by an auth SDK, so `backend.signOut()` below cannot reach them.
    // Without this, signing out left the tokens in place: AuthGate re-read a
    // still-fresh token, decided the user was signed in and sent them straight
    // back to the dashboard.
    await zitadelSession.clear();
    await backend.signOut();
  };

  const getToken = () => backend.getIdToken();
  const refreshToken = () => backend.getIdTokenForced();

  return (
    <AuthContext.Provider value={{ user, loading, signIn, signOut, getToken, refreshToken }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
