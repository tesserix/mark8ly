// Phone helpers for checkout address validation.
//
// Indian mobile numbers are 10 digits starting with 6/7/8/9. Delhivery
// rejects shipment-create for IN destinations with a missing or
// malformed phone, so the format is enforced client-side as well as
// server-side.

/** Bare Indian mobile: 10 digits, first digit 6-9. */
export const IN_PHONE_RE = /^[6-9]\d{9}$/;

export function isIndia(countryCode: string): boolean {
  return countryCode.trim().toUpperCase() === "IN";
}

/**
 * Reduce a typed Indian mobile to the bare 10 digits Delhivery expects.
 *
 * Customers paste "+91 98765 43210" or "098765 43210" far more often
 * than the bare form. Rejecting those outright stalled checkout with no
 * usable signal — the address was treated as incomplete, so no shipping
 * rates loaded and Place order stayed disabled. Strip the formatting and
 * the country/trunk prefix rather than demanding one exact shape.
 *
 * Anything not recognisably an Indian mobile keeps its digits and so
 * still fails IN_PHONE_RE — normalization must not launder a
 * wrong-country number (e.g. an Australian "+61 469 044 601") into a
 * passing one.
 */
export function normalizeInPhone(raw: string): string {
  const digits = raw.replace(/[\s()\-.]/g, "").replace(/^\+/, "");
  if (digits.length === 12 && digits.startsWith("91")) return digits.slice(2);
  if (digits.length === 11 && digits.startsWith("0")) return digits.slice(1);
  return digits;
}

export function isInPhoneValid(phone: string): boolean {
  return IN_PHONE_RE.test(normalizeInPhone(phone));
}
