import { useState, useCallback, useEffect, useRef } from "react";
import {
  Platform,
  View,
  Image,
  ScrollView,
  Switch,
  Pressable,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { Package } from "lucide-react-native";
import { useLocalSearchParams } from "expo-router";
import { useProduct } from "../../../lib/hooks/use-products";
import {
  useUpdateProduct,
  useDeleteMedia,
  useUpdateVariant,
  useAddProductMedia,
  useCategories,
  useUpdateMedia,
  useCreateCategory,
} from "../../../lib/admin-api/product-crud";
import {
  BackHeader,
  Card,
  Eyebrow,
  FieldInput,
  Hairline,
  Screen,
  StatusBadge,
  Text,
  type StatusTone,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type {
  UpdateVariantBody,
  UpdateProductOptionBody,
} from "@repo/mobile-shared/api/products";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { VariantRow } from "@/components/products/VariantRow";
import { ImageViewer } from "@/components/products/ImageViewer";
import { OptionsEditor } from "@/components/products/OptionsEditor";
import { CategoryField } from "@/components/products/CategoryField";
import { MediaGrid } from "@/components/products/MediaGrid";
import { CreateNextStepsBanner } from "@/components/products/CreateNextStepsBanner";
import { useAddOptionHandler } from "@/lib/hooks/use-add-option-handler";
import { useProductMediaHandlers } from "@/lib/hooks/use-product-media-handlers";
import { useCreatedBanner } from "@/lib/hooks/use-created-banner";
import { getErrorMessage, alertOnError } from "@/lib/product-alerts";
import { productThumb } from "@/lib/product-display";

/** How long the transient "Saved" acknowledgement stays visible. */
const SAVED_ACKNOWLEDGEMENT_MS = 2000;

export default function ProductDetailScreen() {
  const dockPad = useDockClearance();
  const { id, created } = useLocalSearchParams<{ id: string; created?: string }>();
  const { data: product, isLoading, error } = useProduct(id);

  const updateMutation = useUpdateProduct();
  const deleteMediaMutation = useDeleteMedia();
  const updateVariantMutation = useUpdateVariant();
  const addMediaMutation = useAddProductMedia();
  const updateMediaMutation = useUpdateMedia();
  const createCategory = useCreateCategory();
  const { data: categories, isLoading: categoriesLoading, error: categoriesError, refetch: refetchCategories } =
    useCategories();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [initialized, setInitialized] = useState(false);
  const [justSaved, setJustSaved] = useState(false);
  const [viewerImage, setViewerImage] = useState<{ uri: string; alt?: string } | null>(null);
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [savePressed, setSavePressed] = useState(false);
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
    // `updateMutation` is a new object every render; `.mutate` is the
    // stable part of it, and the correct dependency.
  }, [id, title, description, isActive, updateMutation.mutate]);

  const { handleAddMedia, handleDeleteExistingMedia, handleReorderMedia, handleAltChange } =
    useProductMediaHandlers({ id, product, addMediaMutation, deleteMediaMutation, updateMediaMutation });

  const handleVariantUpdate = useCallback(
    (variantId: string, body: UpdateVariantBody) => {
      updateVariantMutation.mutate(
        { productId: id, variantId, body },
        alertOnError("Failed to save variant. Please try again."),
      );
    },
    [id, updateVariantMutation.mutate],
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
    [id, updateMutation.mutate],
  );

  const handleAddOption = useAddOptionHandler(id, product, updateMutation);
  const handleCategoriesChange = useCallback(
    (category_ids: string[]) => {
      updateMutation.mutate(
        { id, body: { category_ids } },
        alertOnError("Failed to update categories. Please try again."),
      );
    },
    [id, updateMutation.mutate],
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
  const thumb = productThumb(product);
  // The header badge reflects the SAVED product.status, not the `isActive`
  // draft state below (that's the live Switch toggle, only committed on Save).
  const productIsActive = product.status === "active";
  // 🔴 One accent, spent once: moss is reserved for the header Save action.
  // "active" reads as a solid ink pill, never the `success` tone — that
  // tone's label IS moss, and spending it here would be the exact violation this
  // rhythm pass exists to fix (see StatusBadge's TONE map).
  const statusTone: StatusTone = productIsActive ? "neutral" : "muted";

  return (
    <Screen>
      <BackHeader
        eyebrow="PRODUCT"
        title={product.title}
        rightSlot={
          <Pressable
            onPress={handleSave}
            onPressIn={() => setSavePressed(true)}
            onPressOut={() => setSavePressed(false)}
            disabled={updateMutation.isPending}
            accessibilityRole="button"
            accessibilityLabel={saveLabel}
            hitSlop={8}
            android_ripple={{ color: theme.press.rippleAccent.color, borderless: true }}
            style={[
              savePressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
            ]}
          >
            <Text preset="bodyEmphasis" color="accent" numberOfLines={1}>
              {saveLabel}
            </Text>
          </Pressable>
        }
      />

      <ScrollView
        ref={scrollViewRef}
        contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}
      >
        <View style={styles.header}>
          {thumb ? (
            <Image
              source={{ uri: thumb }}
              style={styles.headerThumb}
              accessibilityLabel={`${product.title} thumbnail`}
            />
          ) : (
            <View
              style={[styles.headerThumb, styles.headerThumbPlaceholder]}
              accessible
              accessibilityLabel="No product image"
            >
              <Package size={24} color={theme.colors.textTertiary} strokeWidth={1.5} />
            </View>
          )}
          <View style={styles.headerInfo}>
            <Text preset="h2" numberOfLines={2}>
              {product.title}
            </Text>
            <StatusBadge label={productIsActive ? "Active" : product.status} tone={statusTone} />
          </View>
        </View>
        <Hairline />

        {showCreatedBanner ? (
          <CreateNextStepsBanner title={product.title} onJump={jumpTo} onDismiss={dismissCreatedBanner} />
        ) : null}

        {/* Movement 1 — Presentation: how the product looks (photos, copy). */}
        <View onLayout={(e) => registerSectionOffset("photos", e.nativeEvent.layout.y)}>
          <Eyebrow
            label="Photos"
            style={styles.eyebrowGutter}
            rightSlot={
              <Pressable
                onPress={handleAddMedia}
                disabled={addMediaMutation.isPending}
                hitSlop={8}
                accessibilityRole="button"
                accessibilityLabel={addMediaMutation.isPending ? "Uploading image" : "Add image"}
              >
                {({ pressed }) => (
                  <Text preset="caption" color={pressed ? "accent" : "text"}>
                    {addMediaMutation.isPending ? "Uploading…" : "Add"}
                  </Text>
                )}
              </Pressable>
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

        <View>
          <Eyebrow label="Details" style={styles.eyebrowGutter} />
          <Card variant="ghost" padding="md" style={styles.card}>
            <Hairline style={styles.cardTopHairline} />
            <FieldInput
              label="Title"
              value={title}
              onChangeText={setTitle}
              placeholder="Product title"
            />
            <FieldInput
              label="Description"
              value={description}
              onChangeText={setDescription}
              placeholder="Describe your product…"
              multiline
              numberOfLines={4}
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
                  // Ink, not moss — moss on this screen is reserved for the
                  // header Save action; a second moss element at rest here
                  // (next to Save) is the one-accent violation this pass fixes.
                  true: theme.colors.text,
                }}
                thumbColor={theme.colors.inverse}
              />
            </View>
          </Card>
        </View>

        {/* Movement 2 — Commerce: how the product sells (options, categories, variants). */}
        <View onLayout={(e) => registerSectionOffset("options", e.nativeEvent.layout.y)}>
          <Eyebrow label="Options" style={styles.movementBreak} />
          <Card variant="ghost" padding="md" style={styles.card}>
            <Hairline style={styles.cardTopHairline} />
            <OptionsEditor options={product.options} onChange={handleOptionsChange} onAddOption={handleAddOption} />
          </Card>
        </View>

        <View>
          <Eyebrow label="Categories" style={styles.eyebrowGutter} />
          <Card variant="ghost" padding="md" style={styles.card}>
            <Hairline style={styles.cardTopHairline} />
            <CategoryField
              categories={categories ?? []}
              selected={product.categories}
              onChange={handleCategoriesChange}
              isLoading={categoriesLoading}
              error={categoriesError}
              onRetry={refetchCategories}
              onCreateCategory={(name) => createCategory.mutateAsync({ name }).then((res) => res.data)}
              isCreatingCategory={createCategory.isPending}
            />
          </Card>
        </View>

        {/* Price, SKU and stock live on the VARIANT, not the product — the
            product-level fields this screen used to edit are not on the wire. */}
        <View onLayout={(e) => registerSectionOffset("variants", e.nativeEvent.layout.y)}>
          <Eyebrow label="Variants" style={styles.eyebrowGutter} />
          <Card padding={0} style={styles.card}>
            {variants.length > 0 ? (
              variants.map((v, i) => (
                <View key={v.id}>
                  {i > 0 ? <Hairline /> : null}
                  <VariantRow
                    variant={v}
                    onUpdate={handleVariantUpdate}
                    defaultOpen={variants.length === 1}
                  />
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
    marginHorizontal: theme.spacing.xl,
  },
  // Eyebrow's own default gutter (lg/16) is 4pt short of this screen's card
  // inset above — set explicitly per screen, never by changing Eyebrow's
  // default (see components/ui/Eyebrow.tsx). One left edge for every section.
  eyebrowGutter: { paddingHorizontal: theme.spacing.xl },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  // Editorial header band — surfaceAlt, not paper, so it reads as a distinct
  // block above the two movements; the bottom Hairline closes it out.
  header: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.lg,
    backgroundColor: theme.colors.surfaceAlt,
  },
  headerThumb: {
    width: 72,
    height: 72,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.border,
  },
  headerThumbPlaceholder: {
    alignItems: "center",
    justifyContent: "center",
  },
  headerInfo: {
    flex: 1,
    gap: theme.spacing.xs,
  },
  // Two movements — Presentation (Photos, Details) and Commerce (Options,
  // Categories, Variants). Within a movement, sections keep Eyebrow's own
  // `lg` rhythm; `movementBreak` overrides that to `xxl` on the first
  // Eyebrow of the next movement, so the boundary reads with more air.
  movementBreak: {
    paddingTop: theme.spacing.xxl,
    paddingHorizontal: theme.spacing.xl,
  },
  // Ghost cards (Details/Options/Categories) carry a top Hairline instead of
  // a border — this is its top margin, standing in for the removed border.
  cardTopHairline: {
    marginBottom: theme.spacing.md,
  },
  fieldDivider: { marginVertical: theme.spacing.md },
  switchRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    minHeight: theme.row.minHeightSingle,
    paddingVertical: theme.row.paddingV,
  },
  empty: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.lg,
  },
});
