import * as SecureStore from "expo-secure-store";

/**
 * Persistent home for the bearer tokens a Zitadel sign-in returns (#686).
 *
 * The GIP path never needed this: the Firebase SDK owned the tokens and
 * minted a fresh id_token on demand. Zitadel tokens arrive as plain values
 * from our own API, so the app has to keep them itself — in SecureStore,
 * the same place the tenant and store ids already live, never AsyncStorage.
 */

const KEYS = {
  ACCESS: "mark8ly_zitadel_access_token",
  REFRESH: "mark8ly_zitadel_refresh_token",
  /** Absolute epoch-ms expiry, computed once at issue. */
  EXPIRES_AT: "mark8ly_zitadel_expires_at",
} as const;

export interface StoredSession {
  accessToken: string;
  refreshToken: string;
  expiresAt: number;
}

/**
 * Treat a token as expired slightly early, so one that would lapse
 * mid-flight is refreshed instead of producing a 401 the client then has to
 * interpret as "signed out".
 */
const EXPIRY_SKEW_MS = 60_000;

export const zitadelSession = {
  /**
   * expiresIn is seconds-from-now, as the server returns it, and is
   * converted to an absolute instant here. Storing the relative value would
   * make it meaningless after the first app restart.
   */
  async save(accessToken: string, refreshToken: string, expiresIn: number): Promise<void> {
    const expiresAt = Date.now() + expiresIn * 1000;
    await Promise.all([
      SecureStore.setItemAsync(KEYS.ACCESS, accessToken),
      SecureStore.setItemAsync(KEYS.REFRESH, refreshToken),
      SecureStore.setItemAsync(KEYS.EXPIRES_AT, String(expiresAt)),
    ]);
  },

  async read(): Promise<StoredSession | null> {
    const [accessToken, refreshToken, rawExp] = await Promise.all([
      SecureStore.getItemAsync(KEYS.ACCESS),
      SecureStore.getItemAsync(KEYS.REFRESH),
      SecureStore.getItemAsync(KEYS.EXPIRES_AT),
    ]);
    if (!accessToken) return null;
    const expiresAt = Number(rawExp);
    return {
      accessToken,
      refreshToken: refreshToken ?? "",
      // A missing or unparseable expiry is treated as already elapsed
      // rather than as "never expires": the safe reading of corrupt state
      // is to re-authenticate, not to trust a token indefinitely.
      expiresAt: Number.isFinite(expiresAt) ? expiresAt : 0,
    };
  },

  /** The access token, or null when absent or (near) expired. */
  async accessTokenIfFresh(): Promise<string | null> {
    const s = await zitadelSession.read();
    if (!s) return null;
    if (s.expiresAt - EXPIRY_SKEW_MS <= Date.now()) return null;
    return s.accessToken;
  },

  async clear(): Promise<void> {
    await Promise.all([
      SecureStore.deleteItemAsync(KEYS.ACCESS),
      SecureStore.deleteItemAsync(KEYS.REFRESH),
      SecureStore.deleteItemAsync(KEYS.EXPIRES_AT),
    ]);
  },
};
