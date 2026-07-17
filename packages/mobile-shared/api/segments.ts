import type { createApiClient } from "./client";
import { enveloped } from "./schema-helpers";
import {
  segmentSchema,
  segmentListSchema,
  type Segment,
  type SegmentListResponse,
} from "./schemas/segments";

/** Body for POST /segments (CreateSegmentRequest, segments_dto.go:10). */
export interface CreateSegmentBody {
  name: string;
  description?: string;
  /** JSON array string of rules — sent verbatim. */
  rules: string;
}

/** Body for PATCH /segments/:id (UpdateSegmentRequest — name + rules required). */
export interface UpdateSegmentBody {
  name: string;
  description?: string;
  rules: string;
}

/**
 * Admin customer-segment CRUD. Mirrors web routes.go:615-635. List is bare
 * `{data}` (no meta); get + mutations return `{data: segment}` unwrapped.
 */
export function createSegmentsApi(client: ReturnType<typeof createApiClient>) {
  const env = enveloped(segmentSchema);
  const unwrap = (p: Promise<{ data: Segment }>) => p.then((r) => r.data);
  return {
    list: () => client.get<SegmentListResponse>("/segments", undefined, segmentListSchema),
    get: (id: string) =>
      unwrap(client.get<{ data: Segment }>(`/segments/${id}`, undefined, env)),
    create: (body: CreateSegmentBody) =>
      unwrap(client.post<{ data: Segment }>("/segments", body, env)),
    update: (id: string, body: UpdateSegmentBody) =>
      unwrap(client.patch<{ data: Segment }>(`/segments/${id}`, body, env)),
    remove: (id: string) => client.delete<{ message?: string }>(`/segments/${id}`),
  };
}

export type { Segment, SegmentListResponse };
