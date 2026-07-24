import type { createApiClient } from "./client";

export function createAccountApi(client: ReturnType<typeof createApiClient>) {
  return {
    // DELETE /api/v1/mobile/admin/account (tenant-scoped). 204 → void.
    deleteAccount: () => client.deleteTenant<void>("/account"),
  };
}
