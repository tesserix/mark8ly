import Link from "next/link";
import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { cookies } from "next/headers";

interface MyGiftCard {
  id: string;
  code_display: string;
  initial_balance: string;
  current_balance: string;
  currency_code: string;
  status: string;
  recipient_email?: string;
  purchased_by_email?: string;
  purchased_via_storefront?: boolean;
  expires_at?: string;
  created_at: string;
}

async function fetchMyGiftCards(storeSlug: string): Promise<MyGiftCard[]> {
  const base =
    process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
  const key = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";
  const c = await cookies();
  const sessionCookie = c.get("mp_customer_session");
  if (!sessionCookie) return [];
  const res = await fetch(
    `${base}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account/gift-cards`,
    {
      headers: {
        ...(key ? { "X-Storefront-Key": key } : {}),
        Cookie: `mp_customer_session=${sessionCookie.value}`,
      },
      cache: "no-store",
    },
  );
  if (!res.ok) return [];
  const body = await res.json();
  return (body.data ?? []) as MyGiftCard[];
}

function formatMoney(amount: string, currency: string) {
  try {
    return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(
      Number(amount),
    );
  } catch {
    return `${currency} ${amount}`;
  }
}

export default async function MyGiftCardsPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);
  const cards = await fetchMyGiftCards(storeSlug);

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Gift cards
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))]/80">
          Cards you&apos;ve purchased, and cards others have sent you.
        </p>
      </header>

      {cards.length === 0 ? (
        <div className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] p-8 text-sm text-[color:var(--storefront-text,var(--ink-900))]/80">
          You don&apos;t have any gift cards yet.{" "}
          <Link
            href="/gift-cards/purchase"
            className="text-[color:var(--storefront-accent,var(--moss-700))] underline-offset-2 hover:underline"
          >
            Buy a gift card
          </Link>
          .
        </div>
      ) : (
        <ul className="space-y-3">
          {cards.map((c) => (
            <li
              key={c.id}
              className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] p-5"
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <p className="font-mono text-sm text-[color:var(--storefront-text,var(--ink-900))]">
                    {c.code_display}
                  </p>
                  <p className="mt-2 font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl text-[color:var(--storefront-text,var(--ink-900))]">
                    {formatMoney(c.current_balance, c.currency_code)}
                  </p>
                  <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
                    of {formatMoney(c.initial_balance, c.currency_code)}{" "}
                    initial
                  </p>
                </div>
                <span className="inline-flex items-center rounded-full bg-[color:var(--storefront-text,var(--ink-900))]/10 px-2.5 py-0.5 text-xs capitalize text-[color:var(--storefront-text,var(--ink-900))]/80">
                  {c.status}
                </span>
              </div>
              {c.expires_at && (
                <p className="mt-3 text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
                  Expires{" "}
                  {new Date(c.expires_at).toLocaleDateString(undefined, {
                    year: "numeric",
                    month: "long",
                    day: "numeric",
                  })}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
