"use server";

// Server actions for the magic-link onboarding flow.
//
// Phase M restructure: the form no longer collects provider
// credentials. The flow is now:
//
//   1. submitOnboarding(form)   → creates session + saves business draft +
//                                 sends magic link. Pure form data.
//
//   2. (user clicks magic link)
//
//   3. verifyToken(token)       → marks the session verified server-side
//                                 and returns its id. The verify page
//                                 then redirects to /onboarding/set-password.
//
//   4. completeOnboardingWithZitadel(...)
//                               → called from the set-password page once
//                                 the merchant picks a password. Reads
//                                 the draft and calls platform-api
//                                 complete, which provisions the Zitadel
//                                 user, the admin project grant and both
//                                 FGA owner tuples.

import { onboarding, tenants, PlatformApiError } from "@/lib/api/platform-api";
import { config } from "@/lib/config";

type Result<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

function fail(err: unknown): { ok: false; code: string; message: string } {
  if (err instanceof PlatformApiError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return { ok: false, code: "unknown", message: String(err) };
}

// ─── Live slug check (live as user types in the form) ──────────────────
export async function checkSlug(
  slug: string,
): Promise<Result<{ available: boolean }>> {
  try {
    const r = await tenants.isSlugAvailable(slug);
    return { ok: true, data: { available: r.available } };
  } catch (err) {
    return fail(err);
  }
}

// ─── submitOnboarding: form submit → create session + send magic link ──
interface SubmitInput {
  email: string;
  businessName: string;
  slug: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
  // §5.1.1 — optional; persisted to draft so completeOnboarding can
  // forward them to the backend tax-ID endpoint once the store exists.
  taxId?: string;
  migrationType?: "new" | "migrating";
  whoisUrl?: string;
  // screenshot_url is the GCS URL returned by the upload helper; the
  // raw File object never reaches the server action.
  screenshotUrl?: string;
}

export async function submitOnboarding(
  input: SubmitInput,
): Promise<Result<{ sessionId: string }>> {
  try {
    const sess = await onboarding.createSession(input.email);

    // Persist the business fields into the session draft so the
    // /onboarding/set-password page (reached after the magic link click)
    // can read them server-side without depending on per-tab state.
    // §5.1.1: include migration fast-path evidence in the draft so
    // completeOnboarding can forward them to the tax-ID endpoint once
    // the store row has been created.  Fields absent when "new store"
    // is selected are simply omitted — the backend ignores them.
    const draft: Record<string, unknown> = {
      business_name: input.businessName,
      slug: input.slug,
      country_code: input.countryCode,
      currency_code: input.currencyCode,
      timezone: input.timezone,
    };
    if (input.taxId) draft.tax_id = input.taxId;
    if (input.migrationType) draft.migration_type = input.migrationType;
    if (input.whoisUrl) draft.whois_url = input.whoisUrl;
    if (input.screenshotUrl) draft.screenshot_url = input.screenshotUrl;

    await onboarding.saveDraft(sess.id, draft);

    await onboarding.sendVerification(sess.id, input.businessName);

    return { ok: true, data: { sessionId: sess.id } };
  } catch (err) {
    return fail(err);
  }
}

// ─── resendMagicLink: re-send the verification email ───────────────────
export async function resendMagicLink(
  sessionId: string,
  businessName: string,
): Promise<Result<{ sent: true }>> {
  try {
    await onboarding.sendVerification(sessionId, businessName);
    return { ok: true, data: { sent: true } };
  } catch (err) {
    return fail(err);
  }
}

// ─── verifyToken: magic link click → mark verified, return session id ──
//
// The verify landing page calls this on mount. It only marks the session
// verified and returns the session id + email so the page can redirect
// to /onboarding/set-password. Tenant creation happens later
// in completeOnboarding, after the user has picked a credential.
export async function verifyToken(
  token: string,
): Promise<Result<{ sessionId: string; email: string }>> {
  try {
    const verifyRes = await fetch(
      `${config.platformApiUrl}/api/v1/onboarding/verify-token`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
        cache: "no-store",
      },
    );
    if (!verifyRes.ok) {
      const body = (await verifyRes.json().catch(() => ({}))) as {
        error?: string;
        message?: string;
      };
      return {
        ok: false,
        code: body.error ?? "verify_failed",
        message: body.message ?? "Verification link is invalid or expired",
      };
    }
    const verifyBody = (await verifyRes.json()) as {
      data: { session_id: string; email: string };
    };
    return {
      ok: true,
      data: {
        sessionId: verifyBody.data.session_id,
        email: verifyBody.data.email,
      },
    };
  } catch (err) {
    return fail(err);
  }
}

// ─── completeOnboardingWithZitadel: set-password submit → tenant ───────
//
// Issue #685. Two things it deliberately does NOT do:
//
//   1. No `owner_user_id`. The merchant has no provider account at this
//      point, so there is no id to send; platform-api no longer requires
//      one when a provisioner is wired. platform-api's complete endpoint
//      creates the merchant's account and the mark8ly-admin project
//      grant, and writes BOTH FGA owner tuples.
//   2. No session mint. See this action's doc for what happens instead.
//
// There is no `users.listMemberTenants(uid)` pre-check, because there is
// no uid to look up. Nothing is lost: that check was a UX affordance, and
// platform-api's own ensureOwnerEmailAvailable — which runs on Complete,
// not only on Create — is the enforcement point for "this email already
// owns a store", backed by the tenants_owner_email_unique index.
interface CompleteWithZitadelInput {
  sessionId: string;
  password: string;
  /** Free-text name from the form; split into the givenName/familyName
   *  Zitadel requires. platform-api derives both from the email when
   *  they arrive empty, so a single-word name is fine. */
  name: string;
}

/**
 * What the merchant does after onboarding on the Zitadel path: they sign
 * in once, with the password they just chose.
 *
 * Not a shortcut, and not an oversight. Zitadel's login-client model
 * requires an `auth_request_id` to sign a user in, and one can only be
 * minted by the OIDC flow Zitadel itself starts by redirecting to a
 * fixed, instance-configured login URI. A page cannot mint one for
 * itself, which is why apps/admin's accept-invite hands the browser to
 * `/login/authorize` rather than logging the invitee in directly (#680).
 *
 * Onboarding cannot even do that much: the admin lives on a DIFFERENT
 * origin from this app ({slug}-admin.mark8ly.com), so there is no session
 * cookie this app could set for it under any scheme. The welcome page's
 * CTA is the hand-off, and admin's own middleware sends an
 * unauthenticated visitor to its login page. The alternative — stashing
 * the password to replay it across an origin boundary — is not a trade
 * worth making.
 */
export async function completeOnboardingWithZitadel(
  input: CompleteWithZitadelInput,
): Promise<Result<{ tenantId: string; slug: string }>> {
  try {
    const sess = await onboarding.getSession(input.sessionId);
    const draft = sess.draft ?? {};
    const businessName = draft.business_name ?? "";
    const slug = draft.slug ?? "";
    const countryCode = draft.country_code ?? "";
    const currencyCode = draft.currency_code ?? "";
    const timezone = draft.timezone ?? "UTC";

    if (!businessName || !slug || !countryCode || !currencyCode) {
      return {
        ok: false,
        code: "draft_incomplete",
        message:
          "We couldn't recover your store details. Please start onboarding again.",
      };
    }

    const { firstName, lastName } = splitName(input.name);

    const completion = await onboarding.complete(input.sessionId, {
      business_name: businessName,
      slug,
      // Lowercased before it leaves this action: every email-keyed FGA
      // tuple is lowercase and the admin login path folds to lower
      // server-side, so a `Founder@Example.com` sent verbatim would later
      // miss its own membership and produce the misleading "We couldn't
      // find a store for this account".
      owner_email: sess.email.trim().toLowerCase(),
      country_code: countryCode,
      currency_code: currencyCode,
      timezone,
      password: input.password,
      first_name: firstName,
      last_name: lastName,
    });

    return {
      ok: true,
      data: { tenantId: completion.tenant_id, slug: completion.slug },
    };
  } catch (err) {
    return fail(err);
  }
}

/** Splits a free-text name into the two parts Zitadel's profile needs. A
 *  single word becomes both, which is what platform-api's own derivation
 *  does — Zitadel rejects an empty familyName outright. */
function splitName(name: string): { firstName: string; lastName: string } {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return { firstName: "", lastName: "" };
  if (parts.length === 1) return { firstName: parts[0]!, lastName: parts[0]! };
  return {
    firstName: parts[0]!,
    lastName: parts.slice(1).join(" "),
  };
}
