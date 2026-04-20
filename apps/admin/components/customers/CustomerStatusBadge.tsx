import type { CustomerStatus } from "@/lib/api/marketplace-api";

interface CustomerStatusBadgeProps {
  status: CustomerStatus;
  className?: string;
}

const statusConfig: Record<
  CustomerStatus,
  { label: string; chipClass: string }
> = {
  active: {
    label: "Active",
    chipClass: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  },
  blocked: {
    label: "Blocked",
    chipClass: "bg-[color:var(--danger)]/10 text-[color:var(--danger)]",
  },
};

export function CustomerStatusBadge({
  status,
  className = "",
}: CustomerStatusBadgeProps) {
  const config = statusConfig[status] ?? statusConfig.active;
  return (
    <span
      className={`inline-flex items-center rounded-sm px-2.5 py-0.5 text-xs font-medium ${config.chipClass} ${className}`}
    >
      {config.label}
    </span>
  );
}
