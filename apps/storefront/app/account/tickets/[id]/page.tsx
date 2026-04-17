import { cookies, headers } from "next/headers";
import Link from "next/link";
import { notFound } from "next/navigation";

import { decodeSession } from "@/lib/auth";
import { resolveStoreSlug } from "@/lib/slug";

import { ReplyForm } from "./ReplyForm";
import { TicketStatusStepper } from "./TicketStatusStepper";

export const dynamic = "force-dynamic";
export const revalidate = 0;

interface TicketReply {
  id: string;
  author_type: "support" | "customer";
  author_name: string;
  content: string;
  created_at: string;
}

interface TicketDetail {
  id: string;
  ticket_number: string;
  subject: string;
  status: "open" | "in_progress" | "resolved" | "closed";
  priority: string;
  created_at: string;
  updated_at: string;
  description: string;
  replies: TicketReply[];
}

interface TicketDetailResponse {
  data: TicketDetail;
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString("en-US", {
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

export default async function TicketDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Ticket
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Please sign in to view this ticket.
        </p>
      </div>
    );
  }

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));

  const apiHeaders: Record<string, string> = {
    Accept: "application/json",
    Cookie: `mp_customer_session=${sessionCookie}`,
  };
  if (STOREFRONT_KEY) apiHeaders["X-Storefront-Key"] = STOREFRONT_KEY;

  let ticket: TicketDetail | null = null;
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
        storeSlug,
      )}/account/tickets/${encodeURIComponent(id)}`,
      { headers: apiHeaders, cache: "no-store" },
    );
    if (res.status === 404) {
      notFound();
    }
    if (res.ok) {
      const body = (await res.json()) as TicketDetailResponse;
      ticket = body.data;
    }
  } catch {
    // fall through to error UI below
  }

  if (!ticket) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Ticket
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Unable to load this ticket right now. Please try again later.
        </p>
      </div>
    );
  }

  const closed = ticket.status === "closed";

  return (
    <div className="space-y-8">
      <div className="space-y-2">
        <Link
          href="/account/tickets"
          className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60 hover:underline"
        >
          ← All tickets
        </Link>
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          {ticket.subject}
        </h1>
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          {ticket.ticket_number} · opened {formatDateTime(ticket.created_at)}
        </p>
      </div>

      {/* Read-only status stepper so the shopper sees exactly where
          their ticket is in the workflow, no ambiguity. */}
      <div className="rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface,white)] p-5">
        <TicketStatusStepper status={ticket.status} />
      </div>

      <article className="space-y-2 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
        <p className="text-xs font-medium text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          You wrote
        </p>
        <p className="whitespace-pre-wrap text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          {ticket.description}
        </p>
      </article>

      {ticket.replies.length > 0 && (
        <ol className="space-y-6">
          {ticket.replies.map((r) => (
            <li
              key={r.id}
              className="space-y-2 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6"
            >
              <p className="text-xs font-medium text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
                {r.author_type === "support"
                  ? `${r.author_name} (support)`
                  : `${r.author_name} (you)`}
                <span className="ml-2 opacity-60">
                  {formatDateTime(r.created_at)}
                </span>
              </p>
              <p className="whitespace-pre-wrap text-sm text-[color:var(--storefront-text,var(--ink-900))]">
                {r.content}
              </p>
            </li>
          ))}
        </ol>
      )}

      <div className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
        <ReplyForm
          ticketId={ticket.id}
          disabled={closed}
          disabledReason="This ticket is closed. Open a new one if you need further help."
        />
      </div>
    </div>
  );
}
