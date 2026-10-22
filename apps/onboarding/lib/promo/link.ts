/**
 * The promo code a shared link carries, e.g. /onboarding?promo=HAVEN90.
 *
 * Agency partners hand their clients a link rather than a code to retype, so
 * the code that attributes the store to them is not lost to a forgotten or
 * mistyped field. Pure so it can be tested without rendering the form.
 *
 * Nothing here is trusted: the value only pre-fills the field, and the usual
 * server check decides what it grants.
 */

// promo_codes.code is varchar(64).
const MAX_CODE_LENGTH = 64;

/** The code in `search` (a location.search string), or "" when there is none. */
export function promoFromSearch(search: string): string {
  const raw = new URLSearchParams(search).get("promo") ?? "";
  // Codes are stored without whitespace, so none in a link can be meaningful.
  return raw.replace(/\s+/g, "").slice(0, MAX_CODE_LENGTH);
}
