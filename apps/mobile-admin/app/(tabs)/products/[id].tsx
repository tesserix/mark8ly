import { useState, useCallback } from "react";
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
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useProduct } from "../../../lib/hooks/use-products";
import {
  useUpdateProduct,
  useDeleteMedia,
  useUpdateVariant,
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
import { useDockClearance } from "@/components/navigation/dock-metrics";

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

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [initialized, setInitialized] = useState(false);

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
    updateMutation.mutate({
      id,
      body: {
        title: title.trim(),
        description: description.trim() || undefined,
        // The enum is draft|active|archived — "inactive" is a hard 400.
        status: isActive ? "active" : "draft",
      },
    });
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

  const handleVariantUpdate = useCallback(
    (variantId: string, body: UpdateVariantBody) => {
      updateVariantMutation.mutate({ productId: id, variantId, body });
    },
    [id, updateVariantMutation],
  );

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

  const saveLabel = updateMutation.isPending ? "Saving…" : "Save";
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
        <Eyebrow label="Photos" />
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
                  onLongPress={() => handleDeleteExistingMedia(m.id)}
                  activeOpacity={0.85}
                  accessibilityRole="button"
                  accessibilityLabel={
                    m.alt
                      ? `${m.alt}. Long press to delete.`
                      : "Product image. Long press to delete."
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
  galleryRow: { gap: theme.spacing.sm },
  mediaThumb: {
    width: 96,
    height: 96,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.surfaceAlt,
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
