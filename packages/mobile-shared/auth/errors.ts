// SDK-free error types and user-facing copy for the auth layer.
//
// The `auth/*` codes mapped below are historical GIP codes. They are kept
// because the mapper is defensive by design — it must never surface a raw
// SDK string — and because `authErrorMessage` is still the fallback for any
// error the Zitadel path does not raise as a `ZitadelAuthError`.

/** Thrown when unlinking would remove the user's only remaining sign-in method. */
export class LastSignInMethodError extends Error {
  constructor() {
    super("Cannot remove the only sign-in method");
    this.name = "LastSignInMethodError";
  }
}

/** The user dismissed a native sign-in sheet. Callers show NOTHING. */
export class AuthCancelledError extends Error {
  constructor() {
    super("Sign-in cancelled");
    this.name = "AuthCancelledError";
  }
}

export type AuthErrorContext =
  | { method: "password" }
  | { method: "social"; provider: "google.com" | "apple.com" };

function errorCode(e: unknown): unknown {
  return typeof e === "object" && e !== null ? (e as { code?: unknown }).code : undefined;
}

/**
 * The single source of user-facing auth copy.
 *
 * Returns `null` ONLY when the user cancelled — callers must render nothing.
 * Never returns a raw `e.message`: native SDK strings (Swift file paths, GIP
 * internals) must never reach a user.
 *
 * Disambiguation is by TAG, never by code: GIP tenants run email-enumeration
 * protection, so `auth/wrong-password` never fires and `auth/invalid-credential`
 * means BOTH "wrong password" AND "expired credential". Only `link.ts`'s
 * `auth/reauth-failed` tag can tell them apart — which is why no message here
 * may ever say "expired".
 */
export function authErrorMessage(e: unknown, ctx?: AuthErrorContext): string | null {
  if (e instanceof AuthCancelledError) return null;
  if (e instanceof LastSignInMethodError) {
    return "You can't remove your only sign-in method.";
  }

  const code = errorCode(e);

  // Safety net for a raw Apple error that bypassed the social-auth wrapper.
  if (code === "ERR_REQUEST_CANCELED") return null;

  if (code === "ERR_REQUEST_UNKNOWN") {
    return "Couldn't complete Apple sign-in. Make sure you're signed in to iCloud on this device.";
  }
  if (code === "auth/reauth-failed") {
    if (ctx?.method === "password") return "That password is incorrect.";
    if (ctx?.method === "social") return "Couldn't verify that account. Try again.";
    // No ctx: forgetting to pass one must never produce confidently WRONG
    // copy, so stay neutral rather than guessing "password".
    return "Couldn't verify your account. Try again.";
  }
  // `auth/user-not-found` shares invalid-credential's wording deliberately.
  // The comment above assumes email-enumeration protection is on, which makes
  // GIP collapse "no such user" into `auth/invalid-credential` — but this
  // tenant returns EMAIL_NOT_FOUND, so `auth/user-not-found` really does reach
  // here and, unmapped, fell through to "Something went wrong. Try again."
  // That is the exact message a merchant who exists only in Zitadel gets
  // (#686), and it reads as a transient fault, so they retry forever. Reusing
  // the neutral copy fixes that without newly confirming whether an address
  // has an account.
  if (code === "auth/invalid-credential" || code === "auth/user-not-found") {
    return "Couldn't sign you in. Check your details and try again.";
  }
  if (code === "auth/credential-already-in-use") {
    return "That account is already linked to a different Mark8ly account.";
  }
  if (code === "auth/provider-already-linked") {
    return "That's already linked to your account.";
  }
  if (code === "auth/requires-recent-login") {
    return "For security, sign out and sign in again, then retry.";
  }
  if (code === "auth/network-request-failed") {
    return "No connection. Check your network and try again.";
  }
  if (code === "auth/too-many-requests") {
    return "Too many attempts. Try again in a few minutes.";
  }
  if (code === "auth/invalid-email") {
    return "Enter a valid email address.";
  }
  if (code === "auth/missing-password") {
    return "Enter your password.";
  }
  if (code === "auth/user-disabled") {
    return "That account has been disabled. Contact support.";
  }
  return "Something went wrong. Try again.";
}
