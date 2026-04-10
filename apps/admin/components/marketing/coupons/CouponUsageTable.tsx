import type { CouponUsageRow } from "@/lib/api/coupons-api";

interface CouponUsageTableProps {
  usages: CouponUsageRow[];
  total: number;
}

export function CouponUsageTable({ usages, total }: CouponUsageTableProps) {
  if (usages.length === 0) {
    return (
      <p className="py-6 text-sm text-ink-500">
        No usage records yet. This coupon has not been redeemed.
      </p>
    );
  }

  return (
    <div>
      <p className="mb-3 text-xs text-ink-500">
        {total} total redemption{total !== 1 ? "s" : ""}
      </p>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-ink-200 text-xs font-medium uppercase tracking-wider text-ink-600">
              <th className="pb-3 pr-4">Customer</th>
              <th className="pb-3 pr-4">Discount</th>
              <th className="pb-3 pr-4">Order</th>
              <th className="pb-3 pr-4">Date</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-ink-100">
            {usages.map((u) => (
              <tr key={u.id}>
                <td className="py-2 pr-4 text-ink-700">{u.customer_email}</td>
                <td className="py-2 pr-4 font-mono text-ink-700">
                  {u.currency_code} {u.discount_amount}
                </td>
                <td className="py-2 pr-4 font-mono text-xs text-ink-600">
                  {u.order_id.slice(0, 8)}...
                </td>
                <td className="py-2 pr-4 text-ink-600">
                  {new Date(u.created_at).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
