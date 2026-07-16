import { useCallback } from "react";
import { Alert } from "react-native";
import * as ImagePicker from "expo-image-picker";
import type { ProductDetail } from "@repo/mobile-shared/api/schemas/products";
import type {
  useAddProductMedia,
  useDeleteMedia,
  useUpdateMedia,
} from "@/lib/admin-api/product-crud";
import { computeReorderWrites } from "@/components/products/MediaGrid";
import { getErrorMessage, alertOnError } from "@/lib/product-alerts";

interface UseProductMediaHandlersArgs {
  id: string;
  product: ProductDetail | undefined;
  addMediaMutation: ReturnType<typeof useAddProductMedia>;
  deleteMediaMutation: ReturnType<typeof useDeleteMedia>;
  updateMediaMutation: ReturnType<typeof useUpdateMedia>;
}

/**
 * The four media handlers for the product detail screen, extracted verbatim
 * out of app/(tabs)/products/[id].tsx (no behaviour change) so that screen
 * stays under its pinned line-count regression test
 * (__tests__/product-detail-sections.test.tsx). This mirrors the existing
 * lib/hooks/use-add-option-handler.ts pattern of moving screen orchestration
 * out into a hook.
 */
export function useProductMediaHandlers({
  id,
  product,
  addMediaMutation,
  deleteMediaMutation,
  updateMediaMutation,
}: UseProductMediaHandlersArgs) {
  const handleDeleteExistingMedia = useCallback(
    (mediaId: string) => {
      Alert.alert("Delete Image", "Remove this image from the product?", [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () =>
            deleteMediaMutation.mutate(
              { productId: id, mediaId },
              alertOnError("Failed to delete image. Please try again."),
            ),
        },
      ]);
    },
    [id, deleteMediaMutation],
  );

  const handleAddMedia = useCallback(async () => {
    try {
      // Deliberately no library-permission request here (pinned by the
      // regression test in __tests__/add-product-media.test.tsx).
      // launchImageLibraryAsync uses the system picker (PHPicker on iOS), which
      // runs out-of-process and needs no library permission. Asking anyway opts
      // into the legacy permission flow: choosing "Limited Access" drops the
      // user into iOS's limited-library management sheet — a grid with an X and
      // no confirm button — from which the real picker never opens. Observed on
      // a simulator. `components/ProductMediaPicker.tsx` has always called the
      // picker directly for the same reason.
      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ["images"],
        quality: 0.8,
        // The system cropper, shown after the photo is chosen and before upload.
        // Orthogonal to permission — see the comment above; do NOT add a
        // permission request alongside it.
        allowsEditing: true,
      });
      if (result.canceled) return;

      const asset = result.assets[0];
      if (!asset) return;

      const currentMediaCount = product?.media.length ?? 0;
      addMediaMutation.mutate(
        {
          productId: id,
          asset: {
            uri: asset.uri,
            fileName: asset.fileName,
            fileSize: asset.fileSize,
            mimeType: asset.mimeType,
          },
          position: currentMediaCount,
        },
        {
          onError: (err) => {
            // A silent upload failure is the exact bug class this project
            // exists to kill — always surface it.
            Alert.alert(
              "Error",
              getErrorMessage(err, "Failed to upload image. Please try again."),
            );
          },
        },
      );
    } catch (err) {
      // launchImageLibraryAsync can itself reject (platform picker errors).
      // onPress doesn't await this handler, so an uncaught rejection here
      // would be a silent failure — the exact class this project exists to kill.
      Alert.alert("Error", getErrorMessage(err, "Failed to open the image picker."));
    }
  }, [id, product?.media.length, addMediaMutation]);

  const handleReorderMedia = useCallback(
    (mediaId: string, newPosition: number) => {
      // 🔴 Adjacent SWAP, not a single-row write. The backend does not shift
      // siblings, so a lone position PATCH would leave two photos sharing a
      // slot (see computeReorderWrites). Move both rows, or neither.
      const writes = computeReorderWrites(product?.media ?? [], mediaId, newPosition);
      for (const write of writes) {
        updateMediaMutation.mutate(
          { productId: id, mediaId: write.id, body: { position: write.position } },
          alertOnError("Failed to reorder photos. Please try again."),
        );
      }
    },
    [id, product?.media, updateMediaMutation],
  );

  const handleAltChange = useCallback(
    (mediaId: string, alt: string) => {
      updateMediaMutation.mutate(
        { productId: id, mediaId, body: { alt } },
        alertOnError("Failed to update alt text. Please try again."),
      );
    },
    [id, updateMediaMutation],
  );

  return { handleAddMedia, handleDeleteExistingMedia, handleReorderMedia, handleAltChange };
}
