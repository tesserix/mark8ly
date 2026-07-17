import type { createApiClient } from "./client";
import {
  categoryListSchema,
  categoryCreateResponseSchema,
  type Category,
  type CategoryListResponse,
  type CreateCategoryBody,
  type CategoryCreateResponse,
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
    create: (body: CreateCategoryBody) =>
      client.post<CategoryCreateResponse>("/categories", body, categoryCreateResponseSchema),
  };
}

export type { Category, CategoryListResponse, CreateCategoryBody, CategoryCreateResponse };
