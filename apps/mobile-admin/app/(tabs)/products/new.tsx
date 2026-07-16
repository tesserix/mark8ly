import { useCallback, useMemo, useState } from "react";
import { View, Alert, ActivityIndicator, TouchableOpacity, ScrollView, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useCreateProduct, useUpdateProduct, useCategories } from "../../../lib/admin-api/product-crud";
import { BackHeader, Card, Eyebrow, FieldInput, Screen, SegmentedControl, Text } from "@/components/ui";
import { CategoryField } from "@/components/products/CategoryField";
import { theme } from "@/lib/theme";
import { deriveSku } from "@/lib/sku";
import { getErrorMessage } from "@/lib/product-alerts";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import type { CategoryRef } from "@repo/mobile-shared/api/schemas/categories";

/** The backend enum is draft|active|archived — "inactive" is a 400. */
type ProductStatus = "draft" | "active";

const STATUS_OPTIONS: { key: ProductStatus; label: string }[] = [
  { key: "draft", label: "Draft" },
  { key: "active", label: "Active" },
];

/** Extra room the floating footer needs beyond the dock's own clearance. */
const FOOTER_CLEARANCE = 104;

export default function NewProductScreen() {
  const router = useRouter();
  const dockPad = useDockClearance();
  const createMutation = useCreateProduct();
  const updateMutation = useUpdateProduct();
  const { data: categories, isLoading: categoriesLoading, error: categoriesError, refetch: refetchCategories } =
    useCategories();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [sku, setSku] = useState("");
  const [stock, setStock] = useState("");
  const [status, setStatus] = useState<ProductStatus>("draft");
  const [selectedCategoryIds, setSelectedCategoryIds] = useState<string[]>([]);

  // Inline validation errors — surfaced under the field, never as an Alert.
  const [titleError, setTitleError] = useState<string | null>(null);
  const [priceError, setPriceError] = useState<string | null>(null);

  const isBusy = createMutation.isPending || updateMutation.isPending;

  // Category has no product yet to read `selected` off of — resolve the
  // picked ids against the fetched category list into the CategoryRef[]
  // shape CategoryField expects.
  const selectedCategories = useMemo<CategoryRef[]>(() => {
    if (!categories) return [];
    return categories
      .filter((c) => selectedCategoryIds.includes(c.id))
      .map((c) => ({ id: c.id, name: c.name, slug: c.slug }));
  }, [categories, selectedCategoryIds]);

  const handleSubmit = useCallback(
    (asDraft: boolean) => {
      const trimmedTitle = title.trim();
      const parsedPrice = parseFloat(price);
      const titleValid = trimmedTitle.length > 0;
      const priceValid = price.trim().length > 0 && !isNaN(parsedPrice) && parsedPrice >= 0;

      setTitleError(titleValid ? null : "Title is required.");
      setPriceError(priceValid ? null : "Enter a valid, non-negative price.");
      if (!titleValid || !priceValid) return;

      const parsedStock = parseInt(stock, 10);

      createMutation.mutate(
        {
          title: trimmedTitle,
          description: description.trim() || undefined,
          status: asDraft ? "draft" : status,
          // Price/SKU/stock live on the variant — the product itself has none.
          // CreateProductRequest requires at least one variant.
          variants: [
            {
              sku: sku.trim() || deriveSku(trimmedTitle),
              price: parsedPrice,
              currency_code: "AUD",
              inventory_quantity: isNaN(parsedStock) ? 0 : parsedStock,
              position: 0,
            },
          ],
        },
        {
          onSuccess: (product) => {
            const goToEdit = () => {
              router.replace({
                pathname: "/(tabs)/products/[id]",
                params: { id: product.id, created: "1" },
              });
            };

            // CreateProductBody has no category_ids — only UpdateProductBody
            // does. If the merchant picked a category, follow up with ONE
            // PATCH through the verified edit contract, then hand off either
            // way (a failed category patch shouldn't strand a created product
            // with no way forward — the merchant can retry from the edit
            // screen).
            if (selectedCategoryIds.length === 0) {
              goToEdit();
              return;
            }
            updateMutation.mutate(
              { id: product.id, body: { category_ids: selectedCategoryIds } },
              {
                onSuccess: goToEdit,
                onError: (err) => {
                  Alert.alert(
                    "Error",
                    getErrorMessage(
                      err,
                      "Failed to set categories. You can add them from the product page.",
                    ),
                  );
                  goToEdit();
                },
              },
            );
          },
          onError: (err) => {
            Alert.alert("Error", getErrorMessage(err, "Failed to create product. Please try again."));
          },
        },
      );
    },
    [title, description, price, sku, stock, status, selectedCategoryIds, createMutation, updateMutation, router],
  );

  return (
    <Screen>
      <BackHeader eyebrow="NEW PRODUCT" title="New product" />

      <ScrollView
        contentContainerStyle={[styles.scroll, { paddingBottom: dockPad + FOOTER_CLEARANCE }]}
        keyboardShouldPersistTaps="handled"
      >
        <Eyebrow label="Essentials" />
        <Card variant="ghost" padding="md" style={styles.card}>
          <FieldInput
            label="Title *"
            value={title}
            onChangeText={(v) => {
              setTitle(v);
              if (titleError) setTitleError(null);
            }}
            placeholder="e.g. Handmade Ceramic Mug"
            accessibilityLabel="Title"
            autoFocus
          />
          {titleError ? (
            <Text preset="caption" color="danger">
              {titleError}
            </Text>
          ) : null}

          <View style={styles.row}>
            <View style={styles.half}>
              <FieldInput
                label="Price (AUD) *"
                value={price}
                onChangeText={(v) => {
                  setPrice(v);
                  if (priceError) setPriceError(null);
                }}
                keyboardType="decimal-pad"
                placeholder="0.00"
                accessibilityLabel="Price (AUD)"
              />
              {priceError ? (
                <Text preset="caption" color="danger">
                  {priceError}
                </Text>
              ) : null}
            </View>
            <View style={styles.half}>
              <FieldInput
                label="Stock"
                value={stock}
                onChangeText={setStock}
                keyboardType="number-pad"
                placeholder="0"
                accessibilityLabel="Stock"
              />
            </View>
          </View>

          <FieldInput
            label="SKU"
            value={sku}
            onChangeText={setSku}
            autoCapitalize="characters"
            placeholder={title.trim() ? deriveSku(title) : "SKU-001"}
            accessibilityLabel="SKU"
          />
          <FieldInput
            label="Description"
            value={description}
            onChangeText={setDescription}
            placeholder="Describe your product…"
            multiline
            numberOfLines={4}
            accessibilityLabel="Description"
          />
        </Card>

        <Eyebrow label="Status" />
        <Card variant="ghost" padding="md" style={styles.card}>
          <SegmentedControl<ProductStatus> segments={STATUS_OPTIONS} value={status} onChange={setStatus} />
        </Card>

        <Eyebrow label="Category" />
        <Card variant="ghost" padding="md" style={styles.card}>
          <CategoryField
            categories={categories ?? []}
            selected={selectedCategories}
            onChange={setSelectedCategoryIds}
            isLoading={categoriesLoading}
            error={categoriesError}
            onRetry={refetchCategories}
          />
        </Card>
      </ScrollView>

      <View style={[styles.footer, { bottom: dockPad }]}>
        <TouchableOpacity
          style={[styles.draftBtn, isBusy && styles.disabled]}
          onPress={() => handleSubmit(true)}
          disabled={isBusy}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel="Save as draft"
        >
          <Text preset="bodyEmphasis" color="text">
            Save as draft
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.primaryBtn, isBusy && styles.disabled]}
          onPress={() => handleSubmit(false)}
          disabled={isBusy}
          activeOpacity={0.85}
          accessibilityRole="button"
          accessibilityLabel="Create product"
        >
          {isBusy ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Create product
            </Text>
          )}
        </TouchableOpacity>
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingTop: theme.spacing.md },
  card: { marginHorizontal: theme.spacing.lg, gap: theme.spacing.sm },
  row: { flexDirection: "row", gap: theme.spacing.sm },
  half: { flex: 1 },
  footer: {
    position: "absolute",
    left: 0,
    right: 0,
    flexDirection: "row",
    gap: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.background,
    borderTopWidth: theme.hairline,
    borderTopColor: theme.colors.hairline,
  },
  draftBtn: {
    paddingHorizontal: theme.spacing.lg,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    minHeight: 44,
    alignItems: "center",
    justifyContent: "center",
  },
  primaryBtn: {
    flex: 1,
    backgroundColor: theme.colors.accent,
    borderRadius: theme.radii.md,
    minHeight: 44,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.6 },
});
