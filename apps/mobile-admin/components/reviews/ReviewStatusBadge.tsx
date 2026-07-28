import { StatusBadge, type StatusTone } from "@/components/ui";
import type { ReviewStatus } from "@repo/mobile-shared/api/types";

/**
 * pending → warning (bronze on amber tint), approved → success (moss on moss
 * tint), rejected → danger (oxblood on blood tint). Mirrors web
 * ReviewStatusBadge. All three are TINTS, not solid fills — `success` in
 * particular doesn't spend the one moss accent.
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
