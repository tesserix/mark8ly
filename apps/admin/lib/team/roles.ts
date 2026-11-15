// Which roles the team page offers, and to whom. The backend re-checks
// every change; this only keeps the menus honest.

import type { InviteRole, TeamMember } from "@/lib/api/platform-api";

// Cashier is tenant-level only: the store type has no cashier relation.
export function inviteRoleOptions(scope: "tenant" | "store"): InviteRole[] {
  return scope === "tenant" ? ["admin", "staff", "viewer", "cashier"] : ["manager", "staff", "viewer"];
}

export function memberRoleOptions(currentRole: string | undefined): InviteRole[] {
  return currentRole === "owner" ? ["admin", "staff", "viewer", "cashier"] : ["staff", "viewer", "cashier"];
}

// Owner: anyone but themselves and the founder. Admin: staff, viewers and cashiers.
export function canEditMember(
  currentRole: string | undefined,
  m: Pick<TeamMember, "kind" | "email" | "role">,
  currentUserEmail: string | undefined,
): boolean {
  if (m.kind === "owner") return false;
  if (currentUserEmail && m.email.toLowerCase() === currentUserEmail.toLowerCase()) return false;
  if (currentRole === "owner") return true;
  if (currentRole === "admin") return m.role === "staff" || m.role === "viewer" || m.role === "cashier";
  return false;
}

export function roleLabel(role: string): string {
  if (role === "cashier") return "Cashier (in-store sales only)";
  return role.charAt(0).toUpperCase() + role.slice(1);
}
