"use server";

// Phase P — team settings server actions.
//
// These run in the admin server process, read the session headers
// that middleware has already populated, and forward to platform-api
// via the internal route. The client-side gates (hide the invite
// modal for low-privilege roles) are UX; the backend FGA check is
// the authoritative one.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  createInvitation,
  revokeInvitation,
  PlatformApiError,
  type TenantRole,
} from "@/lib/api/platform-api";
import { canInviteMembers } from "@/lib/auth/serverSession";

type Result =
  | { ok: true }
  | { ok: false; code: string; message: string };

function fail(err: unknown): { ok: false; code: string; message: string } {
  if (err instanceof PlatformApiError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return {
    ok: false,
    code: "unknown",
    message: err instanceof Error ? err.message : String(err),
  };
}

async function readAuth(): Promise<{
  tenantId: string;
  uid: string;
  role: TenantRole;
}> {
  const h = await headers();
  return {
    tenantId: h.get("x-session-tenant-id") ?? "",
    uid: h.get("x-session-user-id") ?? "",
    role: (h.get("x-session-role") ?? "viewer") as TenantRole,
  };
}

export async function inviteTeammate(input: {
  email: string;
  role: TenantRole;
}): Promise<Result> {
  const { tenantId, uid, role } = await readAuth();
  if (!tenantId || !uid) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    };
  }
  if (!canInviteMembers(role)) {
    return {
      ok: false,
      code: "forbidden",
      message: "You do not have permission to invite teammates.",
    };
  }
  const email = input.email.trim().toLowerCase();
  if (!email || !email.includes("@")) {
    return {
      ok: false,
      code: "invalid_email",
      message: "A valid email address is required.",
    };
  }
  if (input.role === "owner") {
    return {
      ok: false,
      code: "invalid_role",
      message: "Owner is reserved for the store founder.",
    };
  }
  try {
    await createInvitation(tenantId, { uid, email, role: input.role });
    revalidatePath("/settings/team");
    return { ok: true };
  } catch (err) {
    return fail(err);
  }
}

export async function revokeInvite(
  invitationId: string,
): Promise<Result> {
  const { tenantId, uid, role } = await readAuth();
  if (!tenantId || !uid) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    };
  }
  if (!canInviteMembers(role)) {
    return {
      ok: false,
      code: "forbidden",
      message: "You do not have permission to revoke invitations.",
    };
  }
  try {
    await revokeInvitation(tenantId, invitationId, uid);
    revalidatePath("/settings/team");
    return { ok: true };
  } catch (err) {
    return fail(err);
  }
}
