// Shared decision logic behind the storefront's three "Continue with
// Google" controls (CustomerSignInForm, CreateAccountForm,
// SecurityClient's "Add Google"). Lives under lib/** rather than inline
// in each component so it stays covered by apps/storefront's vitest
// config — components/** is not (see @/lib/auth/provider's file header).
//
// Under GIP (getAuthProvider() === "gip"), Google sign-in is unchanged
// from before this phase: the browser goes to the mark8ly.com/auth/google
// trampoline, exactly as buildTrampolineUrl below has always built it.
//
// Under Zitadel, there is no trampoline: Zitadel takes the return URL per
// request, so the browser goes straight from auth-bff's own Google IDP
// intent back to this store's own host (apps/storefront/app/auth/idp/
// finish/route.ts). resolveGoogleSignInUrl calls the startCustomerGoogleSignIn
// server action to get that authUrl instead of building the trampoline URL.

import { getAuthProvider } from "@/lib/auth/provider";
import { startCustomerGoogleSignIn } from "@/app/auth/idp/actions";
import type { GoogleSignInDest } from "@/lib/auth/google-sign-in-dest";

const TRAMPOLINE_BASE =
  process.env.NEXT_PUBLIC_MARK8LY_AUTH_URL ?? "https://mark8ly.com";

export type GoogleSignInIntent = "signin" | "signup" | "link";

export interface GoogleSignInArgs {
  storeSlug: string;
  intent: GoogleSignInIntent;
  dest: GoogleSignInDest;
  /** window.location.origin — passed in so this stays testable without a DOM. */
  origin: string;
}

/**
 * buildTrampolineUrl returns the exact GIP trampoline URL. Byte-identical
 * to what each component's handleGoogle built inline before this phase —
 * pinned here so the "flag unset: still targets mark8ly.com/auth/google
 * unchanged" requirement has one place to fail if it regresses.
 */
export function buildTrampolineUrl(args: GoogleSignInArgs): string {
  const url = new URL("/auth/google", TRAMPOLINE_BASE);
  url.searchParams.set("return_to", `${args.origin}${args.dest}`);
  url.searchParams.set("store_slug", args.storeSlug);
  url.searchParams.set("intent", args.intent);
  return url.toString();
}

export type ResolveGoogleSignInUrlResult =
  | { ok: true; url: string }
  | { ok: false; message: string };

/**
 * resolveGoogleSignInUrl decides where a "Continue with Google" click
 * should send the browser: the trampoline under GIP (untouched), or a
 * freshly minted Zitadel authUrl under Zitadel (via the
 * startCustomerGoogleSignIn server action). Returns a result instead of
 * throwing so the caller can render a truthful message rather than an
 * unhandled rejection.
 */
export async function resolveGoogleSignInUrl(
  args: GoogleSignInArgs,
): Promise<ResolveGoogleSignInUrlResult> {
  if (getAuthProvider() !== "zitadel") {
    return { ok: true, url: buildTrampolineUrl(args) };
  }
  const result = await startCustomerGoogleSignIn(args.dest);
  if (!result.ok) return result;
  return { ok: true, url: result.authUrl };
}
