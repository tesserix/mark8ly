// apps/admin/lib/api/campaigns-api.ts
//
// Admin campaign + segment API client. Follows the same calling
// convention as coupons-api.ts: server components pass SessionHeaders,
// the client does the header rename dance.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

function authHeaders(session: SessionHeaders): Record<string, string> {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    "Content-Type": "application/json",
    Accept: "application/json",
  };
}

// ---------- Types ----------

export type CampaignStatus =
  | "draft"
  | "scheduled"
  | "sending"
  | "sent"
  | "paused"
  | "cancelled";

export interface AdminCampaign {
  id: string;
  name: string;
  type: string;
  status: CampaignStatus;
  subject: string | null;
  content: string | null;
  segment_id: string | null;
  coupon_id: string | null;
  scheduled_at: string | null;
  sent_at: string | null;
  total_recipients: number;
  delivered: number;
  opened: number;
  clicked: number;
  converted: number;
  unsubscribed: number;
  failed: number;
  revenue: string;
  created_at: string;
  updated_at: string;
}

export interface AdminSegment {
  id: string;
  name: string;
  description: string | null;
  rules: string;
  member_count: number;
  created_at: string;
  updated_at: string;
}

export interface ListCampaignsQuery {
  status?: string;
  page?: number;
  per_page?: number;
}

export interface ListCampaignsResponse {
  data: AdminCampaign[];
  meta: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

export interface CreateCampaignBody {
  name: string;
  type?: string;
  subject?: string;
  content?: string;
  segment_id?: string;
  coupon_id?: string;
}

export interface UpdateCampaignBody {
  name?: string;
  subject?: string;
  content?: string;
  segment_id?: string;
  coupon_id?: string;
}

export interface CreateSegmentBody {
  name: string;
  description?: string;
  rules: string;
}

// ---------- Campaign API functions ----------

export async function listCampaigns(
  storeId: string,
  query: ListCampaignsQuery,
  session: SessionHeaders,
): Promise<ListCampaignsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.page) params.set("page", String(query.page));
  if (query.per_page) params.set("per_page", String(query.per_page));

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns?${params}`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return null;
    return (await res.json()) as ListCampaignsResponse;
  } catch {
    return null;
  }
}

export async function getCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<AdminCampaign | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return null;
    const json = await res.json();
    return (json.data ?? json) as AdminCampaign;
  } catch {
    return null;
  }
}

export async function createCampaign(
  storeId: string,
  body: CreateCampaignBody,
  session: SessionHeaders,
): Promise<{ data: AdminCampaign } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCampaign };
  } catch {
    return null;
  }
}

export async function updateCampaign(
  storeId: string,
  campaignId: string,
  body: UpdateCampaignBody,
  session: SessionHeaders,
): Promise<{ data: AdminCampaign } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}`;
  try {
    const res = await fetch(url, {
      method: "PATCH",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCampaign };
  } catch {
    return null;
  }
}

export async function deleteCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}`;
  try {
    const res = await fetch(url, {
      method: "DELETE",
      headers: authHeaders(session),
    });
    return res.ok;
  } catch {
    return false;
  }
}

export async function sendCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<{ data: AdminCampaign } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}/send`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCampaign };
  } catch {
    return null;
  }
}

export async function scheduleCampaign(
  storeId: string,
  campaignId: string,
  scheduledAt: string,
  session: SessionHeaders,
): Promise<{ data: AdminCampaign } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}/schedule`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
      body: JSON.stringify({ scheduled_at: scheduledAt }),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCampaign };
  } catch {
    return null;
  }
}

export async function pauseCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<{ data: AdminCampaign } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}/pause`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCampaign };
  } catch {
    return null;
  }
}

export async function resumeCampaign(
  storeId: string,
  campaignId: string,
  session: SessionHeaders,
): Promise<{ data: AdminCampaign } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/campaigns/${campaignId}/resume`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCampaign };
  } catch {
    return null;
  }
}

// ---------- Segment API functions ----------

export async function listSegments(
  storeId: string,
  session: SessionHeaders,
): Promise<{ data: AdminSegment[] } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/segments`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminSegment[] };
  } catch {
    return null;
  }
}

export async function createSegment(
  storeId: string,
  body: CreateSegmentBody,
  session: SessionHeaders,
): Promise<{ data: AdminSegment } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/segments`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminSegment };
  } catch {
    return null;
  }
}

export async function deleteSegment(
  storeId: string,
  segmentId: string,
  session: SessionHeaders,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/segments/${segmentId}`;
  try {
    const res = await fetch(url, {
      method: "DELETE",
      headers: authHeaders(session),
    });
    return res.ok;
  } catch {
    return false;
  }
}
