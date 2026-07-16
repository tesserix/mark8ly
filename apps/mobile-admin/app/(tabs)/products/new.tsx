import { useState, useCallback } from "react";
import {
  View,
  ScrollView,
  TextInput,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { useCreateProduct } from "../../../lib/admin-api/product-crud";
import {
  BackHeader,
  Card,
  Hairline,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";

// Was 4 — the "Photos" step is gone. Media upload needs a 3-step signed-URL
// flow (POST /media/upload-url -> PUT -> POST /media) that this app does not
// implement; the old step let a merchant pick images that were then dropped.
const TOTAL_STEPS = 3;

/** The backend enum is draft|active|archived — "inactive" is a 400. */
type ProductStatus = "draft" | "active";

const STATUS_OPTIONS: { key: ProductStatus; label: string }[] = [
  { key: "draft", label: "Draft" },
  { key: "active", label: "Active" },
];

/**
 * CreateProductVariantInput.SKU is `binding:"required,max=100"` — a product
 * cannot be created without one. When the merchant leaves SKU blank we derive
 * a stable one from the title rather than fail the request.
 */
function deriveSku(title: string): string {
  return `${title.trim().toUpperCase().replace(/[^A-Z0-9]+/g, "-").slice(0, 40)}-1`;
}

function StepDots({ current, total }: { current: number; total: number }) {
  return (
    <View style={stepStyles.row}>
      {Array.from({ length: total }).map((_, i) => {
        const idx = i + 1;
        const active = idx === current;
        const done = idx < current;
        return (
          <View
            key={i}
            style={[
              stepStyles.dot,
              active && stepStyles.dotActive,
              done && stepStyles.dotDone,
            ]}
          />
        );
      })}
    </View>
  );
}

const stepStyles = StyleSheet.create({
  row: {
    flexDirection: "row",
    gap: theme.spacing.xs,
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    paddingBottom: theme.spacing.sm,
  },
  dot: {
    flex: 1,
    height: 3,
    backgroundColor: theme.colors.border,
    borderRadius: 2,
  },
  dotActive: { backgroundColor: theme.colors.text },
  dotDone: { backgroundColor: theme.colors.accent },
});

function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textSecondary" style={styles.fieldLabel}>
      {label}
    </Text>
  );
}

export default function NewProductScreen() {
  const router = useRouter();
  const createMutation = useCreateProduct();

  const [step, setStep] = useState(1);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [sku, setSku] = useState("");
  const [stock, setStock] = useState("");
  const [tags, setTags] = useState("");
  const [status, setStatus] = useState<ProductStatus>("draft");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const canProceed = useCallback((): boolean => {
    if (step === 1) {
      return title.trim().length > 0 && !isNaN(parseFloat(price)) && parseFloat(price) >= 0;
    }
    return true;
  }, [step, title, price]);

  const handleNext = useCallback(() => {
    if (!canProceed()) {
      Alert.alert("Validation", "Please fill in all required fields (title and price).");
      return;
    }
    if (step < TOTAL_STEPS) setStep(step + 1);
  }, [step, canProceed]);

  const handleBack = useCallback(() => {
    if (step > 1) setStep(step - 1);
  }, [step]);

  const handleCreate = useCallback(
    (saveAsDraft: boolean) => {
      const parsedPrice = parseFloat(price);
      if (!title.trim() || isNaN(parsedPrice)) {
        Alert.alert("Validation", "Product title and price are required.");
        return;
      }
      setIsSubmitting(true);
      const parsedStock = parseInt(stock, 10);
      const tagList = tags.split(",").map((t) => t.trim()).filter(Boolean);

      createMutation.mutate(
        {
          title: title.trim(),
          description: description.trim() || undefined,
          status: saveAsDraft ? "draft" : status,
          tags: tagList.length > 0 ? tagList : undefined,
          // Price/SKU/stock live on the variant — the product itself has none.
          // CreateProductRequest requires at least one variant.
          variants: [
            {
              sku: sku.trim() || deriveSku(title),
              price: parsedPrice,
              currency_code: "AUD",
              inventory_quantity: isNaN(parsedStock) ? 0 : parsedStock,
              position: 0,
            },
          ],
        },
        {
          onSuccess: () => {
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
    [title, description, price, sku, stock, status, tags, createMutation, router],
  );

  if (isSubmitting) {
    return (
      <Screen>
        <BackHeader eyebrow="NEW PRODUCT" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
          <Text preset="body" color="textSecondary">
            Creating product…
          </Text>
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <BackHeader
        eyebrow={`STEP ${step} OF ${TOTAL_STEPS}`}
        title="New Product"
      />
      <StepDots current={step} total={TOTAL_STEPS} />

      <ScrollView
        contentContainerStyle={styles.scroll}
        keyboardShouldPersistTaps="handled"
      >
        {step === 1 ? (
          <View style={styles.stepContent}>
            <Text preset="h2" color="text">
              Details
            </Text>
            <FieldLabel label="Title *" />
            <TextInput
              style={styles.input}
              value={title}
              onChangeText={setTitle}
              placeholder="e.g. Handmade Ceramic Mug"
              placeholderTextColor={theme.colors.textTertiary}
              autoFocus
            />
            <FieldLabel label="Description" />
            <TextInput
              style={[styles.input, styles.multiline]}
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
                <FieldLabel label="Price (AUD) *" />
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
            <FieldLabel label="SKU" />
            <TextInput
              style={styles.input}
              value={sku}
              onChangeText={setSku}
              autoCapitalize="characters"
              placeholder={title.trim() ? deriveSku(title) : "SKU-001"}
              placeholderTextColor={theme.colors.textTertiary}
            />
          </View>
        ) : null}

        {step === 2 ? (
          <View style={styles.stepContent}>
            <Text preset="h2" color="text">
              Organization
            </Text>
            <FieldLabel label="Tags" />
            <TextInput
              style={styles.input}
              value={tags}
              onChangeText={setTags}
              placeholder="tag1, tag2, tag3"
              placeholderTextColor={theme.colors.textTertiary}
              autoCapitalize="none"
            />
            <FieldLabel label="Status" />
            <View style={styles.statusRow}>
              {STATUS_OPTIONS.map((opt) => {
                const selected = status === opt.key;
                return (
                  <TouchableOpacity
                    key={opt.key}
                    style={[styles.statusOpt, selected && styles.statusOptSelected]}
                    onPress={() => setStatus(opt.key)}
                    accessibilityRole="button"
                    accessibilityState={{ selected }}
                    accessibilityLabel={`Status: ${opt.label}`}
                  >
                    <Text
                      preset="bodyEmphasis"
                      color={selected ? "inverse" : "text"}
                    >
                      {opt.label}
                    </Text>
                  </TouchableOpacity>
                );
              })}
            </View>
          </View>
        ) : null}

        {step === 3 ? (
          <View style={styles.stepContent}>
            <Text preset="h2" color="text">
              Review
            </Text>
            <Card padding={0} style={styles.reviewCard}>
              <ReviewRow label="Title" value={title || "Not set"} />
              <Hairline />
              <ReviewRow label="Price" value={price ? `A$${price}` : "Not set"} />
              <Hairline />
              <ReviewRow label="SKU" value={sku || (title.trim() ? deriveSku(title) : "Not set")} />
              <Hairline />
              <ReviewRow label="Stock" value={stock || "0"} />
              {tags ? (
                <>
                  <Hairline />
                  <ReviewRow label="Tags" value={tags} />
                </>
              ) : null}
              <Hairline />
              <ReviewRow label="Status" value={status} />
            </Card>
          </View>
        ) : null}
      </ScrollView>

      <View style={styles.footer}>
        {step > 1 ? (
          <TouchableOpacity
            style={styles.backBtn}
            onPress={handleBack}
            activeOpacity={0.7}
            accessibilityRole="button"
            accessibilityLabel="Go back to previous step"
          >
            <Text preset="bodyEmphasis" color="textSecondary">
              Back
            </Text>
          </TouchableOpacity>
        ) : (
          <View style={styles.backBtn} />
        )}
        {step === TOTAL_STEPS ? (
          <View style={styles.finalActions}>
            <TouchableOpacity
              style={styles.draftBtn}
              onPress={() => handleCreate(true)}
              activeOpacity={0.7}
              accessibilityRole="button"
              accessibilityLabel="Save as draft"
            >
              <Text preset="bodyEmphasis" color="text">
                Draft
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.primaryBtn}
              onPress={() => handleCreate(false)}
              activeOpacity={0.85}
              accessibilityRole="button"
              accessibilityLabel="Create product"
            >
              <Text preset="bodyEmphasis" color="inverse">
                Create
              </Text>
            </TouchableOpacity>
          </View>
        ) : (
          <TouchableOpacity
            style={styles.primaryBtn}
            onPress={handleNext}
            activeOpacity={0.85}
            accessibilityRole="button"
            accessibilityLabel="Next step"
          >
            <Text preset="bodyEmphasis" color="inverse">
              Next
            </Text>
          </TouchableOpacity>
        )}
      </View>
    </Screen>
  );
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.reviewRow}>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
      <Text preset="body" color="text" style={styles.reviewValue}>
        {value}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  stepContent: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    gap: theme.spacing.sm,
  },
  fieldLabel: {
    marginTop: theme.spacing.md,
    marginBottom: theme.spacing.xs,
    letterSpacing: 0.4,
  },
  input: {
    backgroundColor: theme.colors.elevated,
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
  multiline: { minHeight: 88, paddingTop: theme.spacing.sm },
  row: { flexDirection: "row", gap: theme.spacing.sm },
  half: { flex: 1 },
  statusRow: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.xs,
  },
  statusOpt: {
    flex: 1,
    paddingVertical: theme.spacing.sm,
    borderRadius: theme.radii.md,
    alignItems: "center",
    backgroundColor: theme.colors.elevated,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    minHeight: 44,
    justifyContent: "center",
  },
  statusOptSelected: {
    backgroundColor: theme.colors.text,
    borderColor: theme.colors.text,
  },
  reviewCard: { marginTop: theme.spacing.md },
  reviewRow: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.md,
  },
  reviewValue: {
    flex: 2,
    textAlign: "right",
    textTransform: "capitalize",
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.md,
  },
  footer: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    paddingBottom: theme.spacing.lg,
    borderTopWidth: theme.hairline,
    borderTopColor: theme.colors.hairline,
    backgroundColor: theme.colors.background,
  },
  backBtn: {
    minWidth: 64,
    minHeight: 44,
    justifyContent: "center",
  },
  primaryBtn: {
    backgroundColor: theme.colors.accent,
    paddingHorizontal: theme.spacing.xl,
    paddingVertical: theme.spacing.sm,
    borderRadius: theme.radii.md,
    minHeight: 44,
    justifyContent: "center",
  },
  finalActions: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    alignItems: "center",
  },
  draftBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.sm,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    minHeight: 44,
    justifyContent: "center",
  },
});
