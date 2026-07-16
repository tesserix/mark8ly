import { View, Pressable, StyleSheet } from "react-native";
import { Check } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Category, CategoryRef } from "@repo/mobile-shared/api/schemas/categories";

export interface CategoryNode {
  category: Category;
  depth: number;
}

/**
 * Flattens the category tree into a render order, depth-tagged.
 *
 * Robust to bad data on purpose: a category whose parent_id points at a
 * category that doesn't exist (or at itself) is surfaced at root rather than
 * silently dropped — an invisible category is worse than an oddly-placed one.
 */
export function sortCategoryTree(categories: Category[]): CategoryNode[] {
  const byId = new Map(categories.map((c) => [c.id, c]));
  const childrenOf = new Map<string, Category[]>();
  const roots: Category[] = [];

  for (const c of categories) {
    const parentId = c.parent_id;
    const hasRealParent = parentId !== undefined && parentId !== c.id && byId.has(parentId);
    if (hasRealParent) {
      const siblings = childrenOf.get(parentId!) ?? [];
      childrenOf.set(parentId!, [...siblings, c]);
    } else {
      roots.push(c);
    }
  }

  const byPosition = (a: Category, b: Category) => a.position - b.position;
  const out: CategoryNode[] = [];
  const walk = (nodes: Category[], depth: number) => {
    for (const c of [...nodes].sort(byPosition)) {
      out.push({ category: c, depth });
      walk(childrenOf.get(c.id) ?? [], depth + 1);
    }
  };
  walk(roots, 0);
  return out;
}

interface CategoryPickerProps {
  categories: Category[];
  /** What the product currently links to — the lean refs the product embeds. */
  selected: CategoryRef[];
  /** Receives the FULL desired id set. The backend REPLACES links, not merges. */
  onChange: (ids: string[]) => void;
}

export function CategoryPicker({ categories, selected, onChange }: CategoryPickerProps) {
  const selectedIds = new Set(selected.map((c) => c.id));
  const nodes = sortCategoryTree(categories);

  const toggle = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    // Preserve the store's own ordering rather than Set insertion order.
    onChange(nodes.map((n) => n.category.id).filter((cid) => next.has(cid)));
  };

  return (
    <View style={styles.root}>
      {nodes.map(({ category, depth }) => {
        const isSelected = selectedIds.has(category.id);
        return (
          <Pressable
            key={category.id}
            style={[styles.row, { paddingLeft: theme.spacing.md * depth }]}
            onPress={() => toggle(category.id)}
            accessibilityRole="checkbox"
            accessibilityState={{ checked: isSelected }}
            accessibilityLabel={category.name}
          >
            <View style={[styles.box, isSelected && styles.boxChecked]}>
              {isSelected ? <Check size={12} color={theme.colors.inverse} strokeWidth={3} /> : null}
            </View>
            <Text preset="body" color={isSelected ? "text" : "textSecondary"}>
              {category.name}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.xs },
  row: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm, height: 44 },
  box: {
    width: 20,
    height: 20,
    borderRadius: theme.radii.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  boxChecked: { backgroundColor: theme.colors.accent, borderColor: theme.colors.accent },
});
