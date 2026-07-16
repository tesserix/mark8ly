import type { createApiClient } from "./client";
import {
  categoryListSchema,
  type Category,
  type CategoryListResponse,
} from "./schemas/categories";

/**
 * `GET /categories` (mobile_routes.go:95) takes no query params and returns
 * every category for the store in one `{data}` payload — there is no pagination
 * on this endpoint, so there is nothing to page through.
 */
export function createCategoriesApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: () =>
      client.get<CategoryListResponse>("/categories", undefined, categoryListSchema),
  };
}

export type { Category, CategoryListResponse };
