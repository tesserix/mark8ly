import { useState, useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  TextInput,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { ProductMediaPicker } from "../../../components/ProductMediaPicker";
import { useCreateProduct, useUploadMedia } from "../../../lib/admin-api/product-crud";
import { theme } from "@/lib/theme";

const TOTAL_STEPS = 4;

type ProductStatus = "draft" | "active";

const STATUS_OPTIONS: { key: ProductStatus; label: string }[] = [
  { key: "draft", label: "Draft" },
  { key: "active", label: "Active" },
];

function StepIndicator({ current, total }: { current: number; total: number }) {
  const progress = current / total;
  return (
    <View style={stepStyles.container}>
      <Text style={stepStyles.label}>
        {current} of {total}
      </Text>
      <View style={stepStyles.track}>
        <View style={[stepStyles.fill, { width: `${progress * 100}%` }]} />
      </View>
    </View>
  );
}

const stepStyles = StyleSheet.create({
  container: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    paddingBottom: theme.spacing.lg,
  },
  label: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.5,
    marginBottom: theme.spacing.sm,
  },
  track: {
    height: 4,
    borderRadius: 2,
    backgroundColor: `${theme.colors.text}15`,
    overflow: "hidden",
  },
  fill: {
    height: 4,
    borderRadius: 2,
    backgroundColor: theme.colors.accent,
  },
});

function FieldLabel({ label }: { label: string }) {
  return <Text style={styles.fieldLabel}>{label}</Text>;
}

export default function NewProductScreen() {
  const router = useRouter();
  const createMutation = useCreateProduct();
  const uploadMediaMutation = useUploadMedia();

  const [step, setStep] = useState(1);

  // Step 1: Photos
  const [images, setImages] = useState<string[]>([]);

  // Step 2: Details
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [compareAtPrice, setCompareAtPrice] = useState("");
  const [sku, setSku] = useState("");
  const [stock, setStock] = useState("");

  // Step 3: Organization
  const [categoryId, setCategoryId] = useState("");
  const [tags, setTags] = useState("");
  const [status, setStatus] = useState<ProductStatus>("draft");

  const [isSubmitting, setIsSubmitting] = useState(false);

  const canProceed = useCallback((): boolean => {
    if (step === 2) {
      return name.trim().length > 0 && !isNaN(parseFloat(price)) && parseFloat(price) >= 0;
    }
    return true;
  }, [step, name, price]);

  const handleNext = useCallback(() => {
    if (!canProceed()) {
      Alert.alert("Validation", "Please fill in all required fields (name and price).");
      return;
    }
    if (step < TOTAL_STEPS) {
      setStep(step + 1);
    }
  }, [step, canProceed]);

  const handleBack = useCallback(() => {
    if (step > 1) {
      setStep(step - 1);
    }
  }, [step]);

  const handleCreate = useCallback(
    async (saveAsDraft: boolean) => {
      const parsedPrice = parseFloat(price);
      if (!name.trim() || isNaN(parsedPrice)) {
        Alert.alert("Validation", "Product name and price are required.");
        return;
      }

      setIsSubmitting(true);
      const parsedCompare = compareAtPrice ? parseFloat(compareAtPrice) : undefined;
      const parsedStock = parseInt(stock, 10);
      const tagList = tags
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean);

      createMutation.mutate(
        {
          name: name.trim(),
          description: description.trim() || undefined,
          price: parsedPrice,
          compare_at_price: parsedCompare,
          sku: sku.trim() || undefined,
          stock: isNaN(parsedStock) ? 0 : parsedStock,
          status: saveAsDraft ? "draft" : status,
          category_id: categoryId.trim() || undefined,
          tags: tagList.length > 0 ? tagList : undefined,
        },
        {
          onSuccess: async (data) => {
            // Upload images sequentially
            for (const uri of images) {
              try {
                await uploadMediaMutation.mutateAsync({
                  productId: data.id,
                  uri,
                });
              } catch {
                // Continue uploading remaining images even if one fails
              }
            }
            setIsSubmitting(false);
            router.back();
          },
          onError: () => {
            setIsSubmitting(false);
            Alert.alert("Error", "Failed to create product. Please try again.");
          },
        },
      );
    },
    [
      name, description, price, compareAtPrice, sku, stock, status,
      categoryId, tags, images, createMutation, uploadMediaMutation, router,
    ],
  );

  const renderStep = () => {
    switch (step) {
      case 1:
        return (
          <View style={styles.stepContent}>
            <Text style={styles.stepTitle}>Product Photos</Text>
            <Text style={styles.stepSubtitle}>
              Take photos or choose from your library. You can add more later.
            </Text>
            <ProductMediaPicker images={images} onImagesChange={setImages} />
            {images.length === 0 && (
              <Text style={styles.hint}>
                Tip: Good product photos increase sales by up to 40%
              </Text>
            )}
          </View>
        );

      case 2:
        return (
          <View style={styles.stepContent}>
            <Text style={styles.stepTitle}>Product Details</Text>
            <FieldLabel label="Name *" />
            <TextInput
              style={styles.input}
              value={name}
              onChangeText={setName}
              placeholder="e.g. Handmade Ceramic Mug"
              placeholderTextColor={`${theme.colors.text}50`}
              autoFocus
              accessibilityLabel="Product name"
            />
            <FieldLabel label="Description" />
            <TextInput
              style={[styles.input, styles.multilineInput]}
              value={description}
              onChangeText={setDescription}
              placeholder="Describe your product..."
              placeholderTextColor={`${theme.colors.text}50`}
              multiline
              numberOfLines={4}
              textAlignVertical="top"
              accessibilityLabel="Product description"
            />
            <View style={styles.row}>
              <View style={styles.halfField}>
                <FieldLabel label="Price *" />
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
                  placeholder="SKU-001"
                  placeholderTextColor={`${theme.colors.text}50`}
                  autoCapitalize="characters"
                  accessibilityLabel="Product SKU"
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
          </View>
        );

      case 3:
        return (
          <View style={styles.stepContent}>
            <Text style={styles.stepTitle}>Organization</Text>
            <FieldLabel label="Category ID" />
            <TextInput
              style={styles.input}
              value={categoryId}
              onChangeText={setCategoryId}
              placeholder="Category ID (optional)"
              placeholderTextColor={`${theme.colors.text}50`}
              accessibilityLabel="Category ID"
            />
            <FieldLabel label="Tags" />
            <TextInput
              style={styles.input}
              value={tags}
              onChangeText={setTags}
              placeholder="tag1, tag2, tag3"
              placeholderTextColor={`${theme.colors.text}50`}
              autoCapitalize="none"
              accessibilityLabel="Product tags"
            />
            <FieldLabel label="Status" />
            <View style={styles.statusRow}>
              {STATUS_OPTIONS.map((opt) => {
                const isSelected = status === opt.key;
                return (
                  <TouchableOpacity
                    key={opt.key}
                    style={[
                      styles.statusOption,
                      isSelected && styles.statusOptionSelected,
                    ]}
                    onPress={() => setStatus(opt.key)}
                    accessibilityRole="button"
                    accessibilityState={{ selected: isSelected }}
                    accessibilityLabel={`Status: ${opt.label}`}
                  >
                    <Text
                      style={[
                        styles.statusOptionText,
                        isSelected && styles.statusOptionTextSelected,
                      ]}
                    >
                      {opt.label}
                    </Text>
                  </TouchableOpacity>
                );
              })}
            </View>
          </View>
        );

      case 4:
        return (
          <View style={styles.stepContent}>
            <Text style={styles.stepTitle}>Review</Text>
            <View style={styles.reviewCard}>
              <ReviewRow label="Photos" value={`${images.length} image${images.length !== 1 ? "s" : ""}`} />
              <ReviewRow label="Name" value={name || "Not set"} />
              <ReviewRow label="Price" value={price ? `$${price}` : "Not set"} />
              {compareAtPrice ? <ReviewRow label="Compare at" value={`$${compareAtPrice}`} /> : null}
              {sku ? <ReviewRow label="SKU" value={sku} /> : null}
              <ReviewRow label="Stock" value={stock || "0"} />
              {categoryId ? <ReviewRow label="Category" value={categoryId} /> : null}
              {tags ? <ReviewRow label="Tags" value={tags} /> : null}
              <ReviewRow label="Status" value={status} />
            </View>
          </View>
        );

      default:
        return null;
    }
  };

  if (isSubmitting) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color={theme.colors.text} />
        <Text style={styles.loadingText}>Creating product...</Text>
      </View>
    );
  }

  return (
    <View style={styles.screen}>
      <StepIndicator current={step} total={TOTAL_STEPS} />
      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        keyboardShouldPersistTaps="handled"
      >
        {renderStep()}
      </ScrollView>
      <View style={styles.footer}>
        {step > 1 && (
          <TouchableOpacity
            style={styles.backBtn}
            onPress={handleBack}
            activeOpacity={0.7}
            accessibilityRole="button"
            accessibilityLabel="Go back to previous step"
          >
            <Text style={styles.backBtnText}>Back</Text>
          </TouchableOpacity>
        )}
        <View style={styles.footerSpacer} />
        {step === TOTAL_STEPS ? (
          <View style={styles.finalActions}>
            <TouchableOpacity
              style={styles.draftBtn}
              onPress={() => handleCreate(true)}
              activeOpacity={0.7}
              accessibilityRole="button"
              accessibilityLabel="Save product as draft"
            >
              <Text style={styles.draftBtnText}>Save as Draft</Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.createBtn}
              onPress={() => handleCreate(false)}
              activeOpacity={0.7}
              accessibilityRole="button"
              accessibilityLabel="Create product"
            >
              <Text style={styles.createBtnText}>Create Product</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <TouchableOpacity
            style={styles.nextBtn}
            onPress={handleNext}
            activeOpacity={0.7}
            accessibilityRole="button"
            accessibilityLabel="Go to next step"
          >
            <Text style={styles.nextBtnText}>Next</Text>
          </TouchableOpacity>
        )}
      </View>
    </View>
  );
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.reviewRow}>
      <Text style={styles.reviewLabel}>{label}</Text>
      <Text style={styles.reviewValue}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    paddingBottom: theme.spacing.xxl,
  },
  stepContent: {
    paddingHorizontal: theme.spacing.lg,
  },
  stepTitle: {
    fontSize: 20,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: theme.spacing.xs,
  },
  stepSubtitle: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
    marginBottom: theme.spacing.lg,
  },
  hint: {
    fontSize: 12,
    color: theme.colors.accent,
    opacity: 0.7,
    fontStyle: "italic",
    marginTop: theme.spacing.lg,
    textAlign: "center",
  },
  fieldLabel: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.6,
    marginBottom: theme.spacing.xs,
    marginTop: theme.spacing.md,
  },
  input: {
    backgroundColor: theme.colors.elevated,
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
  statusRow: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.xs,
  },
  statusOption: {
    flex: 1,
    paddingVertical: 10,
    borderRadius: theme.radius,
    alignItems: "center",
    backgroundColor: theme.colors.elevated,
    borderWidth: 1,
    borderColor: `${theme.colors.text}20`,
  },
  statusOptionSelected: {
    backgroundColor: theme.colors.text,
    borderColor: theme.colors.text,
  },
  statusOptionText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
  },
  statusOptionTextSelected: {
    color: theme.colors.background,
  },
  reviewCard: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: 14,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
    marginTop: theme.spacing.md,
  },
  reviewRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.sm,
    borderBottomWidth: 0.5,
    borderBottomColor: `${theme.colors.text}10`,
  },
  reviewLabel: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
    flex: 1,
  },
  reviewValue: {
    fontSize: 13,
    color: theme.colors.text,
    fontWeight: "500",
    flex: 2,
    textAlign: "right",
    textTransform: "capitalize",
  },
  footer: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    borderTopWidth: 0.5,
    borderTopColor: `${theme.colors.text}10`,
    backgroundColor: theme.colors.background,
  },
  footerSpacer: {
    flex: 1,
  },
  backBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: 10,
    borderRadius: theme.radius,
  },
  backBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.6,
  },
  nextBtn: {
    backgroundColor: theme.colors.text,
    paddingHorizontal: theme.spacing.xxl,
    paddingVertical: theme.spacing.md,
    borderRadius: theme.radius,
  },
  nextBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.background,
  },
  finalActions: {
    flexDirection: "row",
    gap: theme.spacing.sm,
  },
  draftBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    borderRadius: theme.radius,
    borderWidth: 1,
    borderColor: `${theme.colors.text}20`,
  },
  draftBtnText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.7,
  },
  createBtn: {
    backgroundColor: theme.colors.accent,
    paddingHorizontal: theme.spacing.xl,
    paddingVertical: theme.spacing.md,
    borderRadius: theme.radius,
  },
  createBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.elevated,
  },
  loadingContainer: {
    flex: 1,
    backgroundColor: theme.colors.background,
    justifyContent: "center",
    alignItems: "center",
    gap: theme.spacing.md,
  },
  loadingText: {
    fontSize: 14,
    color: theme.colors.text,
    opacity: 0.6,
  },
});
