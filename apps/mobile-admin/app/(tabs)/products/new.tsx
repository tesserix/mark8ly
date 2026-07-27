import { useCallback, useMemo, useState } from "react";
import {
  View,
  Alert,
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { useRouter } from "expo-router";
import { BottomSheetModalProvider } from "@gorhom/bottom-sheet";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import {
  useCreateProduct,
  useUpdateProduct,
  useCategories,
  useCreateCategory,
} from "../../../lib/admin-api/product-crud";
import { BackHeader, Card, Eyebrow, FieldInput, Screen, SegmentedControl, Text } from "@/components/ui";
import { CategoryField } from "@/components/products/CategoryField";
import { theme } from "@/lib/theme";
import { deriveSku } from "@/lib/sku";
import { getErrorMessage } from "@/lib/product-alerts";
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
  // This screen is presented as a MODAL (products/_layout.tsx) — it covers the
  // tab dock, so clear the home indicator with the safe-area inset, NOT
  // useDockClearance() (which would float the footer ~100px above dead space).
  const insets = useSafeAreaInsets();
  const createMutation = useCreateProduct();
  const updateMutation = useUpdateProduct();
  const createCategory = useCreateCategory();
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
    // This modal screen sits ABOVE the root BottomSheetModalProvider, so it
    // needs its own provider — otherwise CategoryField's picker sheet presents
    // behind the modal and nothing appears to happen. (Gestures still work via
    // the root GestureHandlerRootView, which covers react-native-screens modals.)
    <BottomSheetModalProvider>
      {/* topInset=false: the modal card is already presented below the notch,
          so Screen's own safe-area top padding would double it. */}
      <Screen topInset={false}>
        <BackHeader eyebrow="NEW PRODUCT" title="New product" />

        {/* Keep the floating footer (Create / Save) above the keyboard on iOS.
            Android relies on the default adjustResize windowSoftInputMode. */}
        <KeyboardAvoidingView
          style={styles.kav}
          behavior={Platform.OS === "ios" ? "padding" : undefined}
        >
          <ScrollView
            contentContainerStyle={[styles.scroll, { paddingBottom: insets.bottom + FOOTER_CLEARANCE }]}
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
            onCreateCategory={(name) => createCategory.mutateAsync({ name }).then((res) => res.data)}
            isCreatingCategory={createCategory.isPending}
          />
        </Card>
      </ScrollView>

      <View style={[styles.footer, { bottom: insets.bottom }]}>
        <Pressable
          onPress={() => handleSubmit(true)}
          disabled={isBusy}
          accessibilityRole="button"
          accessibilityLabel="Save as draft"
          android_ripple={{ color: "rgba(14, 14, 12, 0.12)" }}
          style={({ pressed }) => [
            styles.draftBtn,
            isBusy && styles.disabled,
            pressed && Platform.OS === "ios" ? { opacity: 0.55 } : null,
          ]}
        >
          <Text preset="bodyEmphasis" color="text">
            Save as draft
          </Text>
        </Pressable>
        <Pressable
          onPress={() => handleSubmit(false)}
          disabled={isBusy}
          accessibilityRole="button"
          accessibilityLabel="Create product"
          android_ripple={{ color: "rgba(247, 246, 242, 0.24)" }}
          style={({ pressed }) => [
            styles.primaryBtn,
            isBusy && styles.disabled,
            pressed && Platform.OS === "ios" ? { opacity: 0.85 } : null,
          ]}
        >
          {isBusy ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Create product
            </Text>
          )}
        </Pressable>
      </View>
        </KeyboardAvoidingView>
      </Screen>
    </BottomSheetModalProvider>
  );
}

const styles = StyleSheet.create({
  kav: { flex: 1 },
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
