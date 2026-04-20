import type { CustomerStatus } from "@/lib/api/marketplace-api";

interface CustomerStatusBadgeProps {
  status: CustomerStatus;
  className?: string;
}

const statusConfig: Record<
  CustomerStatus,
  { label: string; colorClass: string }
> = {
  active: {
    label: "Active",
    colorClass: "text-[color:var(--moss-700)]",
  },
  blocked: {
    label: "Blocked",
    colorClass: "text-[color:var(--danger)]",
  },
};

export function CustomerStatusBadge({
  status,
  className = "",
}: CustomerStatusBadgeProps) {
  const config = statusConfig[status] ?? statusConfig.active;
  return (
    <span className={`font-medium ${config.colorClass} ${className}`}>
      {config.label}
    </span>
  );
}
