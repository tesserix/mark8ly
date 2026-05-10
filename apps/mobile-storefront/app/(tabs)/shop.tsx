import { useState } from "react";
import {
  FlatList,
  RefreshControl,
  ScrollView,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  View,
} from "react-native";
import { Search, X } from "lucide-react-native";
import { useLocalSearchParams } from "expo-router";
import { useCategories, useProducts } from "@/lib/hooks/use-catalog";
import { ProductCard } from "@/components/ProductCard";
import { EmptyState, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

export default function ShopScreen() {
  const params = useLocalSearchParams<{ category?: string }>();
  const [activeCategory, setActiveCategory] = useState<string | null>(
    typeof params.category === "string" ? params.category : null,
  );
  const [search, setSearch] = useState("");

  const categories = useCategories();
  const products = useProducts({
    ...(activeCategory ? { category: activeCategory } : {}),
    ...(search ? { search } : {}),
  });

  return (
    <Screen>
      <PageHeader eyebrow="SHOP" title="All products" />

      <View style={styles.searchWrap}>
        <Search size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
        <TextInput
          value={search}
          onChangeText={setSearch}
          placeholder="Search products"
          placeholderTextColor={theme.colors.textTertiary}
          style={styles.searchInput}
          autoCapitalize="none"
          autoCorrect={false}
          returnKeyType="search"
        />
        {search ? (
          <TouchableOpacity onPress={() => setSearch("")} hitSlop={8} accessibilityLabel="Clear search">
            <X size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
          </TouchableOpacity>
        ) : null}
      </View>

      {categories.data?.items?.length ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.chips}
        >
          <Chip
            label="All"
            active={!activeCategory}
            onPress={() => setActiveCategory(null)}
          />
          {categories.data.items.map((c) => (
            <Chip
              key={c.id}
              label={c.name}
              active={activeCategory === c.slug}
              onPress={() => setActiveCategory(c.slug)}
            />
          ))}
        </ScrollView>
      ) : null}

      <FlatList
        data={products.data?.items ?? []}
        keyExtractor={(p) => p.id}
        numColumns={2}
        columnWrapperStyle={styles.row}
        ItemSeparatorComponent={() => <View style={{ height: theme.spacing.lg }} />}
        renderItem={({ item }) => <ProductCard product={item} />}
        contentContainerStyle={styles.grid}
        refreshControl={
          <RefreshControl
            refreshing={products.isRefetching}
            onRefresh={products.refetch}
            tintColor={theme.colors.text}
          />
        }
        ListEmptyComponent={
          <EmptyState
            title={products.isLoading ? "Loading…" : "No products"}
            message={
              products.isLoading
                ? undefined
                : search
                  ? `Nothing matches “${search}”.`
                  : "This category is empty for now."
            }
          />
        }
      />
    </Screen>
  );
}

function Chip({
  label,
  active,
  onPress,
}: {
  label: string;
  active: boolean;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity
      onPress={onPress}
      activeOpacity={0.7}
      style={[styles.chip, active && styles.chipActive]}
      accessibilityRole="button"
      accessibilityState={{ selected: active }}
      accessibilityLabel={`Filter by ${label}`}
    >
      <Text
        preset="bodyEmphasis"
        color={active ? "inverse" : "text"}
      >
        {label}
      </Text>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  searchWrap: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
    marginHorizontal: theme.spacing.lg,
    paddingHorizontal: theme.spacing.md,
    height: 44,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.elevated,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
  searchInput: {
    flex: 1,
    fontFamily: theme.fonts.sans,
    fontSize: 14,
    color: theme.colors.text,
    paddingVertical: 0,
  },
  chips: {
    gap: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
  },
  chip: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    borderRadius: theme.radii.pill,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    backgroundColor: theme.colors.elevated,
    minHeight: 36,
    justifyContent: "center",
  },
  chipActive: {
    backgroundColor: theme.colors.primary,
    borderColor: theme.colors.primary,
  },
  grid: {
    paddingHorizontal: theme.spacing.lg,
    paddingBottom: theme.spacing.huge,
    gap: theme.spacing.lg,
  },
  row: { gap: theme.spacing.lg },
});
