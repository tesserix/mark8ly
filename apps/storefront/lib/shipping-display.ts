// Pure display helpers for shipping rates and tracking, kept out of the
// components because storefront vitest only runs lib/** and app/** in node.

export function humanizeService(service: string): string {
  const map: Record<string, string> = {
    ground: "Standard Ground",
    express: "Express",
    priority: "Priority",
    economy: "Economy",
    overnight: "Overnight",
    two_day: "2-Day Shipping",
    standard: "Standard Shipping",
  };
  const key = service.toLowerCase().replace(/[\s-]+/g, "_");
  return map[key] ?? service.charAt(0).toUpperCase() + service.slice(1).replace(/_/g, " ");
}

export function rateLabel(rate: { service: string; name?: string }): string {
  return rate.name || humanizeService(rate.service);
}

export function rateEstimate(rate: { estimate?: string; estimated_days?: number }): string | null {
  if (rate.estimate) return rate.estimate;
  const d = rate.estimated_days ?? 0;
  return d > 0 ? `Est. ${d} business day${d !== 1 ? "s" : ""}` : null;
}

export function trackingHref(shipment: { tracking_url?: string }): string | null {
  const u = shipment.tracking_url;
  return u && u.startsWith("https://") ? u : null;
}
