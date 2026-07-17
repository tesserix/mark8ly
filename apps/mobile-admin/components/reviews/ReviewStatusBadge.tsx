import { StatusBadge, type StatusTone } from "@/components/ui";
import type { ReviewStatus } from "@repo/mobile-shared/api/types";

/**
 * pending → warning (ink-on-amber), approved → success (moss-tint),
 * rejected → danger. Mirrors web ReviewStatusBadge. `success` is the tint,
 * not a solid moss fill, so it doesn't spend the one accent.
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
