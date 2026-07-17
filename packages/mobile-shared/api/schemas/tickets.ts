import { z } from "zod";
import { pageMeta } from "../schema-helpers";

/** A ticket reply — TicketReplyResponse (tickets_dto.go:41). `author_email`
 *  is omitempty → optional. */
export const ticketReplySchema = z.object({
  id: z.string(),
  // "customer" | "merchant" | "platform" | ...
  author_type: z.string(),
  author_name: z.string(),
  author_email: z.string().optional(),
  content: z.string(),
  created_at: z.string(),
});
export type TicketReply = z.infer<typeof ticketReplySchema>;

/**
 * A support ticket — TicketResponse (tickets_dto.go:25). Get/create/status all
 * return this BARE object; `replies` is ALWAYS a present array (the backend
 * initialises it to `[]`); `resolved_at` is omitempty → optional.
 */
export const ticketSchema = z.object({
  id: z.string(),
  ticket_number: z.string(),
  subject: z.string(),
  description: z.string(),
  // "open" | "pending" | "resolved" | "closed"
  status: z.string(),
  // "low" | "normal" | "high" | "urgent"
  priority: z.string(),
  submitted_by_name: z.string(),
  submitted_by_email: z.string(),
  resolved_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
  replies: z.array(ticketReplySchema),
});
export type Ticket = z.infer<typeof ticketSchema>;

/**
 * LIST envelope: `{data, meta, counts?}` (tickets.go:89). `counts` is the
 * per-status tally (CountByStatus) — a status→count map; kept optional.
 */
export const ticketListSchema = z.object({
  data: z.array(ticketSchema),
  meta: pageMeta,
  counts: z.record(z.string(), z.number()).optional(),
});
export type TicketListResponse = z.infer<typeof ticketListSchema>;
