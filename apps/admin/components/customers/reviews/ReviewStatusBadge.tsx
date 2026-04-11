import type { ReviewStatus } from "@/lib/api/marketplace-api";

interface ReviewStatusBadgeProps {
  status: ReviewStatus;
}

const LABELS: Record<ReviewStatus, string> = {
  pending: "Pending",
  approved: "Approved",
  rejected: "Rejected",
};

const STYLES: Record<ReviewStatus, string> = {
  pending:
    "bg-[color:var(--warning-bg,rgba(180,120,20,0.08))] text-[color:var(--warning,#7a4f00)]",
  approved:
    "bg-[color:rgba(45,74,43,0.08)] text-[color:var(--moss-700)]",
  rejected:
    "bg-[color:rgba(142,26,26,0.08)] text-[color:var(--danger,#8e1a1a)]",
};

export function ReviewStatusBadge({ status }: ReviewStatusBadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-sm px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.12em] ${STYLES[status]}`}
    >
      {LABELS[status]}
    </span>
  );
}
