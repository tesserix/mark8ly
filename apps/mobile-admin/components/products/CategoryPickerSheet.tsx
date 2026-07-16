import { forwardRef, useImperativeHandle, useMemo, useRef, useState, type ComponentType } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal, BottomSheetFlatList as GorhomBottomSheetFlatList } from "@gorhom/bottom-sheet";
import * as Haptics from "expo-haptics";
import { Check } from "lucide-react-native";
import { Text, SearchField, EmptyState } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Category, CategoryRef } from "@repo/mobile-shared/api/schemas/categories";
import { sortCategoryTree, type CategoryNode } from "./CategoryPicker";

/**
 * @gorhom/bottom-sheet ships its own copy of @types/react, whose `ReactNode`
 * includes `bigint`; this project's does not, so the raw export trips TS2786
 * ("cannot be used as a JSX component") — same fix as `OptionBuilderSheet`'s
 * `ScrollBody` cast. Re-typed through this project's React to the props
 * actually used here; runtime is unaffected.
 */
const BottomSheetFlatList = GorhomBottomSheetFlatList as unknown as ComponentType<{
  data: CategoryNode[];
  keyExtractor: (item: CategoryNode) => string;
  contentContainerStyle?: unknown;
  renderItem: (info: { item: CategoryNode }) => JSX.Element;
}>;

/**
 * Matches-only filter over an already-flattened, depth-tagged tree.
 *
 * Pure and exported so it can be pinned without mounting the sheet's portal
 * content (impractical under this project's jest setup — see
 * `CategoryField`'s test file header). Case-insensitive substring match on
 * name; an empty (or whitespace-only) query restores the full tree.
 */
export function filterTree(nodes: CategoryNode[], query: string): CategoryNode[] {
  const trimmed = query.trim().toLowerCase();
  if (trimmed === "") return nodes;
  return nodes.filter((n) => n.category.name.toLowerCase().includes(trimmed));
}

export interface CategoryPickerSheetHandle {
  present: () => void;
}

interface CategoryPickerSheetProps {
  categories: Category[];
  /** What the product currently links to — seeds the staged selection on open. */
  selected: CategoryRef[];
  /** Called ONCE with the full desired id set when the merchant taps Done. */
  onApply: (ids: string[]) => void;
  /** Categories are still loading — shows a spinner instead of the tree. */
  isLoading?: boolean;
}

export const CategoryPickerSheet = forwardRef<CategoryPickerSheetHandle, CategoryPickerSheetProps>(
  function CategoryPickerSheet({ categories, selected, onApply, isLoading = false }, ref) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [query, setQuery] = useState("");
    const [staged, setStaged] = useState<Set<string>>(new Set());

    useImperativeHandle(ref, () => ({
      present: () => {
        // Every open re-seeds from the product's current links — a stale
        // staged set from a previous dismissed-without-Done attempt must
        // never leak into the next one.
        setStaged(new Set(selected.map((c) => c.id)));
        setQuery("");
        modalRef.current?.present();
      },
    }));

    const nodes = useMemo(() => sortCategoryTree(categories), [categories]);
    const visibleNodes = useMemo(() => filterTree(nodes, query), [nodes, query]);

    const toggle = (id: string) => {
      void Haptics.selectionAsync();
      setStaged((prev) => {
        const next = new Set(prev);
        if (next.has(id)) {
          next.delete(id);
        } else {
          next.add(id);
        }
        return next;
      });
    };

    const handleDone = () => {
      // Preserve the store's own ordering rather than Set insertion order —
      // same rule as CategoryPicker.toggle.
      onApply(nodes.map((n) => n.category.id).filter((id) => staged.has(id)));
      modalRef.current?.dismiss();
    };

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["92%"]}
        enablePanDownToClose
        enableDynamicSizing={false}
      >
        <View style={styles.root}>
          <View style={styles.header}>
            <Text preset="h3" color="text">
              Categories
            </Text>
            <Pressable onPress={handleDone} accessibilityRole="button" accessibilityLabel="Done">
              <Text preset="bodyEmphasis" color="accent">
                Done
              </Text>
            </Pressable>
          </View>

          <SearchField
            value={query}
            onChangeText={setQuery}
            placeholder="Search categories"
            style={styles.search}
          />

          {isLoading ? (
            <View style={styles.centered}>
              <ActivityIndicator color={theme.colors.accent} />
            </View>
          ) : categories.length === 0 ? (
            <EmptyState
              title="No categories yet"
              message="Categories are created from the web admin — this app doesn't support adding them yet."
            />
          ) : visibleNodes.length === 0 ? (
            <Text preset="body" color="textTertiary" style={styles.noResults}>
              No categories match &quot;{query}&quot;
            </Text>
          ) : (
            <BottomSheetFlatList
              data={visibleNodes}
              keyExtractor={(n: CategoryNode) => n.category.id}
              contentContainerStyle={styles.listContent}
              renderItem={({ item }: { item: CategoryNode }) => {
                const isSelected = staged.has(item.category.id);
                return (
                  <Pressable
                    style={[styles.row, { paddingLeft: theme.spacing.md * item.depth }]}
                    onPress={() => toggle(item.category.id)}
                    accessibilityRole="checkbox"
                    accessibilityState={{ checked: isSelected }}
                    accessibilityLabel={item.category.name}
                  >
                    <Text
                      preset="body"
                      color={isSelected ? "text" : "textSecondary"}
                      style={styles.rowText}
                    >
                      {item.category.name}
                    </Text>
                    {isSelected ? (
                      <Check size={18} color={theme.colors.accent} strokeWidth={2.5} />
                    ) : null}
                  </Pressable>
                );
              }}
            />
          )}
        </View>
      </BottomSheetModal>
    );
  },
);

const styles = StyleSheet.create({
  root: { flex: 1, padding: theme.spacing.lg, gap: theme.spacing.md },
  header: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  search: { marginBottom: theme.spacing.xs },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  noResults: { paddingVertical: theme.spacing.lg },
  listContent: { paddingBottom: theme.spacing.huge },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    height: 44,
  },
  rowText: { flex: 1 },
});
