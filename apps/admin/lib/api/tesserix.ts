// tesserix-home client. Used from the merchant admin to file platform support
// tickets and read active platform announcements.
//
// ONE CHANNEL now (tesserix-home#152). Everything here goes to the PLATFORM
// API over the in-cluster ClusterIP, authenticated with a Zitadel machine
// token. That token identifies mark8ly, and the API scopes every read and
// write to the product it resolves to — so this client cannot reach another
// product's queue even if it asked.
//
// THE SHARED SECRET IS GONE from this file. `INTERNAL_API_TOKEN` and
// `X-Internal-Token` are no longer used by the admin app at all: the tickets
// calls moved first, and the announcements client that remained here had no
// callers — the banner reaches announcements through its own proxy in
// app/api/platform-announcements/route.ts, which also now speaks to the
// platform API.
//
// The mark8ly admin server remains the trusted caller in both: it forwards the
// merchant's identity (tenantId, name, email, userId) from its own
// authenticated session, not from client input. What changed is that the
// platform API now VERIFIES the product half of that claim rather than taking
// it on trust, and records a merchant's reply as the merchant.

import { getPlatformToken } from "./platform-token";



const PRODUCT_ID = "mark8ly";

// The platform API, reached over the in-cluster ClusterIP.
//
// NOT tesserix.app: the platform API has no public path and no VirtualService.
// Its address is a Service in another namespace, which this pod can reach —
// verified from this namespace during the #152 rollout.
const PLATFORM_API_URL =
  process.env.TESSERIX_PLATFORM_API_URL ?? "http://platform-api.tesserix.svc.cluster.local";


// Headers for the platform API: a Zitadel machine token.
//
// Bearer is correct HERE even though the comment above says otherwise for
// tesserix.app. That constraint belongs to the public ingress, and this call
// never passes through it. Do not "fix" this to X-Internal-Token.
async function platformHeaders(): Promise<Record<string, string>> {
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${await getPlatformToken()}`,
  };
}

// The platform API answers §4.4 envelopes: { success, data, meta } on success
// and { error: { code, message } } on refusal. apps/web answered bare bodies
// and a STRING error, so both shapes are read here — the string form until
// announcements move, the object form for everything else.
function refusalOf(body: Record<string, unknown> | null, status: number): { code: string; message: string } {
  const err = body?.error;
  if (err && typeof err === "object") {
    const o = err as { code?: string; message?: string };
    // The API's own message, which names what it declined. Falling back to a
    // status here would discard the one thing the caller can act on.
    return { code: o.code ?? `http_${status}`, message: o.message ?? messageForCode(o.code ?? "", status) };
  }
  const code = typeof err === "string" ? err : `http_${status}`;
  return { code, message: messageForCode(code, status) };
}

// Discriminated result type so callers branch cleanly on success/failure
// without try/catching at every call site. Mirrors the marketplace-api
// client convention used elsewhere in the admin app.
export type Result<T> =
  | { ok: true; data: T }
  | { ok: false; error: { code: string; message: string } };

// Shared shape for tickets coming back from tesserix-home.
export interface PlatformTicket {
  id: string;
  product_id: string;
  tenant_id: string;
  ticket_number: string;
  subject: string;
  description: string;
  status: string;
  priority: string;
  submitted_by_name: string;
  submitted_by_email: string;
  // submitted_by_user_id is deliberately ABSENT, matching the platform API's
  // wire shape. It was declared here and never rendered; keeping it would
  // promise a field every response now omits.
  resolved_at: string | null;
  created_at: string;
  updated_at: string;
}

// One announcement as the PLATFORM API returns it (tesserix-home#152).
//
// Narrower than the old apps/web row, and deliberately so on that side:
// `audience_filter` names the other products a broadcast targets,
// `created_by` identifies staff to a merchant, and `is_published` is always
// true for anything this endpoint returns. Declaring them here would promise
// fields every response omits.
export interface PlatformAnnouncement {
  id: string;
  title: string;
  body: string;
  severity: string;
  starts_at: string;
  ends_at: string | null;
}

export interface FilePlatformTicketInput {
  tenantId: string;
  subject: string;
  description: string;
  priority?: "low" | "medium" | "high" | "urgent";
  submittedByName: string;
  submittedByEmail: string;
  submittedByUserId?: string;
}

async function readJsonSafely(res: Response): Promise<Record<string, unknown> | null> {
  try {
    return (await res.json()) as Record<string, unknown>;
  } catch {
    return null;
  }
}

export async function filePlatformTicket(
  input: FilePlatformTicketInput,
): Promise<Result<PlatformTicket>> {
  try {
    const res = await fetch(`${PLATFORM_API_URL}/v1/tickets`, {
      method: "POST",
      headers: await platformHeaders(),
      cache: "no-store",
      body: JSON.stringify({
        // No product. The platform API takes it from the SCOPE this machine
        // token resolves to, so a product sent here would be ignored at best
        // and a claim to file into another product's queue at worst.
        tenant_id: input.tenantId,
        subject: input.subject,
        description: input.description,
        priority: input.priority,
        submitted_by_name: input.submittedByName,
        submitted_by_email: input.submittedByEmail,
        submitted_by_user_id: input.submittedByUserId,
      }),
    });
    if (!res.ok) {
      return { ok: false, error: refusalOf(await readJsonSafely(res), res.status) };
    }
    const body = (await res.json()) as { data: { ticket: PlatformTicket } };
    return { ok: true, data: body.data.ticket };
  } catch (err) {
    return {
      ok: false,
      error: {
        code: "network_error",
        message: err instanceof Error ? err.message : "Could not reach tesserix.",
      },
    };
  }
}

export async function listMyPlatformTickets(
  tenantId: string,
): Promise<PlatformTicket[]> {
  // No tenant, no call. The platform API refuses a product caller that names
  // none, so asking would be a guaranteed 422 — and on the OLD API the same
  // omission returned every tenant's tickets, which is the bug this replaces.
  if (!tenantId) return [];

  try {
    const url = new URL(`${PLATFORM_API_URL}/v1/tickets`);
    url.searchParams.set("tenant", tenantId);

    const res = await fetch(url.toString(), {
      headers: await platformHeaders(),
      cache: "no-store",
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { data?: { tickets?: PlatformTicket[] } };
    return body.data?.tickets ?? [];
  } catch {
    return [];
  }
}

export interface PlatformTicketReply {
  id: string;
  ticket_id: string;
  author_type: "merchant" | "platform_admin";
  author_name: string;
  author_email: string | null;
  // Absent for the same reason as submitted_by_user_id above.
  content: string;
  created_at: string;
}

export interface PlatformTicketWithThread {
  ticket: PlatformTicket;
  replies: PlatformTicketReply[];
}

export async function getMyPlatformTicket(
  tenantId: string,
  ticketId: string,
): Promise<PlatformTicketWithThread | null> {
  if (!tenantId || !ticketId) return null;

  try {
    const url = new URL(`${PLATFORM_API_URL}/v1/tickets/${ticketId}`);
    url.searchParams.set("tenant", tenantId);

    const res = await fetch(url.toString(), {
      headers: await platformHeaders(),
      cache: "no-store",
    });
    // A ticket outside this tenant answers 404, exactly as one that does not
    // exist — so this null covers both, and deliberately cannot tell them
    // apart. That non-disclosure is the API's, not an accident here.
    if (!res.ok) return null;
    const body = (await res.json()) as { data: PlatformTicketWithThread };
    return body.data;
  } catch {
    return null;
  }
}

export interface ReplyInput {
  tenantId: string;
  ticketId: string;
  content: string;
  authorName: string;
  authorEmail?: string;
  authorUserId?: string;
}

export async function replyToPlatformTicket(
  input: ReplyInput,
): Promise<Result<PlatformTicketReply>> {
  try {
    const res = await fetch(`${PLATFORM_API_URL}/v1/tickets/${input.ticketId}/replies`, {
      method: "POST",
      headers: await platformHeaders(),
      cache: "no-store",
      body: JSON.stringify({
        tenant_id: input.tenantId,
        content: input.content,
        // The MERCHANT's identity, and the reason it is required: without it
        // the platform API refuses rather than recording the reply as the
        // support team, which is what it did before tesserix-home#568.
        author_name: input.authorName,
        author_email: input.authorEmail,
        author_user_id: input.authorUserId,
      }),
    });
    if (!res.ok) {
      return { ok: false, error: refusalOf(await readJsonSafely(res), res.status) };
    }
    const body = (await res.json()) as { data: { reply: PlatformTicketReply } };
    return { ok: true, data: body.data.reply };
  } catch (err) {
    return {
      ok: false,
      error: {
        code: "network_error",
        message: err instanceof Error ? err.message : "Could not reach tesserix.",
      },
    };
  }
}


function messageForCode(code: string, status: number): string {
  switch (code) {
    case "unauthorized":
      return "Platform support is not configured for this environment. Contact support@tesserix.app.";
    case "invalid_payload":
      return "Some fields are invalid. Please review and try again.";
    case "missing_params":
      return "Missing required parameters.";
    case "not_found":
      return "Ticket not found.";
    case "ticket_closed":
      return "This ticket is closed and can't accept new replies.";
    case "db_unavailable":
      return "Tesserix is temporarily unavailable. Please try again in a moment.";
    default:
      return `Request failed (HTTP ${status}). Please try again.`;
  }
}
