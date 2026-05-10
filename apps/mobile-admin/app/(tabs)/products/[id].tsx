import { useState, useCallback } from "react";
import {
  View,
  ScrollView,
  TextInput,
  Image,
  Switch,
  TouchableOpacity,
  Modal,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { Plus, Trash2 } from "lucide-react-native";
import { useProduct } from "../../../lib/hooks/use-products";
import {
  useUpdateProduct,
  useUploadMedia,
  useDeleteMedia,
  useCreateVariant,
  useUpdateVariant,
} from "../../../lib/admin-api/product-crud";
import { ProductMediaPicker } from "../../../components/ProductMediaPicker";
import {
  BackHeader,
  Card,
  Eyebrow,
  Hairline,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Variant } from "@repo/mobile-shared/api/types";

function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textSecondary" style={styles.fieldLabel}>
      {label}
    </Text>
  );
}

interface VariantRowProps {
  variant: Variant;
  onUpdate: (variantId: string, body: Record<string, unknown>) => void;
}

function VariantRow({ variant, onUpdate }: VariantRowProps) {
  const [price, setPrice] = useState(String(variant.price));
  const [stock, setStock] = useState(String(variant.stock));

  const handleBlurPrice = () => {
    const parsed = parseFloat(price);
    if (!isNaN(parsed) && parsed !== variant.price) onUpdate(variant.id, { price: parsed });
  };
  const handleBlurStock = () => {
    const parsed = parseInt(stock, 10);
    if (!isNaN(parsed) && parsed !== variant.stock) onUpdate(variant.id, { stock: parsed });
  };

  return (
    <View style={styles.variantRow}>
      <Text preset="bodyEmphasis" color="text">
        {variant.name}
      </Text>
      {variant.sku ? (
        <Text preset="caption" color="textTertiary">
          SKU · {variant.sku}
        </Text>
      ) : null}
      <View style={styles.variantFields}>
        <View style={styles.variantField}>
          <FieldLabel label="Price" />
          <TextInput
            style={styles.variantInput}
            value={price}
            onChangeText={setPrice}
            onBlur={handleBlurPrice}
            keyboardType="decimal-pad"
            accessibilityLabel={`${variant.name} price`}
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
            accessibilityLabel={`${variant.name} stock`}
          />
        </View>
      </View>
    </View>
  );
}

export default function ProductDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: product, isLoading, error } = useProduct(id);

  const updateMutation = useUpdateProduct();
  const uploadMediaMutation = useUploadMedia();
  const deleteMediaMutation = useDeleteMedia();
  const createVariantMutation = useCreateVariant();
  const updateVariantMutation = useUpdateVariant();

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [compareAtPrice, setCompareAtPrice] = useState("");
  const [sku, setSku] = useState("");
  const [stock, setStock] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [newImages, setNewImages] = useState<string[]>([]);
  const [variantModalVisible, setVariantModalVisible] = useState(false);
  const [newVariantName, setNewVariantName] = useState("");
  const [newVariantPrice, setNewVariantPrice] = useState("");
  const [newVariantSku, setNewVariantSku] = useState("");
  const [newVariantStock, setNewVariantStock] = useState("");
  const [initialized, setInitialized] = useState(false);

  if (product && !initialized) {
    setName(product.name);
    setDescription(product.description ?? "");
    setPrice(String(product.price));
    setCompareAtPrice(product.compare_at_price ? String(product.compare_at_price) : "");
    setSku(product.sku ?? "");
    setStock(String(product.stock));
    setIsActive(product.status === "active");
    setInitialized(true);
  }

  const handleSave = useCallback(() => {
    const parsedPrice = parseFloat(price);
    const parsedCompare = compareAtPrice ? parseFloat(compareAtPrice) : undefined;
    const parsedStock = parseInt(stock, 10);

    if (!name.trim()) {
      Alert.alert("Validation", "Product name is required.");
      return;
    }
    if (isNaN(parsedPrice) || parsedPrice < 0) {
      Alert.alert("Validation", "Enter a valid price.");
      return;
    }
    updateMutation.mutate({
      id,
      body: {
        name: name.trim(),
        description: description.trim() || undefined,
        price: parsedPrice,
        compare_at_price: parsedCompare,
        sku: sku.trim() || undefined,
        stock: isNaN(parsedStock) ? 0 : parsedStock,
        status: isActive ? "active" : "inactive",
      },
    });
  }, [id, name, description, price, compareAtPrice, sku, stock, isActive, updateMutation]);

  const handleUploadNewImages = useCallback(
    async (uris: string[]) => {
      setNewImages(uris);
      const latestUri = uris[uris.length - 1];
      if (latestUri) uploadMediaMutation.mutate({ productId: id, uri: latestUri });
    },
    [id, uploadMediaMutation],
  );

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
    (variantId: string, body: Record<string, unknown>) => {
      updateVariantMutation.mutate({ productId: id, variantId, body });
    },
    [id, updateVariantMutation],
  );

  const handleAddVariant = useCallback(() => {
    const parsedPrice = parseFloat(newVariantPrice);
    if (!newVariantName.trim() || isNaN(parsedPrice)) {
      Alert.alert("Validation", "Variant name and price are required.");
      return;
    }
    const parsedStock = parseInt(newVariantStock, 10);
    createVariantMutation.mutate(
      {
        productId: id,
        body: {
          name: newVariantName.trim(),
          sku: newVariantSku.trim() || undefined,
          price: parsedPrice,
          stock: isNaN(parsedStock) ? 0 : parsedStock,
        },
      },
      {
        onSuccess: () => {
          setVariantModalVisible(false);
          setNewVariantName("");
          setNewVariantPrice("");
          setNewVariantSku("");
          setNewVariantStock("");
        },
      },
    );
  }, [id, newVariantName, newVariantPrice, newVariantSku, newVariantStock, createVariantMutation]);

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

  return (
    <Screen>
      <BackHeader
        eyebrow="PRODUCT"
        title={product.name}
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

      <ScrollView contentContainerStyle={styles.scroll}>
        <Eyebrow label="Photos" />
        <Card padding="md" style={styles.card}>
          {product.media.length > 0 ? (
            <ScrollView
              horizontal
              showsHorizontalScrollIndicator={false}
              contentContainerStyle={styles.galleryRow}
            >
              {product.media.map((m) => (
                <TouchableOpacity
                  key={m.id}
                  onLongPress={() => handleDeleteExistingMedia(m.id)}
                  activeOpacity={0.85}
                  accessibilityRole="button"
                  accessibilityLabel="Product image. Long press to delete."
                >
                  <Image source={{ uri: m.url }} style={styles.mediaThumb} />
                </TouchableOpacity>
              ))}
            </ScrollView>
          ) : (
            <Text preset="caption" color="textTertiary">
              No images yet — add some below.
            </Text>
          )}
          <View style={styles.pickerWrap}>
            <ProductMediaPicker images={newImages} onImagesChange={handleUploadNewImages} />
          </View>
        </Card>

        <Eyebrow label="Details" />
        <Card padding="md" style={styles.card}>
          <FieldLabel label="Name" />
          <TextInput
            style={styles.input}
            value={name}
            onChangeText={setName}
            placeholder="Product name"
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
          <View style={styles.row}>
            <View style={styles.half}>
              <FieldLabel label="Price" />
              <TextInput
                style={styles.input}
                value={price}
                onChangeText={setPrice}
                keyboardType="decimal-pad"
                placeholder="0.00"
                placeholderTextColor={theme.colors.textTertiary}
              />
            </View>
            <View style={styles.half}>
              <FieldLabel label="Compare at" />
              <TextInput
                style={styles.input}
                value={compareAtPrice}
                onChangeText={setCompareAtPrice}
                keyboardType="decimal-pad"
                placeholder="0.00"
                placeholderTextColor={theme.colors.textTertiary}
              />
            </View>
          </View>
          <View style={styles.row}>
            <View style={styles.half}>
              <FieldLabel label="SKU" />
              <TextInput
                style={styles.input}
                value={sku}
                onChangeText={setSku}
                autoCapitalize="characters"
                placeholder="SKU"
                placeholderTextColor={theme.colors.textTertiary}
              />
            </View>
            <View style={styles.half}>
              <FieldLabel label="Stock" />
              <TextInput
                style={styles.input}
                value={stock}
                onChangeText={setStock}
                keyboardType="number-pad"
                placeholder="0"
                placeholderTextColor={theme.colors.textTertiary}
              />
            </View>
          </View>
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

        <Eyebrow label="Variants" />
        <Card padding={0} style={styles.card}>
          {product.variants.length > 0 ? (
            product.variants.map((v, i) => (
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
          <Hairline />
          <TouchableOpacity
            style={styles.addVariantBtn}
            onPress={() => setVariantModalVisible(true)}
            activeOpacity={0.6}
            accessibilityRole="button"
            accessibilityLabel="Add variant"
          >
            <Plus size={16} color={theme.colors.accent} strokeWidth={2} />
            <Text preset="bodyEmphasis" color="accent">
              Add Variant
            </Text>
          </TouchableOpacity>
        </Card>
      </ScrollView>

      <Modal visible={variantModalVisible} transparent animationType="fade">
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <Text preset="h3" color="text">
              New Variant
            </Text>
            <TextInput
              style={styles.modalInput}
              placeholder="Variant name (e.g. Large, Red)"
              placeholderTextColor={theme.colors.textTertiary}
              value={newVariantName}
              onChangeText={setNewVariantName}
              autoFocus
            />
            <TextInput
              style={styles.modalInput}
              placeholder="Price"
              placeholderTextColor={theme.colors.textTertiary}
              value={newVariantPrice}
              onChangeText={setNewVariantPrice}
              keyboardType="decimal-pad"
            />
            <TextInput
              style={styles.modalInput}
              placeholder="SKU (optional)"
              placeholderTextColor={theme.colors.textTertiary}
              value={newVariantSku}
              onChangeText={setNewVariantSku}
              autoCapitalize="characters"
            />
            <TextInput
              style={styles.modalInput}
              placeholder="Stock (optional)"
              placeholderTextColor={theme.colors.textTertiary}
              value={newVariantStock}
              onChangeText={setNewVariantStock}
              keyboardType="number-pad"
            />
            <View style={styles.modalActions}>
              <TouchableOpacity
                style={styles.modalCancelBtn}
                onPress={() => {
                  setVariantModalVisible(false);
                  setNewVariantName("");
                  setNewVariantPrice("");
                  setNewVariantSku("");
                  setNewVariantStock("");
                }}
                accessibilityRole="button"
                accessibilityLabel="Cancel"
              >
                <Text preset="bodyEmphasis" color="textSecondary">
                  Cancel
                </Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={styles.modalSubmitBtn}
                onPress={handleAddVariant}
                disabled={createVariantMutation.isPending}
                accessibilityRole="button"
                accessibilityLabel={
                  createVariantMutation.isPending ? "Adding variant" : "Add variant"
                }
              >
                <Text preset="bodyEmphasis" color="inverse">
                  {createVariantMutation.isPending ? "Adding…" : "Add Variant"}
                </Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
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
  pickerWrap: { marginTop: theme.spacing.md },
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
  row: { flexDirection: "row", gap: theme.spacing.sm },
  half: { flex: 1 },
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
  addVariantBtn: {
    flexDirection: "row",
    gap: theme.spacing.xs,
    alignItems: "center",
    justifyContent: "center",
    paddingVertical: theme.spacing.md,
    minHeight: 48,
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: theme.colors.overlay,
    justifyContent: "center",
    alignItems: "center",
    padding: theme.spacing.xl,
  },
  modalContent: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.lg,
    padding: theme.spacing.xl,
    width: "100%",
    maxWidth: 360,
    gap: theme.spacing.sm,
  },
  modalInput: {
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
  modalActions: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.sm,
  },
  modalCancelBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.sm,
    minHeight: 44,
    justifyContent: "center",
  },
  modalSubmitBtn: {
    backgroundColor: theme.colors.accent,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.sm,
    borderRadius: theme.radii.md,
    minHeight: 44,
    justifyContent: "center",
  },
});
