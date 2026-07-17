import { z } from "zod";

/**
 * Team wire shapes — mirror marketplace-api's teamproxy structs, which in turn
 * mirror platform-api's invitation.Member / invitation.Invitation. Everything
 * is wrapped in `{data: ...}` by the proxy handler. `accepted_at` is a Go
 * `*time.Time` + omitempty → ABSENT when null → `.optional()`.
 */
export const teamMemberSchema = z.object({
  email: z.string(),
  // "owner" | "admin" | "staff" | "viewer"
  role: z.string(),
  // "owner" | "invited"
  kind: z.string(),
  accepted_at: z.string().optional(),
});
export type TeamMember = z.infer<typeof teamMemberSchema>;

export const teamInvitationSchema = z.object({
  id: z.string(),
  store_id: z.string().optional(),
  email: z.string(),
  role: z.string(),
  expires_at: z.string(),
  // "pending" | "accepted" | "expired" | "revoked"
  status: z.string(),
  created_at: z.string(),
  accepted_at: z.string().optional(),
});
export type TeamInvitation = z.infer<typeof teamInvitationSchema>;

/** `{data: [...]}` list envelopes (the proxy handler wraps in data). */
export const teamMemberListSchema = z.object({ data: z.array(teamMemberSchema) });
export type TeamMemberListResponse = z.infer<typeof teamMemberListSchema>;
export const teamInvitationListSchema = z.object({ data: z.array(teamInvitationSchema) });
export type TeamInvitationListResponse = z.infer<typeof teamInvitationListSchema>;

/** Roles the mobile UI can assign (owner is excluded — it's transfer-only). */
export const ASSIGNABLE_ROLES = ["admin", "staff", "viewer"] as const;
export type AssignableRole = (typeof ASSIGNABLE_ROLES)[number];
