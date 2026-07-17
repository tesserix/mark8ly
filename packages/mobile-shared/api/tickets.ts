import type { createApiClient } from "./client";
import {
  ticketSchema,
  ticketReplySchema,
  ticketListSchema,
  type Ticket,
  type TicketReply,
  type TicketListResponse,
} from "./schemas/tickets";

export interface ListTicketsParams {
  status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

/** Body for POST /tickets (CreateTicketRequest). `priority` optional. */
export interface CreateTicketBody {
  subject: string;
  description: string;
  priority?: string;
}

/**
 * Support tickets. Mirrors web routes.go:802-820. List is `{data, meta, counts?}`;
 * get/create/status return the BARE ticket; reply returns the BARE reply.
 */
export function createTicketsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListTicketsParams) =>
      client.get<TicketListResponse>("/tickets", params as Record<string, string>, ticketListSchema),
    get: (id: string) => client.get<Ticket>(`/tickets/${id}`, undefined, ticketSchema),
    create: (body: CreateTicketBody) => client.post<Ticket>("/tickets", body, ticketSchema),
    reply: (id: string, content: string) =>
      client.post<TicketReply>(`/tickets/${id}/reply`, { content }, ticketReplySchema),
    updateStatus: (id: string, status: string) =>
      client.patch<Ticket>(`/tickets/${id}`, { status }, ticketSchema),
  };
}

export type { Ticket, TicketReply, TicketListResponse };
