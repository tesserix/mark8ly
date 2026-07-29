import type { createApiClient } from "./client";
import { enveloped } from "./schema-helpers";
import {
  giftCardDetailSchema,
  giftCardListSchema,
  type GiftCard,
  type GiftCardDetail,
  type GiftCardListResponse,
} from "./schemas/gift-cards";
import { giftCardSchema } from "./schemas/gift-cards";

export interface ListGiftCardsParams {
  status?: string;
  page?: string;
  page_size?: string;
}

/** Body for POST /gift-cards (IssueGiftCardRequest, gift_cards_dto.go:12). */
export interface IssueGiftCardBody {
  initial_balance: number;
  currency_code: string;
  sender_name?: string;
  sender_email?: string;
  recipient_name?: string;
  recipient_email?: string;
  message?: string;
  expires_at?: string;
}

/**
 * Admin gift-card issue, read and enable/disable. Mirrors the mobile route
 * table (`mobile_routes.go`, itself mirroring web `routes.go`). There is
 * deliberately NO delete: the backend exposes none, and one would CASCADE
 * the transaction ledger — including rows that reference real orders.
 *
 * List is `{data, meta}`; issue/enable/disable return `{data: card}`
 * unwrapped; get returns `{data: detail}` (card + ledger) unwrapped.
 */
export function createGiftCardsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListGiftCardsParams) =>
      client.get<GiftCardListResponse>(
        "/gift-cards",
        params as Record<string, string>,
        giftCardListSchema,
      ),
    get: (id: string) =>
      client
        .get<{ data: GiftCardDetail }>(`/gift-cards/${id}`, undefined, enveloped(giftCardDetailSchema))
        .then((r) => r.data),
    issue: (body: IssueGiftCardBody) =>
      client
        .post<{ data: GiftCard }>("/gift-cards", body, enveloped(giftCardSchema))
        .then((r) => r.data),
    /**
     * Freeze a card's remaining balance. NO request body — the handler reads
     * only the path params.
     *
     * Reversible and idempotent: re-enabling restores the balance in full,
     * and asking for the state the card is already in is a 200, not a 409
     * (repository.go `classifyStatusTransition`). Refused with 409
     * `invalid_transition` from any other status, and 410 `gift_card_expired`
     * once `expires_at` has passed.
     */
    disable: (id: string) =>
      client
        .post<{ data: GiftCard }>(`/gift-cards/${id}/disable`, undefined, enveloped(giftCardSchema))
        .then((r) => r.data),
    /** The exact inverse of `disable`, with the same refusals. */
    enable: (id: string) =>
      client
        .post<{ data: GiftCard }>(`/gift-cards/${id}/enable`, undefined, enveloped(giftCardSchema))
        .then((r) => r.data),
  };
}

export type { GiftCard, GiftCardDetail, GiftCardListResponse };
