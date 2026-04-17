import { cookies, headers } from "next/headers";
import Link from "next/link";

import { decodeSession } from "@/lib/auth";
import { resolveStoreSlug } from "@/lib/slug";
import { TicketsHeaderSection } from "./TicketsHeaderSection";

export const metadata = {
  title: "Support tickets",
};

export const dynamic = "force-dynamic";
export const revalidate = 0;

interface TicketSummary {
  id: string;
  ticket_number: string;
  subject: string;
  status: string;
  priority: string;
  created_at: string;
  updated_at: string;
}

interface TicketsResponse {
  data: TicketSummary[];
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function statusLabel(status: string): string {
  const labels: Record<string, string> = {
    open: "Open",
    resolved: "Resolved",
    closed: "Closed",
  };
  return labels[status] ?? status;
}

function statusClasses(status: string): string {
  switch (status) {
    case "open":
      return "bg-emerald-50 text-emerald-800 border-emerald-200";
    case "resolved":
      return "bg-blue-50 text-blue-800 border-blue-200";
    case "closed":
      return "bg-neutral-100 text-neutral-500 border-neutral-200";
    default:
      return "bg-neutral-100 text-neutral-600 border-neutral-200";
  }
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  } catch {
    return iso;
  }
}

export default async function TicketsPage() {
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Support
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Please sign in to view your support tickets.
        </p>
      </div>
    );
  }

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));

  let tickets: TicketSummary[] = [];
  let fetchError = false;

  try {
    const apiHeaders: Record<string, string> = {
      Accept: "application/json",
      Cookie: `mp_customer_session=${sessionCookie}`,
    };
    if (STOREFRONT_KEY) apiHeaders["X-Storefront-Key"] = STOREFRONT_KEY;

    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
        storeSlug,
      )}/account/tickets`,
      { headers: apiHeaders, cache: "no-store" },
    );

    if (res.ok) {
      const body = (await res.json()) as TicketsResponse;
      tickets = body.data ?? [];
    } else {
      fetchError = true;
    }
  } catch {
    fetchError = true;
  }

  return (
    <div className="space-y-6">
      <TicketsHeaderSection
        storeSlug={storeSlug}
        customerEmail={session.email}
      />

      {fetchError && (
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Unable to load your tickets right now. Please try again later.
        </p>
      )}

      {!fetchError && tickets.length === 0 && (
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          You haven&apos;t opened any tickets yet. Tap{" "}
          <span className="font-medium">New ticket</span> above to get in
          touch.
        </p>
      )}

      {tickets.length > 0 && (
        <ul className="divide-y divide-[color:var(--storefront-text,var(--ink-900))]/10 border-t border-[color:var(--storefront-text,var(--ink-900))]/10">
          {tickets.map((t) => (
            <li key={t.id}>
              <Link
                href={`/account/tickets/${t.id}`}
                className="group flex items-baseline justify-between gap-4 py-5 transition-opacity hover:opacity-75"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex items-center gap-3">
                    <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                      {t.ticket_number}
                    </span>
                    <span
                      className={`inline-block rounded-sm border px-2 py-0.5 text-xs font-medium leading-tight ${statusClasses(
                        t.status,
                      )}`}
                    >
                      {statusLabel(t.status)}
                    </span>
                  </div>
                  <p className="truncate text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
                    {t.subject}
                  </p>
                </div>
                <span className="shrink-0 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
                  {formatDate(t.updated_at)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
