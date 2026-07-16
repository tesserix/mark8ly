import { useState, useCallback, useEffect, useRef } from "react";
import {
  View,
  ScrollView,
  TextInput,
  Switch,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useProduct } from "../../../lib/hooks/use-products";
import {
  useUpdateProduct,
  useDeleteMedia,
  useUpdateVariant,
  useAddProductMedia,
  useCategories,
  useUpdateMedia,
} from "../../../lib/admin-api/product-crud";
import { BackHeader, Card, Eyebrow, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type {
  UpdateVariantBody,
  UpdateProductOptionBody,
} from "@repo/mobile-shared/api/products";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { VariantEditor } from "@/components/products/VariantEditor";
import { ImageViewer } from "@/components/products/ImageViewer";
import { OptionsEditor } from "@/components/products/OptionsEditor";
import { CategoryField } from "@/components/products/CategoryField";
import { MediaGrid } from "@/components/products/MediaGrid";
import { CreateNextStepsBanner } from "@/components/products/CreateNextStepsBanner";
import { useAddOptionHandler } from "@/lib/hooks/use-add-option-handler";
import { useProductMediaHandlers } from "@/lib/hooks/use-product-media-handlers";
import { useCreatedBanner } from "@/lib/hooks/use-created-banner";
import { getErrorMessage, alertOnError } from "@/lib/product-alerts";

/** How long the transient "Saved" acknowledgement stays visible. */
const SAVED_ACKNOWLEDGEMENT_MS = 2000;

function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textSecondary" style={styles.fieldLabel}>
      {label}
    </Text>
  );
}

export default function ProductDetailScreen() {
  const dockPad = useDockClearance();
  const { id, created } = useLocalSearchParams<{ id: string; created?: string }>();
  const { data: product, isLoading, error } = useProduct(id);

  const updateMutation = useUpdateProduct();
  const deleteMediaMutation = useDeleteMedia();
  const updateVariantMutation = useUpdateVariant();
  const addMediaMutation = useAddProductMedia();
  const updateMediaMutation = useUpdateMedia();
  const { data: categories, isLoading: categoriesLoading, error: categoriesError, refetch: refetchCategories } =
    useCategories();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [initialized, setInitialized] = useState(false);
  const [justSaved, setJustSaved] = useState(false);
  const [viewerImage, setViewerImage] = useState<{ uri: string; alt?: string } | null>(null);
  const savedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const scrollViewRef = useRef<ScrollView>(null);
  const { show: showCreatedBanner, dismiss: dismissCreatedBanner, registerSectionOffset, jumpTo } =
    useCreatedBanner(created, scrollViewRef);

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
          // Drop any in-flight "Saved" from a previous success. Without this a
          // save that fails within SAVED_ACKNOWLEDGEMENT_MS of a successful one
          // leaves the button reading "Saved" while an error alert is on screen
          // — an acknowledgement that lies, which is the whole thing this
          // acknowledgement exists to prevent.
          if (savedTimerRef.current) {
            clearTimeout(savedTimerRef.current);
            savedTimerRef.current = null;
          }
          setJustSaved(false);
          Alert.alert("Error", getErrorMessage(err, "Failed to save product. Please try again."));
        },
      },
    );
  }, [id, title, description, isActive, updateMutation]);

  const { handleAddMedia, handleDeleteExistingMedia, handleReorderMedia, handleAltChange } =
    useProductMediaHandlers({ id, product, addMediaMutation, deleteMediaMutation, updateMediaMutation });

  const handleVariantUpdate = useCallback(
    (variantId: string, body: UpdateVariantBody) => {
      updateVariantMutation.mutate(
        { productId: id, variantId, body },
        alertOnError("Failed to save variant. Please try again."),
      );
    },
    [id, updateVariantMutation],
  );

  // Options and categories both route through UpdateAggregate (products.go:172).
  // Send ONLY the changed section — never bundle `variants` in, because
  // UpdateAggregateRequest.Variants is a full desired matrix that soft-deletes
  // anything it omits.
  const handleOptionsChange = useCallback(
    (options: UpdateProductOptionBody[]) => {
      updateMutation.mutate(
        { id, body: { options } },
        alertOnError("Failed to update options. Please try again."),
      );
    },
    [id, updateMutation],
  );

  const handleAddOption = useAddOptionHandler(id, product, updateMutation);
  const handleCategoriesChange = useCallback(
    (category_ids: string[]) => {
      updateMutation.mutate(
        { id, body: { category_ids } },
        alertOnError("Failed to update categories. Please try again."),
      );
    },
    [id, updateMutation],
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
            <Text preset="bodyEmphasis" color="accent" numberOfLines={1}>
              {saveLabel}
            </Text>
          </TouchableOpacity>
        }
      />

      <ScrollView
        ref={scrollViewRef}
        contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}
      >
        {showCreatedBanner ? (
          <CreateNextStepsBanner title={product.title} onJump={jumpTo} onDismiss={dismissCreatedBanner} />
        ) : null}

        <View onLayout={(e) => registerSectionOffset("photos", e.nativeEvent.layout.y)}>
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
              <MediaGrid
                media={media}
                onReorder={handleReorderMedia}
                onAltChange={handleAltChange}
                onPress={(m) => setViewerImage({ uri: m.url, alt: m.alt })}
                onLongPress={handleDeleteExistingMedia}
              />
            ) : (
              <Text preset="caption" color="textTertiary">
                No images yet.
              </Text>
            )}
          </Card>
        </View>

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

        <View onLayout={(e) => registerSectionOffset("options", e.nativeEvent.layout.y)}>
          <Eyebrow label="Options" />
          <Card padding="md" style={styles.card}>
            <OptionsEditor options={product.options} onChange={handleOptionsChange} onAddOption={handleAddOption} />
          </Card>
        </View>

        <Eyebrow label="Categories" />
        <Card padding="md" style={styles.card}>
          <CategoryField
            categories={categories ?? []}
            selected={product.categories}
            onChange={handleCategoriesChange}
            isLoading={categoriesLoading}
            error={categoriesError}
            onRetry={refetchCategories}
          />
        </Card>

        {/* Price, SKU and stock live on the VARIANT, not the product — the
            product-level fields this screen used to edit are not on the wire. */}
        <View onLayout={(e) => registerSectionOffset("variants", e.nativeEvent.layout.y)}>
          <Eyebrow label="Variants" />
          <Card padding={0} style={styles.card}>
            {variants.length > 0 ? (
              variants.map((v, i) => (
                <View key={v.id}>
                  {i > 0 ? <Hairline /> : null}
                  <VariantEditor variant={v} onUpdate={handleVariantUpdate} />
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
        </View>
      </ScrollView>

      <ImageViewer image={viewerImage} onClose={() => setViewerImage(null)} />
    </Screen>
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
  empty: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.lg,
  },
});
