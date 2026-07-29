import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import { useApiClient } from "@/lib/api-client";

/**
 * The three statuses the backend accepts, as a closed union.
 *
 * The wire schema types `status` as a bare `z.string()` (see
 * api/schemas/products.ts) — deliberately, because it is parsing a RESPONSE
 * and a server that grows a fourth status must not blank the merchant's
 * catalogue. A REQUEST is the opposite problem: `binding:"omitempty,oneof=
 * draft active archived"` (validation.go:307) rejects anything else with a
 * 400, so the narrow type belongs here, on the way out.
 *
 * "inactive" is the one that has actually shipped broken: it is not a status,
 * and `?status=inactive` was a hard 400 that emptied the Draft tab.
 */
export type ProductStatus = "draft" | "active" | "archived";

export interface SetProductStatusVars {
  id: string;
  status: ProductStatus;
}

/**
 * Flips one product's status.
 *
 * A thin, single-purpose wrapper over the same `PATCH /products/:id`
 * `useUpdateProduct` uses (lib/admin-api/product-crud.ts), and it inherits
 * that call's invalidation deliberately:
 *
 *   ["products"]      → prefix-matches the list key ["products", status,
 *                       search], so the Products screen refetches itself and
 *                       stays the authority on its own contents. That is why
 *                       the screen needs no optimistic hide.
 *   ["product", id]   → the detail screen, which is NOT under that prefix.
 *
 * NOT ["dashboard"], and that is checked rather than assumed: the dashboard
 * payload's only product-shaped blocks are `top_products` (sales-derived) and
 * `low_stock`, whose query filters on `pv.deleted_at`/`p.deleted_at` and
 * inventory quantity and never reads `p.status` (dashboard.go:145-163). A
 * status change moves nothing on it, so invalidating it would be a refetch
 * for nothing.
 *
 * Separate from `useUpdateProduct` rather than folded into it because the
 * two have different blast radii: this one is fired from a swipe, by a thumb,
 * with no form and no confirmation step behind it except the one the caller
 * puts there.
 */
export function useSetProductStatus() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, status }: SetProductStatusVars) =>
      productsApi.update(id, { status }),
    // No `return` on these — react-query would then AWAIT the refetches
    // before settling the mutation, which keeps `isPending` true until fresh
    // data lands. See the guard test in __tests__/mutation-invalidations.
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["product", variables.id] });
    },
  });
}
