import type { StatusTone } from "@/components/ui";
import { formatMoney } from "@/lib/money";
import type { Coupon } from "@repo/mobile-shared/api/types";

/** Human value for a coupon: "10% off" · "$15 off" · "Free shipping". */
export function formatCouponValue(coupon: Coupon, currency: string): string {
  switch (coupon.type) {
    case "percentage":
      return `${coupon.value}% off`;
    case "fixed_amount":
      return `${formatMoney(coupon.value, coupon.currency_code || currency)} off`;
    case "free_shipping":
      return "Free shipping";
    default:
      return String(coupon.value);
  }
}

const STATUS_TONE: Record<string, StatusTone> = {
  active: "success",
  scheduled: "info",
  expired: "muted",
  disabled: "warning",
};

export function couponStatusTone(status: string): StatusTone {
  return STATUS_TONE[status] ?? "muted";
}
