// Printable return authorization. Customers land here from the
// "Print return label" button on the order page once the request is
// approved. The page is intentionally un-chromeless — no header, no
// footer, no Otto widget — so browser-print renders a clean one-page
// document the customer can hand to the pickup courier.

import { cookies, headers } from "next/headers";
import { notFound } from "next/navigation";

import { resolveStoreSlug } from "@/lib/slug";
import { fetchOrder } from "@/lib/api/checkout-api";
import { PrintReturnLabelButton } from "@/components/account/PrintReturnLabelButton";

interface StorefrontReturn {
  id: string;
  return_number: string;
  type: "return" | "replace";
  status: string;
  reason?: string;
  notes?: string;
  pickup_details?: string;
  reject_reason?: string;
  currency_code: string;
  requested_at: string;
  approved_at?: string;
}

interface PageProps {
  params: Promise<{ id: string; returnId: string }>;
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

async function fetchReturns(
  slug: string,
  orderId: string,
  sessionCookie: string,
): Promise<StorefrontReturn[]> {
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(slug)}/orders/${encodeURIComponent(orderId)}/returns`,
      {
        method: "GET",
        headers: {
          Accept: "application/json",
          "X-Storefront-Key": STOREFRONT_KEY,
          Cookie: sessionCookie ? `mp_customer_session=${sessionCookie}` : "",
        },
        cache: "no-store",
      },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { returns?: StorefrontReturn[] };
    return body.returns ?? [];
  } catch {
    return [];
  }
}

export default async function ReturnLabelPage({ params }: PageProps) {
  const { id, returnId } = await params;
  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value ?? "";
  if (!session) notFound();

  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
  if (!slug) notFound();

  const [order, returns] = await Promise.all([
    fetchOrder(slug, id),
    fetchReturns(slug, id, session),
  ]);
  if (!order) notFound();
  const r = returns.find((x) => x.id === returnId);
  // Only expose the label once the admin has approved (or moved further
  // along the lifecycle). Showing it earlier would confuse the courier.
  if (
    !r ||
    (r.status !== "approved" && r.status !== "received" && r.status !== "refunded")
  ) {
    notFound();
  }
  const typeLabel = r.type === "replace" ? "REPLACEMENT" : "RETURN";
  const ship = order.shipping_address;

  return (
    <div className="min-h-screen bg-white p-8 font-sans text-[13px] text-black print:p-0">
      <div className="mx-auto max-w-2xl space-y-6 border border-black/20 bg-white p-8 shadow-sm print:border-0 print:shadow-none">
        {/* Header */}
        <header className="flex flex-col items-start justify-between gap-4 border-b-2 border-black pb-4 sm:flex-row sm:items-start">
          <div>
            <p className="text-xs uppercase tracking-[0.25em] text-black/60">
              {typeLabel} AUTHORIZATION
            </p>
            <h1 className="mt-1 text-3xl font-bold leading-tight">
              {r.return_number}
            </h1>
            <p className="mt-1 text-xs text-black/60">
              Order {order.order_number}
              {" · "}Issued{" "}
              {r.approved_at
                ? new Date(r.approved_at).toLocaleDateString("en-GB", {
                    year: "numeric",
                    month: "short",
                    day: "numeric",
                  })
                : ""}
            </p>
          </div>
          <BarcodeBlock value={r.return_number} />
        </header>

        {/* Addresses */}
        <section className="grid grid-cols-1 gap-6 sm:grid-cols-2 print:grid-cols-2">
          <AddressCard
            label="FROM (CUSTOMER)"
            name={ship?.name ?? ""}
            lines={[ship?.line1, ship?.line2].filter(Boolean) as string[]}
            cityRegion={[
              ship?.city ?? "",
              ship?.region ?? "",
              ship?.postal_code ?? "",
            ]
              .filter(Boolean)
              .join(", ")}
            country={ship?.country_code ?? ""}
          />
          <AddressCard
            label="TO (RETURNS)"
            name={slug.toUpperCase() + " · Returns dept"}
            lines={["Store will share exact address in pickup details"]}
            cityRegion=""
            country=""
          />
        </section>

        {/* Items */}
        <section>
          <p className="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-black/60">
            Items in this {r.type === "replace" ? "replacement" : "return"}
          </p>
          <div className="-mx-2 overflow-x-auto px-2 print:overflow-visible">
            <table className="w-full min-w-[320px] border-collapse text-left">
              <thead>
                <tr className="border-b border-black/20 text-[11px] uppercase tracking-wider text-black/60">
                  <th className="py-2">SKU</th>
                  <th className="py-2 text-right">Qty</th>
                </tr>
              </thead>
              <tbody>
                {order.items.map((it, i) => (
                  <tr key={i} className="border-b border-black/10">
                    <td className="py-2 font-mono text-[11px] break-all">
                      {it.sku_snapshot}
                    </td>
                    <td className="py-2 text-right tabular-nums">{it.quantity}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        {/* Pickup / logistics */}
        {r.pickup_details && (
          <section className="rounded border border-black/30 bg-black/[0.03] p-4">
            <p className="mb-1 text-xs font-semibold uppercase tracking-[0.2em] text-black/60">
              Pickup instructions
            </p>
            <p className="whitespace-pre-wrap leading-relaxed">
              {r.pickup_details}
            </p>
          </section>
        )}

        {/* Instructions */}
        <section className="space-y-2 text-[12px] leading-relaxed">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-black/60">
            Before pickup
          </p>
          <ol className="list-inside list-decimal space-y-1">
            <li>
              Keep the item{order.items.length > 1 ? "s" : ""} in the original
              packaging, sealed.
            </li>
            <li>
              Attach this authorization sheet to the package or quote
              <span className="mx-1 font-mono font-semibold">
                {r.return_number}
              </span>
              to the courier.
            </li>
            <li>Hand the package to the pickup agent during the scheduled slot.</li>
          </ol>
        </section>

        <footer className="flex items-center justify-between border-t border-black/20 pt-3 text-[11px] text-black/50">
          <span>Reason: {r.reason ?? "—"}</span>
          <span>Page 1 of 1</span>
        </footer>
      </div>

      {/* Print button + no-print hides when the customer prints */}
      <div className="mx-auto mt-6 max-w-2xl text-center print:hidden">
        <PrintReturnLabelButton />
      </div>
    </div>
  );
}

function AddressCard({
  label,
  name,
  lines,
  cityRegion,
  country,
}: {
  label: string;
  name: string;
  lines: string[];
  cityRegion: string;
  country: string;
}) {
  return (
    <div>
      <p className="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-black/60">
        {label}
      </p>
      <div className="space-y-0.5 leading-snug">
        <p className="font-semibold">{name}</p>
        {lines.map((l, i) => (
          <p key={i}>{l}</p>
        ))}
        {cityRegion && <p>{cityRegion}</p>}
        {country && <p className="uppercase tracking-wider">{country}</p>}
      </div>
    </div>
  );
}

// BarcodeBlock renders a lightweight visual substitute for a real 1D
// barcode — a monospace strip with the RMA number below. Sufficient for
// the courier to read; a real barcode can be wired in when we add a PDF
// generator.
function BarcodeBlock({ value }: { value: string }) {
  return (
    <div className="text-right">
      <div
        aria-hidden="true"
        className="ml-auto h-16 w-[180px] bg-[length:6px_100%] bg-repeat-x bg-[linear-gradient(to_right,black_50%,white_50%)]"
      />
      <p className="mt-1 font-mono text-[10px] tracking-widest">{value}</p>
    </div>
  );
}
