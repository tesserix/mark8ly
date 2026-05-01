// tesserix-home internal API client. Used from the merchant admin to
// file platform support tickets and read active platform announcements.
//
// The tesserix-home /api/internal/* endpoints are bearer-authenticated
// with a shared secret (INTERNAL_API_TOKEN) that lives in both pods'
// envs (provisioned via the GSM-backed ExternalSecret in each chart).
// The mark8ly admin server is the trusted caller — it forwards the
// merchant's identity (tenantId, name, email, userId) from its own
// authenticated session, not from client input.

const TESSERIX_INTERNAL_URL =
  process.env.TESSERIX_INTERNAL_URL ?? "https://tesserix.app";

const INTERNAL_API_TOKEN = process.env.INTERNAL_API_TOKEN ?? "";

const PRODUCT_ID = "mark8ly";

function authHeaders(): Record<string, string> {
  // X-Internal-Token rather than Authorization Bearer — the
  // istio-ingress at tesserix.app has a RequestAuthentication policy
  // that parses Authorization Bearer as JWT and rejects our opaque
  // shared-secret token. A custom header bypasses that path.
  return {
    "Content-Type": "application/json",
    "X-Internal-Token": INTERNAL_API_TOKEN.trim(),
  };
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
  submitted_by_user_id: string | null;
  resolved_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface PlatformAnnouncement {
  id: string;
  title: string;
  body: string;
  severity: string;
  audience_filter: Record<string, unknown>;
  starts_at: string;
  ends_at: string | null;
  is_published: boolean;
  created_at: string;
  updated_at: string;
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

function configError(): Result<never> {
  return {
    ok: false,
    error: {
      code: "not_configured",
      message:
        "Platform support is not configured for this environment (missing INTERNAL_API_TOKEN).",
    },
  };
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
  if (!INTERNAL_API_TOKEN) return configError();

  try {
    const res = await fetch(
      `${TESSERIX_INTERNAL_URL}/api/internal/platform-tickets`,
      {
        method: "POST",
        headers: authHeaders(),
        cache: "no-store",
        body: JSON.stringify({
          productId: PRODUCT_ID,
          tenantId: input.tenantId,
          subject: input.subject,
          description: input.description,
          priority: input.priority,
          submittedByName: input.submittedByName,
          submittedByEmail: input.submittedByEmail,
          submittedByUserId: input.submittedByUserId,
        }),
      },
    );
    if (!res.ok) {
      const body = await readJsonSafely(res);
      const code = typeof body?.error === "string" ? body.error : `http_${res.status}`;
      return {
        ok: false,
        error: {
          code,
          message: messageForCode(code, res.status),
        },
      };
    }
    const body = (await res.json()) as { ticket: PlatformTicket };
    return { ok: true, data: body.ticket };
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
  if (!INTERNAL_API_TOKEN || !tenantId) return [];

  try {
    const url = new URL(
      `${TESSERIX_INTERNAL_URL}/api/internal/platform-tickets`,
    );
    url.searchParams.set("product", PRODUCT_ID);
    url.searchParams.set("tenant_id", tenantId);

    const res = await fetch(url.toString(), {
      headers: authHeaders(),
      cache: "no-store",
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { rows: PlatformTicket[] };
    return body.rows ?? [];
  } catch {
    return [];
  }
}

export async function listActivePlatformAnnouncements(
  tenantStatus: string = "active",
): Promise<PlatformAnnouncement[]> {
  if (!INTERNAL_API_TOKEN) return [];

  try {
    const url = new URL(
      `${TESSERIX_INTERNAL_URL}/api/internal/platform-announcements`,
    );
    url.searchParams.set("product", PRODUCT_ID);
    url.searchParams.set("tenant_status", tenantStatus);

    const res = await fetch(url.toString(), {
      headers: authHeaders(),
      cache: "no-store",
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { rows: PlatformAnnouncement[] };
    return body.rows ?? [];
  } catch {
    return [];
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
    case "db_unavailable":
      return "Tesserix is temporarily unavailable. Please try again in a moment.";
    default:
      return `Request failed (HTTP ${status}). Please try again.`;
  }
}
