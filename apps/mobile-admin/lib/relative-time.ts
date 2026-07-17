/**
 * Compact "time ago" label (2m / 3h / 5d ago), falling back to a date past a
 * month. Shared by the order and notification rows so the phrasing stays
 * consistent across the app.
 */
export function formatRelativeTime(dateString: string): string {
  const then = new Date(dateString).getTime();
  if (Number.isNaN(then)) return "";

  const now = Date.now();
  const min = Math.floor((now - then) / 60_000);
  const hr = Math.floor((now - then) / 3_600_000);
  const day = Math.floor((now - then) / 86_400_000);

  if (min < 1) return "just now";
  if (min < 60) return `${min}m ago`;
  if (hr < 24) return `${hr}h ago`;
  if (day < 30) return `${day}d ago`;
  return new Date(dateString).toLocaleDateString();
}
