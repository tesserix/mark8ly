import Link from "next/link";
import type { AdminCoupon } from "@/lib/api/coupons-api";

interface CouponsListProps {
  coupons: AdminCoupon[];
}

function formatType(type: AdminCoupon["type"]): string {
  switch (type) {
    case "percentage":
      return "Percentage";
    case "fixed_amount":
      return "Fixed amount";
    case "free_shipping":
      return "Free shipping";
    default:
      return type;
  }
}

function statusBadge(status: AdminCoupon["status"]) {
  const colors: Record<string, string> = {
    active: "bg-moss-700/10 text-moss-700",
    disabled: "bg-ink-100 text-ink-500",
    expired: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? "bg-ink-100 text-ink-500"}`}
    >
      {status}
    </span>
  );
}

export function CouponsList({ coupons }: CouponsListProps) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-ink-200 text-xs font-medium uppercase tracking-wider text-ink-600">
            <th className="pb-3 pr-4">Code</th>
            <th className="pb-3 pr-4">Title</th>
            <th className="pb-3 pr-4">Type</th>
            <th className="pb-3 pr-4">Value</th>
            <th className="pb-3 pr-4">Used</th>
            <th className="pb-3 pr-4">Status</th>
            <th className="pb-3 pr-4">Expires</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-ink-100">
          {coupons.map((c) => (
            <tr key={c.id} className="group">
              <td className="py-3 pr-4">
                <Link
                  href={`/marketing/coupons/${c.id}`}
                  className="font-mono text-sm font-medium text-moss-700 underline-offset-2 group-hover:underline"
                >
                  {c.code}
                </Link>
              </td>
              <td className="py-3 pr-4 text-ink-700">{c.title}</td>
              <td className="py-3 pr-4 text-ink-600">{formatType(c.type)}</td>
              <td className="py-3 pr-4 font-mono text-ink-700">
                {c.type === "percentage"
                  ? `${c.value}%`
                  : c.type === "free_shipping"
                    ? "--"
                    : `${c.currency_code ?? ""} ${c.value}`}
              </td>
              <td className="py-3 pr-4 text-ink-600">
                {c.usage_count}
                {c.usage_limit != null ? ` / ${c.usage_limit}` : ""}
              </td>
              <td className="py-3 pr-4">{statusBadge(c.status)}</td>
              <td className="py-3 pr-4 text-ink-600">
                {c.ends_at
                  ? new Date(c.ends_at).toLocaleDateString()
                  : "No expiry"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
