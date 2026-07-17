import type { createApiClient } from "./client";
import { enveloped } from "./schema-helpers";
import {
  campaignSchema,
  campaignListSchema,
  type Campaign,
  type CampaignListResponse,
} from "./schemas/campaigns";

export interface ListCampaignsParams {
  status?: string;
  page?: string;
  page_size?: string;
}

/** Body for POST /campaigns (CreateCampaignRequest, campaigns_dto.go:12). */
export interface CreateCampaignBody {
  name: string;
  type?: string;
  subject?: string;
  content?: string;
  segment_id?: string;
  coupon_id?: string;
  show_on_storefront?: boolean;
  storefront_label?: string;
  storefront_priority?: number;
}

/** Body for PATCH /campaigns/:id (UpdateCampaignRequest, campaigns_dto.go:25). */
export interface UpdateCampaignBody {
  name?: string;
  subject?: string;
  content?: string;
  segment_id?: string;
  coupon_id?: string;
  show_on_storefront?: boolean;
  storefront_label?: string;
  storefront_priority?: number;
}

/**
 * Admin campaign management + lifecycle. Mirrors web routes.go:531-563.
 * List is `{data, meta}`; get + every mutation/lifecycle action returns
 * `{data: campaign}` which we unwrap. Send can 429 (budget/slot) or 403.
 */
export function createCampaignsApi(client: ReturnType<typeof createApiClient>) {
  const env = enveloped(campaignSchema);
  const unwrap = (p: Promise<{ data: Campaign }>) => p.then((r) => r.data);
  return {
    list: (params?: ListCampaignsParams) =>
      client.get<CampaignListResponse>(
        "/campaigns",
        params as Record<string, string>,
        campaignListSchema,
      ),
    get: (id: string) =>
      unwrap(client.get<{ data: Campaign }>(`/campaigns/${id}`, undefined, env)),
    create: (body: CreateCampaignBody) =>
      unwrap(client.post<{ data: Campaign }>("/campaigns", body, env)),
    patch: (id: string, body: UpdateCampaignBody) =>
      unwrap(client.patch<{ data: Campaign }>(`/campaigns/${id}`, body, env)),
    remove: (id: string) => client.delete<{ message?: string }>(`/campaigns/${id}`),
    send: (id: string) => unwrap(client.post<{ data: Campaign }>(`/campaigns/${id}/send`, {}, env)),
    schedule: (id: string, scheduledAt: string) =>
      unwrap(
        client.post<{ data: Campaign }>(
          `/campaigns/${id}/schedule`,
          { scheduled_at: scheduledAt },
          env,
        ),
      ),
    pause: (id: string) => unwrap(client.post<{ data: Campaign }>(`/campaigns/${id}/pause`, {}, env)),
    resume: (id: string) =>
      unwrap(client.post<{ data: Campaign }>(`/campaigns/${id}/resume`, {}, env)),
  };
}

export type { Campaign, CampaignListResponse };
