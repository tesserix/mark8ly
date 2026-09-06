/**
 * Parsing the URL the authentication session hands back (#686 item 1).
 *
 * Kept apart from the flow so it can be tested on its own, and hand-rolled
 * rather than using `new URL(...).searchParams`: React Native's URL
 * polyfill has historically shipped without a working `searchParams`, and
 * a silently-empty parse here would turn every successful Google sign-in
 * into "that sign-in attempt expired".
 */

export interface IdpCallback {
  intentId?: string;
  intentToken?: string;
  /** Zitadel's own failure code, when the sign-in did not succeed. */
  error?: string;
}

function decode(value: string): string {
  try {
    return decodeURIComponent(value.replace(/\+/g, " "));
  } catch {
    // A malformed escape is not worth failing the whole callback over;
    // the raw value is still more useful than dropping the parameter.
    return value;
  }
}

/**
 * Reads `id`, `token` and `error` out of the redirect the bridge page sent
 * to the app's own scheme. Anything else in the query is ignored — notably
 * Zitadel's `user`, which rides in a URL the browser followed, is
 * attacker-controlled, and is never an identity.
 */
export function parseIdpCallback(url: string): IdpCallback {
  const start = url.indexOf("?");
  if (start === -1) return {};
  // Drop any fragment: a value after '#' was never a query parameter.
  const query = url.slice(start + 1).split("#")[0];

  const out: IdpCallback = {};
  for (const pair of query.split("&")) {
    if (!pair) continue;
    const eq = pair.indexOf("=");
    const key = eq === -1 ? pair : pair.slice(0, eq);
    const value = eq === -1 ? "" : decode(pair.slice(eq + 1));
    if (key === "id") out.intentId = value;
    else if (key === "token") out.intentToken = value;
    else if (key === "error") out.error = value;
  }
  return out;
}
