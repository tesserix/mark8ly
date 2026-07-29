import type { StatusTone } from "@/components/ui";

/**
 * The ONE display string for a customer's status — badge copy and
 * screen-reader copy both, so the two can never disagree.
 *
 * This is the same fix `productStatusLabel` (lib/product-display.ts) made
 * for the identical defect on `ProductRow`: the badge and the row's
 * `accessibilityLabel` were computed separately and drifted apart, so
 * VoiceOver announced the raw wire value beside a badge reading the
 * titleised one.
 *
 * The wire schema types `status` as a bare `z.string()` (customerSchema,
 * api/schemas/customers.ts) — a server that adds a third status must not
 * blank the merchant's address book — so this has to cope with a value it
 * has never seen. The fallback humanises rather than leaking the wire token
 * verbatim ("out_of_stock" would otherwise render, and be announced, as
 * literally that).
 */
type KnownCustomerStatus = "active" | "blocked";

const STATUS_LABELS: Record<KnownCustomerStatus, string> = {
  active: "Active",
  blocked: "Blocked",
};

export function customerStatusLabel(status: string): string {
  const known = STATUS_LABELS[status as KnownCustomerStatus];
  if (known) return known;
  const words = status.replace(/[_-]+/g, " ").trim();
  if (words.length === 0) return "Unknown";
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/**
 * `active` never reaches `StatusBadge` at all — `CustomerRow` renders no
 * badge for it, deliberately (see that file). This tone map only matters for
 * the cases that DO get a badge: `blocked` is tinted `danger`, matching
 * every other restrictive state in the app (orders, reviews, gift cards).
 * Anything else — a status this app has never seen — is tinted `muted`
 * rather than `danger`, so an unrecognised value reads as "notable" instead
 * of borrowing an alarm color it hasn't earned.
 */
export function customerStatusTone(status: string): StatusTone {
  return status === "blocked" ? "danger" : "muted";
}
