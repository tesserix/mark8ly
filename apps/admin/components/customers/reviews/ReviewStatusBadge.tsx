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
    "bg-[color:var(--warning)]/10 text-[color:var(--warning)]",
  approved:
    "bg-[color:var(--accent-tint)] text-[color:var(--moss-700)]",
  rejected:
    "bg-[color:var(--danger)]/10 text-[color:var(--danger)]",
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
