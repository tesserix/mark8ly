import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createProductsApi, type UpdateVariantBody } from "@repo/mobile-shared/api/products";
import { useApiClient } from "@/lib/api-client";

/**
 * The two variant fields a merchant can change from the Products LIST, with
 * a thumb, without opening the editor.
 *
 * Named for the wire, not for the UI: `inventory_quantity` is what
 * `UpdateVariantRequest` calls it. `stock` is not a field on that request and
 * a body using it is discarded with a cheerful 200 — a bug this app has
 * already shipped once (see `createProductsApi.updateVariant`).
 */
export type VariantEditField = "price" | "inventory_quantity";

export interface QuickEditVariantVars {
  productId: string;
  variantId: string;
  field: VariantEditField;
  value: number;
}

/**
 * The PATCH body for one field change — and ONLY that field.
 *
 * Every field on `UpdateVariantRequest` (validation.go:43-58) is an optional
 * pointer, so the server cannot tell "the merchant left this alone" from "set
 * it back to this". The phone's copy of a variant is as old as the list it was
 * rendered from, so a body that helpfully echoes back the other twelve fields
 * silently reverts anything changed in the web admin since that list loaded.
 *
 * Built here rather than at the call site so there is exactly one place that
 * decides what goes on the wire, and one place a test can pin it.
 */
export function variantPatchBody(field: VariantEditField, value: number): UpdateVariantBody {
  return field === "price" ? { price: value } : { inventory_quantity: value };
}

/**
 * One field, one variant, one PATCH — the list-screen counterpart to
 * `useUpdateVariant` in product-crud.ts.
 *
 * Deliberately NOT the same hook. `useUpdateVariant` takes a caller-composed
 * `UpdateVariantBody` because the product editor legitimately writes several
 * variant fields at once from a form the merchant can see in full; this one
 * takes a FIELD and a VALUE precisely so a list-screen caller cannot compose
 * a body at all. It also invalidates one key more:
 *
 *   ["products"]     → prefix-matches the list key ["products", status,
 *                      search], so the Products screen refetches itself and
 *                      stays the authority on the new price. That is why the
 *                      sheet needs no optimistic update.
 *   ["product", id]  → the detail screen, which is NOT under that prefix.
 *
 * NOT ["dashboard"]: its only product-shaped blocks are `top_products`
 * (sales-derived) and `low_stock`, and a merchant watching one variant's
 * number change is not asking for the heaviest payload in the app to be
 * refetched behind them.
 *
 * No `return` on the invalidations — react-query would then AWAIT the
 * refetches before settling the mutation, keeping `isPending` true (and the
 * sheet's spinner up) until fresh list data landed.
 */
export function useQuickEditVariant() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ productId, variantId, field, value }: QuickEditVariantVars) =>
      productsApi.updateVariant(productId, variantId, variantPatchBody(field, value)),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["product", variables.productId] });
    },
  });
}
