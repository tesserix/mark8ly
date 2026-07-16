import { useRef } from "react";
import { View, Pressable, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Category, CategoryRef } from "@repo/mobile-shared/api/schemas/categories";
import { CategoryPickerSheet, type CategoryPickerSheetHandle } from "./CategoryPickerSheet";

/** Beyond this many selected categories, the field folds the rest into a "+N" chip. */
const MAX_VISIBLE_CHIPS = 4;

interface CategoryFieldProps {
  categories: Category[];
  /** What the product currently links to — the lean refs the product embeds. */
  selected: CategoryRef[];
  /** Receives the FULL desired id set. The backend REPLACES links, not merges. */
  onChange: (ids: string[]) => void;
  /** Categories are still loading — field renders disabled with a status label. */
  isLoading?: boolean;
  /** Categories failed to load — field becomes a retry affordance instead of opening the sheet. */
  error?: unknown;
  onRetry?: () => void;
}

export function CategoryField({
  categories,
  selected,
  onChange,
  isLoading = false,
  error,
  onRetry,
}: CategoryFieldProps) {
  const sheetRef = useRef<CategoryPickerSheetHandle>(null);

  const visible = selected.slice(0, MAX_VISIBLE_CHIPS);
  const overflow = selected.length - visible.length;

  // The sheet stays mounted across all three field states — a refetch that
  // flips `isLoading` true while the sheet is already open (open, edit,
  // refetch mid-edit) must show its own spinner, not unmount the whole
  // BottomSheetModal out from under the merchant.
  return (
    <>
      {isLoading ? (
        <View style={[styles.field, styles.fieldDisabled]}>
          <Text preset="body" color="textTertiary">
            Loading categories…
          </Text>
        </View>
      ) : error ? (
        <Pressable
          style={styles.field}
          onPress={onRetry}
          accessibilityRole="button"
          accessibilityLabel="Couldn't load categories, tap to retry"
        >
          <Text preset="body" color="danger">
            Couldn&apos;t load, tap to retry
          </Text>
        </Pressable>
      ) : (
        <Pressable
          style={styles.field}
          onPress={() => sheetRef.current?.present()}
          accessibilityRole="button"
          accessibilityLabel={`Categories, ${selected.length} selected, edit`}
        >
          {selected.length === 0 ? (
            <Text preset="body" color="textTertiary">
              Add categories
            </Text>
          ) : (
            <View style={styles.chips}>
              {visible.map((c) => (
                <View key={c.id} style={styles.chip}>
                  <Text preset="caption" color="text">
                    {c.name}
                  </Text>
                </View>
              ))}
              {overflow > 0 ? (
                <View style={styles.chip}>
                  <Text preset="caption" color="textSecondary">
                    +{overflow}
                  </Text>
                </View>
              ) : null}
            </View>
          )}
          <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
        </Pressable>
      )}
      <CategoryPickerSheet
        ref={sheetRef}
        categories={categories}
        selected={selected}
        onApply={onChange}
        isLoading={isLoading}
      />
    </>
  );
}

const styles = StyleSheet.create({
  field: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    minHeight: 44,
    gap: theme.spacing.sm,
  },
  fieldDisabled: { opacity: 0.5 },
  chips: { flex: 1, flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.sm,
    height: 32,
    borderRadius: theme.radii.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.elevated,
  },
});
