import { StatusBadge, type StatusTone } from "@/components/ui";
import type { ReviewStatus } from "@repo/mobile-shared/api/types";

/**
 * pending → warning (bronze on amber tint), approved → success (moss on moss
 * tint), rejected → danger (oxblood on blood tint). Mirrors web
 * ReviewStatusBadge.
 *
 * All three are TINTS, not solid fills, which is the rule the spec's
 * Guardrails actually state: "Success stays a moss tint (#E8EEE2/#2D4A2B),
 * never a solid moss fill". A moss-tint success badge is PERMITTED generally
 * and does not spend the one accent — `OrderStatusBadges` maps fulfilled →
 * success on the same grounds. The "one accent per view" line in the spec is
 * scoped to the DASHBOARD, where the moss is spent on the revenue chart's
 * fill/stroke/endpoint and the Approve swipe; that is why `lib/queue.ts`
 * deliberately emits no `success` tone at all, and it is a statement about
 * that screen, not about this badge.
 */
const TONE: Record<ReviewStatus, StatusTone> = {
  pending: "warning",
  approved: "success",
  rejected: "danger",
};

const LABEL: Record<ReviewStatus, string> = {
  pending: "Pending",
  approved: "Approved",
  rejected: "Rejected",
};

export function ReviewStatusBadge({ status }: { status: ReviewStatus }) {
  return <StatusBadge label={LABEL[status]} tone={TONE[status]} />;
}
