// The ONE place the storefront normalizes a customer email before it
// touches Zitadel, the pending-signup token, or the session cookie.
//
// Why this exists: marketplace-api keys its customer upsert on
// `strings.TrimSpace(strings.ToLower(email))` (services/marketplace-api/
// internal/customer/service.go). auth-bff, meanwhile, echoes register's
// `req.Email` straight back — whatever casing/whitespace the shopper (or
// their browser's autofill) typed. Sign THAT verbatim string into the
// pending-signup token and the token would certify "the string the server
// signed", not "the address that received the code, in the form the
// profile is keyed by" — today's margin (an MTA folding a differently-
// cased local part to the same mailbox) is accidental, not a guarantee.
//
// Fix: normalize once, here, and use this SAME function on both sides of
// the token — registerCustomer normalizes before sending to auth-bff (and
// before signing), verifyCustomerEmail normalizes `input.email` identically
// before checking it against that signature. If these two call sites ever
// used different normalization, every legitimate signup with a mixed-case
// or padded address would break, which is exactly why there is only one
// function to import instead of two inlined `.trim().toLowerCase()`s.
export function normalizeCustomerEmail(email: string): string {
  return email.trim().toLowerCase();
}
