import type { createApiClient } from "./client";
import type { DashboardResponse } from "./types";

export function createDashboardApi(client: ReturnType<typeof createApiClient>) {
  return {
    get: () => client.get<DashboardResponse>("/dashboard"),
  };
}
