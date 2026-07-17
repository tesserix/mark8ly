import { forwardRef, useImperativeHandle, useMemo, useRef, useState, type ComponentType } from "react";
import { View, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { BottomSheetModal, BottomSheetFlatList as GorhomBottomSheetFlatList } from "@gorhom/bottom-sheet";
import * as Haptics from "expo-haptics";
import { Check, Plus } from "lucide-react-native";
import { Text, SearchField, EmptyState, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";
import { ApiError } from "@repo/mobile-shared/api/client";
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
  /**
   * Create a new category by name; resolves to the created category, which the
   * sheet then stages (selects) so it's part of the applied set. Absent → the
   * "＋ New category" affordance is hidden.
   */
  onCreate?: (name: string) => Promise<Category>;
  /** A create request is in flight — disables the create control + shows a spinner. */
  isCreating?: boolean;
}

export const CategoryPickerSheet = forwardRef<CategoryPickerSheetHandle, CategoryPickerSheetProps>(
  function CategoryPickerSheet(
    { categories, selected, onApply, isLoading = false, onCreate, isCreating = false },
    ref,
  ) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [query, setQuery] = useState("");
    const [staged, setStaged] = useState<Set<string>>(new Set());
    // The inline "new category" composer: `composing` toggles the name field,
    // `newName` holds its draft.
    const [composing, setComposing] = useState(false);
    const [newName, setNewName] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        // Every open re-seeds from the product's current links — a stale
        // staged set from a previous dismissed-without-Done attempt must
        // never leak into the next one.
        setStaged(new Set(selected.map((c) => c.id)));
        setQuery("");
        setComposing(false);
        setNewName("");
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

    const handleCreate = async () => {
      const name = newName.trim();
      if (name === "" || !onCreate) return;
      try {
        const created = await onCreate(name);
        // Stage the new category immediately. The ["categories"] refetch that
        // follows brings it into `nodes` (where it shows a Check); until then
        // handleDone's `extras` guards it from being dropped.
        setStaged((prev) => new Set(prev).add(created.id));
        setNewName("");
        setComposing(false);
      } catch (err) {
        // Duplicate slug / validation / network all surface here — never swallow.
        Alert.alert(
          "Couldn't create category",
          err instanceof ApiError ? err.message : "Please try again.",
        );
      }
    };

    const handleDone = () => {
      // Preserve the store's own ordering rather than Set insertion order —
      // same rule as CategoryPicker.toggle.
      const ordered = nodes.map((n) => n.category.id).filter((id) => staged.has(id));
      // A just-created category may not be in `nodes` yet (its refetch is in
      // flight) — append any staged id the tree doesn't know about so a fresh
      // selection is never dropped on Done.
      const extras = [...staged].filter((id) => !ordered.includes(id));
      onApply([...ordered, ...extras]);
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

          {onCreate ? (
            composing ? (
              <View style={styles.composer}>
                <View style={styles.composerInput}>
                  <FieldInput
                    value={newName}
                    onChangeText={setNewName}
                    onSubmitEditing={handleCreate}
                    placeholder="New category name"
                    accessibilityLabel="New category name"
                    autoFocus
                    returnKeyType="done"
                    editable={!isCreating}
                  />
                </View>
                <Pressable
                  style={[
                    styles.createBtn,
                    (newName.trim() === "" || isCreating) && styles.createBtnDisabled,
                  ]}
                  onPress={handleCreate}
                  disabled={newName.trim() === "" || isCreating}
                  accessibilityRole="button"
                  accessibilityLabel="Create category"
                >
                  {isCreating ? (
                    <ActivityIndicator size="small" color={theme.colors.inverse} />
                  ) : (
                    <Text preset="bodyEmphasis" color="inverse">
                      Create
                    </Text>
                  )}
                </Pressable>
              </View>
            ) : (
              <Pressable
                style={styles.addRow}
                onPress={() => setComposing(true)}
                accessibilityRole="button"
                accessibilityLabel="New category"
              >
                {({ pressed }) => (
                  <>
                    <Plus
                      size={16}
                      color={pressed ? theme.colors.accent : theme.colors.text}
                      strokeWidth={2.5}
                    />
                    <Text preset="bodyEmphasis" color={pressed ? "accent" : "text"}>
                      New category
                    </Text>
                  </>
                )}
              </Pressable>
            )
          ) : null}

          {isLoading ? (
            <View style={styles.centered} testID="category-picker-loading">
              <ActivityIndicator size="small" color={theme.colors.text} />
            </View>
          ) : categories.length === 0 ? (
            <EmptyState
              title="No categories yet"
              message={
                onCreate
                  ? "Add your first category with ＋ New category above."
                  : "Categories are created from the web admin — this app doesn't support adding them yet."
              }
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
  addRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.xs, height: 44 },
  composer: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  composerInput: { flex: 1 },
  createBtn: {
    paddingHorizontal: theme.spacing.lg,
    height: 44,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  createBtnDisabled: { opacity: 0.4 },
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
