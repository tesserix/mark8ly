import type { StatusTone } from "@/components/ui";

const STATUS_TONE: Record<string, StatusTone> = {
  active: "success",
  redeemed: "muted",
  expired: "muted",
  disabled: "warning",
};

export function giftCardStatusTone(status: string): StatusTone {
  return STATUS_TONE[status] ?? "muted";
}

export function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
