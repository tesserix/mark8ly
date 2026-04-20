// Shared formatters for admin UI. Centralizes number, money, and relative
// time formatting so list components don't each carry their own Intl
// configuration and drift apart.
//
// Locale handling: every formatter accepts an optional `locale` argument
// that takes precedence over the ambient locale. When omitted, formatters
// resolve to `defaultLocale()` — which honors NEXT_PUBLIC_ADMIN_LOCALE
// during build and falls back to the runtime's default (server: "en-US",
// browser: navigator.language) in all other cases.
//
// This gives us a single seam to flip the admin to another locale later
// (LocaleProvider can wire into defaultLocale() without touching call
// sites) while keeping the common case — one English-speaking user per
// tenant — zero-config.

type LocaleArg = string | string[] | undefined;

function defaultLocale(): LocaleArg {
  const envLocale = process.env.NEXT_PUBLIC_ADMIN_LOCALE;
  if (envLocale && envLocale.trim().length > 0) return envLocale;
  if (typeof navigator !== "undefined" && navigator.language) {
    return navigator.language;
  }
  return undefined;
}

export function formatMoney(
  amount: number | string | null | undefined,
  currencyCode: string | undefined | null = "USD",
  locale?: LocaleArg,
): string {
  const value =
    typeof amount === "string" ? Number.parseFloat(amount) : (amount ?? 0);
  if (!Number.isFinite(value)) return "—";
  try {
    return new Intl.NumberFormat(locale ?? defaultLocale(), {
      style: "currency",
      currency: (currencyCode ?? "USD").toUpperCase(),
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  } catch {
    return `${value.toFixed(2)} ${currencyCode ?? ""}`.trim();
  }
}

export function formatDate(
  iso: string | null | undefined,
  locale?: LocaleArg,
): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleDateString(locale ?? defaultLocale(), {
      year: "numeric",
      month: "long",
      day: "numeric",
    });
  } catch {
    return iso;
  }
}

export function formatDateTime(
  iso: string | null | undefined,
  locale?: LocaleArg,
): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString(locale ?? defaultLocale(), {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

// Short human relative time: "3m ago", "2h ago", "5d ago".
// Falls back to the locale date for anything older than ~30 days.
export function timeAgo(
  iso: string | null | undefined,
  locale?: LocaleArg,
): string {
  if (!iso) return "—";
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "—";

  const diffMs = Date.now() - then;
  const seconds = Math.max(1, Math.floor(diffMs / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return formatDate(iso, locale);
}

export function formatNumber(
  value: number | null | undefined,
  locale?: LocaleArg,
  options?: Intl.NumberFormatOptions,
): string {
  if (value == null || !Number.isFinite(value)) return "—";
  try {
    return new Intl.NumberFormat(locale ?? defaultLocale(), options).format(
      value,
    );
  } catch {
    return String(value);
  }
}
