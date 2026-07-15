import type { createApiClient } from "./client";
import { dashboardResponseSchema } from "./schemas/dashboard";

export function createDashboardApi(client: ReturnType<typeof createApiClient>) {
  return {
    get: () => client.get("/dashboard", undefined, dashboardResponseSchema),
  };
}
