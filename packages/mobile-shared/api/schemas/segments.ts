import { z } from "zod";

/**
 * Wire truth for customer segments — SegmentResponse (segments_dto.go:24-32).
 * `description` is a pointer WITHOUT omitempty → JSON `null` → `.nullable()`.
 * `rules` is a JSON-array STRING (the backend stores + returns it verbatim);
 * the mobile UI treats it as opaque text, not a parsed structure.
 */
export const segmentSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string().nullable(),
  rules: z.string(),
  member_count: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Segment = z.infer<typeof segmentSchema>;

/** LIST envelope: bare `{data}` — segments.go:39 sends NO meta. */
export const segmentListSchema = z.object({ data: z.array(segmentSchema) });
export type SegmentListResponse = z.infer<typeof segmentListSchema>;
