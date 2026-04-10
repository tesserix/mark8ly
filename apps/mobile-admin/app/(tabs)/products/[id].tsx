import { useState, useCallback, useLayoutEffect } from "react";
import {
  View,
  Text,
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
import { useLocalSearchParams, useNavigation } from "expo-router";
import { useProduct } from "../../../lib/hooks/use-products";
import {
  useUpdateProduct,
  useUploadMedia,
  useDeleteMedia,
  useCreateVariant,
  useUpdateVariant,
} from "../../../lib/admin-api/product-crud";
import { ProductMediaPicker } from "../../../components/ProductMediaPicker";
import { theme } from "@/lib/theme";
import type { Variant } from "@repo/mobile-shared/api/types";

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

function SectionHeader({ title }: { title: string }) {
  return <Text style={styles.sectionHeader}>{title}</Text>;
}

function FieldLabel({ label }: { label: string }) {
  return <Text style={styles.fieldLabel}>{label}</Text>;
}

interface VariantRowProps {
  variant: Variant;
  onUpdate: (variantId: string, body: Record<string, unknown>) => void;
}

function VariantRow({ variant, onUpdate }: VariantRowProps) {
  const [price, setPrice] = useState(String(variant.price));
  const [stock, setStock] = useState(String(variant.stock));

  const handleBlurPrice = useCallback(() => {
    const parsed = parseFloat(price);
    if (!isNaN(parsed) && parsed !== variant.price) {
      onUpdate(variant.id, { price: parsed });
    }
  }, [price, variant.id, variant.price, onUpdate]);

  const handleBlurStock = useCallback(() => {
    const parsed = parseInt(stock, 10);
    if (!isNaN(parsed) && parsed !== variant.stock) {
      onUpdate(variant.id, { stock: parsed });
    }
  }, [stock, variant.id, variant.stock, onUpdate]);

  return (
    <View style={styles.variantRow}>
      <Text style={styles.variantName}>{variant.name}</Text>
      {variant.sku ? <Text style={styles.variantSku}>SKU: {variant.sku}</Text> : null}
      <View style={styles.variantFields}>
        <View style={styles.variantField}>
          <Text style={styles.variantFieldLabel}>Price</Text>
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
          <Text style={styles.variantFieldLabel}>Stock</Text>
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
  const navigation = useNavigation();
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

  // Populate fields once product loads
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

  useLayoutEffect(() => {
    navigation.setOptions({
      headerRight: () => (
        <TouchableOpacity
          onPress={handleSave}
          disabled={updateMutation.isPending}
          style={styles.headerSaveBtn}
          accessibilityRole="button"
          accessibilityLabel={updateMutation.isPending ? "Saving product" : "Save product"}
        >
          <Text style={styles.headerSaveText}>
            {updateMutation.isPending ? "Saving..." : "Save"}
          </Text>
        </TouchableOpacity>
      ),
    });
  }, [navigation, handleSave, updateMutation.isPending]);

  const handleUploadNewImages = useCallback(
    async (uris: string[]) => {
      setNewImages(uris);
      // Upload any newly added images
      const latestUri = uris[uris.length - 1];
      if (latestUri) {
        uploadMediaMutation.mutate({ productId: id, uri: latestUri });
      }
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
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color={theme.colors.text} />
      </View>
    );
  }

  if (error) {
    return (
      <View style={styles.loadingContainer}>
        <Text style={styles.errorText}>Failed to load product</Text>
      </View>
    );
  }

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      {/* Existing media gallery */}
      <View style={styles.card}>
        <SectionHeader title="Photos" />
        {product.media.length > 0 ? (
          <ScrollView horizontal showsHorizontalScrollIndicator={false}>
            {product.media.map((m) => (
              <TouchableOpacity
                key={m.id}
                onLongPress={() => handleDeleteExistingMedia(m.id)}
                activeOpacity={0.8}
                accessibilityRole="button"
                accessibilityLabel={`Product image. Long press to delete`}
                accessibilityHint="Long press to delete"
              >
                <Image
                  source={{ uri: m.url }}
                  style={styles.mediaThumb}
                  accessibilityLabel="Product image"
                />
              </TouchableOpacity>
            ))}
          </ScrollView>
        ) : (
          <Text style={styles.emptyText}>No images yet</Text>
        )}
        <View style={styles.pickerWrap}>
          <ProductMediaPicker images={newImages} onImagesChange={handleUploadNewImages} />
        </View>
      </View>

      {/* Editable fields */}
      <View style={styles.card}>
        <SectionHeader title="Details" />
        <FieldLabel label="Name" />
        <TextInput
          style={styles.input}
          value={name}
          onChangeText={setName}
          placeholder="Product name"
          placeholderTextColor={`${theme.colors.text}50`}
          accessibilityLabel="Product name"
        />
        <FieldLabel label="Description" />
        <TextInput
          style={[styles.input, styles.multilineInput]}
          value={description}
          onChangeText={setDescription}
          placeholder="Product description"
          placeholderTextColor={`${theme.colors.text}50`}
          multiline
          numberOfLines={4}
          textAlignVertical="top"
          accessibilityLabel="Product description"
        />
        <View style={styles.row}>
          <View style={styles.halfField}>
            <FieldLabel label="Price" />
            <TextInput
              style={styles.input}
              value={price}
              onChangeText={setPrice}
              keyboardType="decimal-pad"
              placeholder="0.00"
              placeholderTextColor={`${theme.colors.text}50`}
              accessibilityLabel="Product price"
            />
          </View>
          <View style={styles.halfField}>
            <FieldLabel label="Compare at Price" />
            <TextInput
              style={styles.input}
              value={compareAtPrice}
              onChangeText={setCompareAtPrice}
              keyboardType="decimal-pad"
              placeholder="0.00"
              placeholderTextColor={`${theme.colors.text}50`}
              accessibilityLabel="Compare at price"
            />
          </View>
        </View>
        <View style={styles.row}>
          <View style={styles.halfField}>
            <FieldLabel label="SKU" />
            <TextInput
              style={styles.input}
              value={sku}
              onChangeText={setSku}
              placeholder="SKU"
              placeholderTextColor={`${theme.colors.text}50`}
              autoCapitalize="characters"
              accessibilityLabel="SKU"
            />
          </View>
          <View style={styles.halfField}>
            <FieldLabel label="Stock" />
            <TextInput
              style={styles.input}
              value={stock}
              onChangeText={setStock}
              keyboardType="number-pad"
              placeholder="0"
              placeholderTextColor={`${theme.colors.text}50`}
              accessibilityLabel="Stock quantity"
            />
          </View>
        </View>
        <View style={styles.switchRow}>
          <Text style={styles.switchLabel}>Active</Text>
          <Switch
            value={isActive}
            onValueChange={setIsActive}
            trackColor={{ false: `${theme.colors.text}20`, true: theme.colors.accent }}
            thumbColor={theme.colors.elevated}
            accessibilityLabel="Product active status"
          />
        </View>
      </View>

      {/* Variants */}
      <View style={styles.card}>
        <SectionHeader title="Variants" />
        {product.variants.length > 0 ? (
          product.variants.map((v) => (
            <VariantRow key={v.id} variant={v} onUpdate={handleVariantUpdate} />
          ))
        ) : (
          <Text style={styles.emptyText}>No variants</Text>
        )}
        <TouchableOpacity
          style={styles.addVariantBtn}
          onPress={() => setVariantModalVisible(true)}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel="Add variant"
        >
          <Text style={styles.addVariantText}>+ Add Variant</Text>
        </TouchableOpacity>
      </View>

      {/* Add variant modal */}
      <Modal visible={variantModalVisible} transparent animationType="fade">
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <Text style={styles.modalTitle}>New Variant</Text>
            <TextInput
              style={styles.modalInput}
              placeholder="Variant name (e.g. Large, Red)"
              placeholderTextColor={`${theme.colors.text}50`}
              value={newVariantName}
              onChangeText={setNewVariantName}
              autoFocus
              accessibilityLabel="Variant name"
            />
            <TextInput
              style={styles.modalInput}
              placeholder="Price"
              placeholderTextColor={`${theme.colors.text}50`}
              value={newVariantPrice}
              onChangeText={setNewVariantPrice}
              keyboardType="decimal-pad"
              accessibilityLabel="Variant price"
            />
            <TextInput
              style={styles.modalInput}
              placeholder="SKU (optional)"
              placeholderTextColor={`${theme.colors.text}50`}
              value={newVariantSku}
              onChangeText={setNewVariantSku}
              autoCapitalize="characters"
              accessibilityLabel="Variant SKU"
            />
            <TextInput
              style={styles.modalInput}
              placeholder="Stock (optional)"
              placeholderTextColor={`${theme.colors.text}50`}
              value={newVariantStock}
              onChangeText={setNewVariantStock}
              keyboardType="number-pad"
              accessibilityLabel="Variant stock"
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
                accessibilityLabel="Cancel adding variant"
              >
                <Text style={styles.modalCancelText}>Cancel</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={styles.modalSubmitBtn}
                onPress={handleAddVariant}
                disabled={createVariantMutation.isPending}
                accessibilityRole="button"
                accessibilityLabel={createVariantMutation.isPending ? "Adding variant" : "Add variant"}
              >
                <Text style={styles.modalSubmitText}>
                  {createVariantMutation.isPending ? "Adding..." : "Add Variant"}
                </Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  content: {
    padding: theme.spacing.lg,
    paddingBottom: 40,
    gap: theme.spacing.md,
  },
  loadingContainer: {
    flex: 1,
    backgroundColor: theme.colors.background,
    justifyContent: "center",
    alignItems: "center",
  },
  errorText: {
    fontSize: 15,
    color: theme.colors.danger,
    fontWeight: "500",
  },
  card: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: 14,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  sectionHeader: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 10,
  },
  mediaThumb: {
    width: 100,
    height: 100,
    borderRadius: theme.radius,
    marginRight: theme.spacing.sm,
  },
  pickerWrap: {
    marginTop: theme.spacing.md,
  },
  emptyText: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.4,
    fontStyle: "italic",
    marginBottom: theme.spacing.sm,
  },
  fieldLabel: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.6,
    marginBottom: theme.spacing.xs,
    marginTop: 10,
  },
  input: {
    backgroundColor: theme.colors.background,
    borderRadius: theme.radius,
    paddingHorizontal: 14,
    paddingVertical: 10,
    fontSize: 14,
    color: theme.colors.text,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  multilineInput: {
    minHeight: 80,
  },
  row: {
    flexDirection: "row",
    gap: 10,
  },
  halfField: {
    flex: 1,
  },
  switchRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginTop: 14,
  },
  switchLabel: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
  },
  variantRow: {
    paddingVertical: 10,
    borderBottomWidth: 0.5,
    borderBottomColor: `${theme.colors.text}10`,
  },
  variantName: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: 2,
  },
  variantSku: {
    fontSize: 11,
    color: theme.colors.text,
    opacity: 0.5,
    marginBottom: theme.spacing.sm,
  },
  variantFields: {
    flexDirection: "row",
    gap: 10,
  },
  variantField: {
    flex: 1,
  },
  variantFieldLabel: {
    fontSize: 11,
    color: theme.colors.text,
    opacity: 0.5,
    marginBottom: 2,
  },
  variantInput: {
    backgroundColor: theme.colors.background,
    borderRadius: 4,
    paddingHorizontal: 10,
    paddingVertical: 6,
    fontSize: 13,
    color: theme.colors.text,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  addVariantBtn: {
    marginTop: 10,
    paddingVertical: 10,
    alignItems: "center",
    borderRadius: theme.radius,
    borderWidth: 1,
    borderColor: `${theme.colors.text}20`,
    borderStyle: "dashed",
  },
  addVariantText: {
    fontSize: 13,
    fontWeight: "600",
    color: theme.colors.accent,
  },
  headerSaveBtn: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
  },
  headerSaveText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.accent,
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.4)",
    justifyContent: "center",
    alignItems: "center",
    padding: theme.spacing.xxl,
  },
  modalContent: {
    backgroundColor: theme.colors.elevated,
    borderRadius: 10,
    padding: theme.spacing.xl,
    width: "100%",
    maxWidth: 340,
  },
  modalTitle: {
    fontSize: 16,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: 14,
  },
  modalInput: {
    backgroundColor: theme.colors.background,
    borderRadius: theme.radius,
    paddingHorizontal: 14,
    paddingVertical: 10,
    fontSize: 14,
    color: theme.colors.text,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
    marginBottom: 10,
  },
  modalActions: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: 10,
    marginTop: theme.spacing.sm,
  },
  modalCancelBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: 10,
    borderRadius: theme.radius,
  },
  modalCancelText: {
    fontSize: 14,
    fontWeight: "500",
    color: theme.colors.text,
    opacity: 0.6,
  },
  modalSubmitBtn: {
    backgroundColor: theme.colors.accent,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: 10,
    borderRadius: theme.radius,
  },
  modalSubmitText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.elevated,
  },
});
