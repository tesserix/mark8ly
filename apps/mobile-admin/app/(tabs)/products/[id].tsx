import { useState, useCallback, useEffect, useRef } from "react";
import {
  View,
  ScrollView,
  TextInput,
  Image,
  Switch,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
  Modal,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import * as ImagePicker from "expo-image-picker";
import { useLocalSearchParams } from "expo-router";
import { useProduct } from "../../../lib/hooks/use-products";
import {
  useUpdateProduct,
  useDeleteMedia,
  useUpdateVariant,
  useAddProductMedia,
} from "../../../lib/admin-api/product-crud";
import {
  BackHeader,
  Card,
  Eyebrow,
  Hairline,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductVariant } from "@repo/mobile-shared/api/types";
import type { UpdateVariantBody } from "@repo/mobile-shared/api/products";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useDockClearance } from "@/components/navigation/dock-metrics";

/** How long the transient "Saved" acknowledgement stays visible. */
const SAVED_ACKNOWLEDGEMENT_MS = 2000;

/**
 * This branch went to real trouble to make contract-mismatch messages name
 * the offending field (see client.ts's ApiError construction) — surface that
 * instead of a generic string wherever we have it.
 */
function getErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message;
  return fallback;
}

function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textSecondary" style={styles.fieldLabel}>
      {label}
    </Text>
  );
}

/**
 * The wire has no variant name. A variant is described by its option values
 * ("M / Blue"); every product's option_values is `[]` today, so the SKU —
 * which every real variant has — is the honest fallback.
 */
function variantLabel(variant: ProductVariant): string {
  if (variant.option_values.length > 0) {
    return variant.option_values.map((o) => o.value).join(" / ");
  }
  return variant.sku;
}

interface VariantRowProps {
  variant: ProductVariant;
  onUpdate: (variantId: string, body: UpdateVariantBody) => void;
}

function VariantRow({ variant, onUpdate }: VariantRowProps) {
  const [price, setPrice] = useState(String(variant.price));
  const [stock, setStock] = useState(String(variant.inventory_quantity));
  const label = variantLabel(variant);

  const handleBlurPrice = () => {
    const parsed = parseFloat(price);
    if (!isNaN(parsed) && parsed !== variant.price) onUpdate(variant.id, { price: parsed });
  };
  const handleBlurStock = () => {
    const parsed = parseInt(stock, 10);
    // `inventory_quantity`, NOT `stock` — UpdateVariantRequest has no `stock`
    // field, so the old body's stock edits were discarded with a 200.
    if (!isNaN(parsed) && parsed !== variant.inventory_quantity) {
      onUpdate(variant.id, { inventory_quantity: parsed });
    }
  };

  return (
    <View style={styles.variantRow}>
      <Text preset="bodyEmphasis" color="text">
        {label}
      </Text>
      <Text preset="caption" color="textTertiary">
        SKU · {variant.sku}
      </Text>
      <View style={styles.variantFields}>
        <View style={styles.variantField}>
          <FieldLabel label={`Price (${variant.currency_code})`} />
          <TextInput
            style={styles.variantInput}
            value={price}
            onChangeText={setPrice}
            onBlur={handleBlurPrice}
            keyboardType="decimal-pad"
            accessibilityLabel={`${label} price`}
          />
        </View>
        <View style={styles.variantField}>
          <FieldLabel label="Stock" />
          <TextInput
            style={styles.variantInput}
            value={stock}
            onChangeText={setStock}
            onBlur={handleBlurStock}
            keyboardType="number-pad"
            accessibilityLabel={`${label} stock`}
          />
        </View>
      </View>
    </View>
  );
}

export default function ProductDetailScreen() {
  const dockPad = useDockClearance();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: product, isLoading, error } = useProduct(id);

  const updateMutation = useUpdateProduct();
  const deleteMediaMutation = useDeleteMedia();
  const updateVariantMutation = useUpdateVariant();
  const addMediaMutation = useAddProductMedia();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [initialized, setInitialized] = useState(false);
  const [justSaved, setJustSaved] = useState(false);
  const [viewerImage, setViewerImage] = useState<{ uri: string; alt?: string } | null>(null);
  const savedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // A `setTimeout` firing after unmount would set state on a dead component.
  useEffect(() => {
    return () => {
      if (savedTimerRef.current) clearTimeout(savedTimerRef.current);
    };
  }, []);

  if (product && !initialized) {
    setTitle(product.title);
    setDescription(product.description ?? "");
    setIsActive(product.status === "active");
    setInitialized(true);
  }

  const handleSave = useCallback(() => {
    if (!title.trim()) {
      Alert.alert("Validation", "Product title is required.");
      return;
    }
    updateMutation.mutate(
      {
        id,
        body: {
          title: title.trim(),
          description: description.trim() || undefined,
          // The enum is draft|active|archived — "inactive" is a hard 400.
          status: isActive ? "active" : "draft",
        },
      },
      {
        onSuccess: () => {
          // Save -> Saving… -> Saved (~2s) -> Save. Success is otherwise
          // invisible: react-query refetches identical data, so nothing on
          // screen changes without this acknowledgement.
          setJustSaved(true);
          if (savedTimerRef.current) clearTimeout(savedTimerRef.current);
          savedTimerRef.current = setTimeout(() => {
            setJustSaved(false);
            savedTimerRef.current = null;
          }, SAVED_ACKNOWLEDGEMENT_MS);
        },
        onError: (err) => {
          Alert.alert("Error", getErrorMessage(err, "Failed to save product. Please try again."));
        },
      },
    );
  }, [id, title, description, isActive, updateMutation]);

  const handleDeleteExistingMedia = useCallback(
    (mediaId: string) => {
      Alert.alert("Delete Image", "Remove this image from the product?", [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () => deleteMediaMutation.mutate({ productId: id, mediaId }),
        },
      ]);
    },
    [id, deleteMediaMutation],
  );

  const handleAddMedia = useCallback(async () => {
    try {
      const permission = await ImagePicker.requestMediaLibraryPermissionsAsync();
      if (!permission.granted) {
        Alert.alert(
          "Permission needed",
          "Allow photo library access in Settings to add product images.",
        );
        return;
      }

      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ["images"],
        quality: 0.8,
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
      // requestMediaLibraryPermissionsAsync/launchImageLibraryAsync can
      // themselves reject (e.g. platform picker errors). onPress doesn't
      // await this handler, so an uncaught rejection here would be a
      // silent failure — the exact class this project exists to kill.
      Alert.alert("Error", getErrorMessage(err, "Failed to open the image picker."));
    }
  }, [id, product?.media.length, addMediaMutation]);

  const handleVariantUpdate = useCallback(
    (variantId: string, body: UpdateVariantBody) => {
      updateVariantMutation.mutate(
        { productId: id, variantId, body },
        {
          onError: (err) => {
            Alert.alert("Error", getErrorMessage(err, "Failed to save variant. Please try again."));
          },
        },
      );
    },
    [id, updateVariantMutation],
  );

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="PRODUCT" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load product
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !product) {
    return (
      <Screen>
        <BackHeader eyebrow="PRODUCT" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const saveLabel = updateMutation.isPending ? "Saving…" : justSaved ? "Saved" : "Save";
  // The wire returns media and variants UNSORTED — a real product came back
  // as positions 2,3,4,0,1. Sort for display; never mutate the query cache.
  const media = [...product.media].sort((a, b) => a.position - b.position);
  const variants = [...product.variants].sort((a, b) => a.position - b.position);

  return (
    <Screen>
      <BackHeader
        eyebrow="PRODUCT"
        title={product.title}
        rightSlot={
          <TouchableOpacity
            onPress={handleSave}
            disabled={updateMutation.isPending}
            accessibilityRole="button"
            accessibilityLabel={saveLabel}
            hitSlop={8}
          >
            <Text preset="bodyEmphasis" color="accent">
              {saveLabel}
            </Text>
          </TouchableOpacity>
        }
      />

      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <Eyebrow
          label="Photos"
          rightSlot={
            <TouchableOpacity
              onPress={handleAddMedia}
              disabled={addMediaMutation.isPending}
              hitSlop={8}
              accessibilityRole="button"
              accessibilityLabel={addMediaMutation.isPending ? "Uploading image" : "Add image"}
            >
              <Text preset="caption" color="accent">
                {addMediaMutation.isPending ? "Uploading…" : "Add"}
              </Text>
            </TouchableOpacity>
          }
        />
        <Card padding="md" style={styles.card}>
          {media.length > 0 ? (
            <ScrollView
              horizontal
              showsHorizontalScrollIndicator={false}
              contentContainerStyle={styles.galleryRow}
            >
              {media.map((m) => (
                <TouchableOpacity
                  key={m.id}
                  onPress={() => setViewerImage({ uri: m.url, alt: m.alt })}
                  onLongPress={() => handleDeleteExistingMedia(m.id)}
                  activeOpacity={0.85}
                  accessibilityRole="button"
                  accessibilityLabel={
                    m.alt
                      ? `${m.alt}. Tap to view. Long press to delete.`
                      : "Product image. Tap to view. Long press to delete."
                  }
                >
                  <Image source={{ uri: m.url }} style={styles.mediaThumb} />
                </TouchableOpacity>
              ))}
            </ScrollView>
          ) : (
            <Text preset="caption" color="textTertiary">
              No images yet.
            </Text>
          )}
        </Card>

        <Eyebrow label="Details" />
        <Card padding="md" style={styles.card}>
          <FieldLabel label="Title" />
          <TextInput
            style={styles.input}
            value={title}
            onChangeText={setTitle}
            placeholder="Product title"
            placeholderTextColor={theme.colors.textTertiary}
          />
          <FieldLabel label="Description" />
          <TextInput
            style={[styles.input, styles.multilineInput]}
            value={description}
            onChangeText={setDescription}
            placeholder="Describe your product…"
            placeholderTextColor={theme.colors.textTertiary}
            multiline
            numberOfLines={4}
            textAlignVertical="top"
          />
          <Hairline style={styles.fieldDivider} />
          <View style={styles.switchRow}>
            <View>
              <Text preset="bodyEmphasis" color="text">
                Active
              </Text>
              <Text preset="caption" color="textTertiary">
                Visible in the storefront
              </Text>
            </View>
            <Switch
              value={isActive}
              onValueChange={setIsActive}
              trackColor={{
                false: theme.colors.border,
                true: theme.colors.accent,
              }}
              thumbColor={theme.colors.inverse}
            />
          </View>
        </Card>

        {/* Price, SKU and stock live on the VARIANT, not the product — the
            product-level fields this screen used to edit are not on the wire. */}
        <Eyebrow label="Variants" />
        <Card padding={0} style={styles.card}>
          {variants.length > 0 ? (
            variants.map((v, i) => (
              <View key={v.id}>
                {i > 0 ? <Hairline /> : null}
                <VariantRow variant={v} onUpdate={handleVariantUpdate} />
              </View>
            ))
          ) : (
            <View style={styles.empty}>
              <Text preset="caption" color="textTertiary">
                No variants yet.
              </Text>
            </View>
          )}
        </Card>
      </ScrollView>

      <Modal
        visible={viewerImage !== null}
        transparent
        animationType="fade"
        onRequestClose={() => setViewerImage(null)}
      >
        <ImageViewer image={viewerImage} onClose={() => setViewerImage(null)} />
      </Modal>
    </Screen>
  );
}

interface ImageViewerProps {
  image: { uri: string; alt?: string } | null;
  onClose: () => void;
}

/** Full-screen, dismissable viewer for a tapped product image. */
function ImageViewer({ image, onClose }: ImageViewerProps) {
  const insets = useSafeAreaInsets();

  if (!image) return null;

  return (
    <View style={styles.viewerBackdrop}>
      <TouchableOpacity
        style={[styles.viewerClose, { top: insets.top + theme.spacing.md }]}
        onPress={onClose}
        hitSlop={12}
        accessibilityRole="button"
        accessibilityLabel="Close image viewer"
      >
        <Text preset="bodyEmphasis" color="inverse">
          Close
        </Text>
      </TouchableOpacity>
      <Image
        source={{ uri: image.uri }}
        style={styles.viewerImage}
        resizeMode="contain"
        accessibilityLabel={image.alt ?? "Product image"}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  scroll: {
    paddingBottom: theme.spacing.huge,
  },
  card: {
    marginHorizontal: theme.spacing.lg,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  galleryRow: { gap: theme.spacing.sm },
  mediaThumb: {
    width: 96,
    height: 96,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.surfaceAlt,
  },
  viewerBackdrop: {
    flex: 1,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  viewerClose: {
    position: "absolute",
    right: theme.spacing.lg,
    zIndex: 1,
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
  },
  viewerImage: {
    width: "100%",
    height: "80%",
  },
  fieldLabel: {
    marginTop: theme.spacing.sm,
    marginBottom: theme.spacing.xs,
    letterSpacing: 0.4,
  },
  input: {
    backgroundColor: theme.colors.surfaceAlt,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
    fontFamily: theme.fonts.sans,
    fontSize: 15,
    color: theme.colors.text,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    minHeight: 44,
  },
  multilineInput: { minHeight: 88, paddingTop: theme.spacing.sm },
  fieldDivider: { marginVertical: theme.spacing.md },
  switchRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  variantRow: {
    paddingVertical: theme.spacing.md,
    paddingHorizontal: theme.spacing.md,
    gap: 4,
  },
  variantFields: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.sm,
  },
  variantField: { flex: 1 },
  variantInput: {
    backgroundColor: theme.colors.surfaceAlt,
    borderRadius: theme.radii.sm,
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.xs,
    fontFamily: theme.fonts.sans,
    fontSize: 14,
    color: theme.colors.text,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    minHeight: 36,
  },
  empty: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.lg,
  },
});
