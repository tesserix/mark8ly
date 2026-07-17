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
 * Admin gift-card issue + read. Mirrors web routes.go:471-485 (no delete on
 * the backend). List is `{data, meta}`; issue returns `{data: card}` unwrapped;
 * get returns `{data: detail}` (card + transaction ledger) unwrapped.
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
  };
}

export type { GiftCard, GiftCardDetail, GiftCardListResponse };
