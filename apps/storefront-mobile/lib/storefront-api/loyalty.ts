import type { ApiClient } from "./client-provider";

export interface LoyaltyProgram {
  id: string;
  name: string;
  tiers: LoyaltyTier[];
  points_per_currency: number;
  currency_per_point: number;
}

export interface LoyaltyTier {
  id: string;
  name: string;
  min_points: number;
  multiplier: number;
}

export interface LoyaltyMembership {
  enrolled: boolean;
  points_balance: number;
  lifetime_points: number;
  current_tier: string;
  next_tier?: string;
  points_to_next_tier?: number;
  referral_code: string;
  history: LoyaltyTransaction[];
}

export interface LoyaltyTransaction {
  id: string;
  type: "earned" | "redeemed" | "expired" | "adjusted";
  points: number;
  description: string;
  created_at: string;
}

interface LoyaltyProgramResponse {
  data: LoyaltyProgram;
}

interface LoyaltyMembershipResponse {
  data: LoyaltyMembership;
}

interface EnrollResponse {
  data: { enrolled: boolean };
}

export function getLoyaltyProgram(
  client: ApiClient,
): Promise<LoyaltyProgramResponse> {
  return client.get<LoyaltyProgramResponse>("/loyalty/program");
}

export function getLoyaltyMe(
  client: ApiClient,
): Promise<LoyaltyMembershipResponse> {
  return client.get<LoyaltyMembershipResponse>("/loyalty/me");
}

export function enrollLoyalty(
  client: ApiClient,
): Promise<EnrollResponse> {
  return client.post<EnrollResponse>("/loyalty/enroll", {});
}
