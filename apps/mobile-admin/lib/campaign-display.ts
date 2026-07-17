import type { StatusTone } from "@/components/ui";

const STATUS_TONE: Record<string, StatusTone> = {
  draft: "muted",
  scheduled: "info",
  sending: "info",
  sent: "success",
  paused: "warning",
  failed: "danger",
};

export function campaignStatusTone(status: string): StatusTone {
  return STATUS_TONE[status] ?? "muted";
}

export function titleizeStatus(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
