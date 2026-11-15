// In-store receipt (#1106), opened from the link a seller emailed or
// printed. The token is the only credential, so it works only on the
// address of the store it belongs to.

import Link from "next/link";
import { headers } from "next/headers";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

import { NewLinkButton } from "@/components/receipt/NewLinkButton";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { fetchReceipt } from "@/lib/api/receipt-api";
import { receiptView } from "@/lib/receipt";
import { resolveStoreSlug } from "@/lib/slug";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Receipt",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function ReceiptPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const slug = await resolveStoreSlug((await headers()).get("host"));
  if (!slug) notFound();
  const [receipt, branding] = await Promise.all([fetchReceipt(slug, token), fetchBranding(slug)]);
  if (!receipt) notFound();

  const v = receiptView(receipt);
  const storeName = branding?.store?.name ?? "Receipt";
  const logo = branding?.branding?.logo_url ?? null;

  return (
    <main className="mx-auto flex min-h-screen max-w-xl flex-col gap-8 px-5 py-10 text-[color:var(--storefront-text)] print:max-w-none print:py-0">
      <header className="flex flex-col items-center gap-3 text-center">
        {logo ? <img src={logo} alt={storeName} className="h-14 w-auto object-contain" /> : null}
        <h1 className="font-[family-name:var(--storefront-heading-font)] text-2xl">{storeName}</h1>
        <p className="text-sm opacity-80">{v.placed}</p>
        <p className="text-sm">Order {v.orderNumber}</p>
      </header>

      <ul className="flex flex-col divide-y divide-current/10 border-y border-current/10">
        {v.lines.map((l, i) => (
          <li key={i} className="flex items-center gap-4 py-4 print:break-inside-avoid">
            {l.imageUrl ? (
              <img src={l.imageUrl} alt="" className="h-16 w-16 shrink-0 rounded object-cover" />
            ) : (
              <span aria-hidden="true" className="h-16 w-16 shrink-0 rounded bg-current/10" />
            )}
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="font-medium">{l.title}</span>
              <span className="text-sm opacity-80">{l.detail}</span>
            </span>
            <span className="tabular-nums">{l.price}</span>
          </li>
        ))}
      </ul>

      <dl className="grid grid-cols-[1fr_auto] gap-y-2 text-sm">
        {v.totals.map(([k, val]) => (
          <div key={k} className={`contents ${k === "Total" ? "text-base font-semibold" : ""}`}>
            <dt>{k}</dt>
            <dd className="text-right tabular-nums">{val}</dd>
          </div>
        ))}
      </dl>

      {v.contact.length > 0 ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
          {v.contact.map(([k, val]) => (
            <div key={k} className="contents">
              <dt className="opacity-80">{k}</dt>
              <dd>{val}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {v.offerNewLink ? <NewLinkButton token={token} /> : null}

      <Link
        href="/"
        className="min-h-11 self-center rounded-md bg-[color:var(--storefront-primary)] px-6 py-3 text-sm font-medium text-[color:var(--storefront-background)] print:hidden"
      >
        Shop {storeName} online
      </Link>
    </main>
  );
}
