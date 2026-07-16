import { useCallback } from "react";
import { Alert } from "react-native";
import * as Haptics from "expo-haptics";
import { ApiError } from "@repo/mobile-shared/api/client";
import type { ProductDetail } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateProductOptionBody } from "@repo/mobile-shared/api/products";
import type { useUpdateProduct } from "@/lib/admin-api/product-crud";
import { buildOptionMatrix, OptionMatrixError } from "@/lib/option-matrix";
import { toOptionRequestBodies } from "@/components/products/OptionsEditor";

/**
 * Builds the "add an option axis" handler for the product detail screen.
 *
 * `buildOptionMatrix` is the ONLY safe way to produce a `variants` PATCH body
 * — it is a FULL DESIRED MATRIX, and the backend soft-deletes any existing
 * variant omitted from it. An `OptionMatrixError` (ambiguous or malformed
 * axis) surfaces as an alert instead of crashing or sending a partial
 * matrix; a mutation failure (network/API) surfaces the same way.
 *
 * Lives outside app/(tabs)/products/[id].tsx on purpose: that screen has a
 * pinned line-count regression test (__tests__/product-detail-sections.test.tsx),
 * and this is the kind of orchestration logic the screen already delegates
 * out (see computeReorderWrites, uploadProductMedia).
 */
export function useAddOptionHandler(
  id: string,
  product: ProductDetail | undefined,
  updateMutation: ReturnType<typeof useUpdateProduct>,
) {
  return useCallback(
    (option: UpdateProductOptionBody) => {
      if (!product) return;
      try {
        const existing = toOptionRequestBodies(product.options);
        const { options, variants } = buildOptionMatrix(product, [...existing, option]);
        updateMutation.mutate(
          { id, body: { options, variants } },
          {
            onSuccess: () => {
              void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
            },
            onError: (err: unknown) => {
              Alert.alert(
                "Error",
                err instanceof ApiError ? err.message : "Failed to add option. Please try again.",
              );
            },
          },
        );
      } catch (err) {
        Alert.alert(
          "Can't add option",
          err instanceof OptionMatrixError ? err.message : "That option can't be added safely.",
        );
      }
    },
    [id, product, updateMutation],
  );
}
