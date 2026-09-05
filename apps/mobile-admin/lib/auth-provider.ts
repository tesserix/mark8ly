/**
 * Which auth provider this build uses (#686).
 *
 * NEXT_PUBLIC-style flags are inlined at BUILD time, so this cannot be
 * flipped by a chart env var — a switch means a rebuild, and for the app a
 * store release. Defaults to GIP so an unmodified build is unchanged.
 */
export function isZitadelProvider(): boolean {
  return process.env.EXPO_PUBLIC_AUTH_PROVIDER === "zitadel";
}
