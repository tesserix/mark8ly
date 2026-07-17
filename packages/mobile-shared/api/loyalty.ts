import type { createApiClient } from "./client";
import { enveloped, envelopedNullable } from "./schema-helpers";
import {
  loyaltyProgramSchema,
  loyaltyMemberListSchema,
  loyaltyMemberDetailSchema,
  referralListSchema,
  type LoyaltyProgram,
  type LoyaltyMemberListResponse,
  type LoyaltyMemberDetail,
  type ReferralListResponse,
} from "./schemas/loyalty";

export interface LoyaltyPageParams {
  page?: string;
  limit?: string;
}

/** Body for PUT /loyalty/program (UpdateLoyaltyProgramRequest, loyalty_dto.go:13). */
export interface UpdateLoyaltyProgramBody {
  is_active: boolean;
  points_per_unit: number;
  points_currency: string;
  signup_bonus?: number;
  referral_bonus?: number;
  referee_bonus?: number;
  point_expiry_days?: number | null;
  min_redeem_points: number;
  points_value: number;
  tiers?: Array<{ name: string; min_points: number; multiplier: number }>;
}

/** Body for POST /loyalty/members/:id/adjust (AdjustPointsRequest). Points non-zero. */
export interface AdjustPointsBody {
  points: number;
  description: string;
}

/**
 * Admin loyalty program + members + referrals. Mirrors web routes.go:487-513.
 * GetProgram is `{data: program | null}`; UpdateProgram returns `{data: program}`;
 * member/referral lists use loyalty's own `{data, meta:{total,page,limit}}`;
 * GetMember nests `{data: member, transactions:{data,meta}}`; adjust → `{message}`.
 */
export function createLoyaltyApi(client: ReturnType<typeof createApiClient>) {
  return {
    getProgram: () =>
      client
        .get<{ data: LoyaltyProgram | null }>(
          "/loyalty/program",
          undefined,
          envelopedNullable(loyaltyProgramSchema),
        )
        .then((r) => r.data),
    updateProgram: (body: UpdateLoyaltyProgramBody) =>
      client
        .put<{ data: LoyaltyProgram }>("/loyalty/program", body, enveloped(loyaltyProgramSchema))
        .then((r) => r.data),
    listMembers: (params?: LoyaltyPageParams) =>
      client.get<LoyaltyMemberListResponse>(
        "/loyalty/members",
        params as Record<string, string>,
        loyaltyMemberListSchema,
      ),
    getMember: (id: string) =>
      client.get<LoyaltyMemberDetail>(
        `/loyalty/members/${id}`,
        undefined,
        loyaltyMemberDetailSchema,
      ),
    adjustPoints: (id: string, body: AdjustPointsBody) =>
      client.post<{ message: string }>(`/loyalty/members/${id}/adjust`, body),
    listReferrals: (params?: LoyaltyPageParams) =>
      client.get<ReferralListResponse>(
        "/loyalty/referrals",
        params as Record<string, string>,
        referralListSchema,
      ),
  };
}

export type { LoyaltyProgram, LoyaltyMemberListResponse, LoyaltyMemberDetail, ReferralListResponse };
