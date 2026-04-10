import { ScrollView, Text, StyleSheet, Pressable } from "react-native";
import { useRouter } from "expo-router";
import type { StorefrontCategory } from "@repo/mobile-shared/api/storefront-types";

interface CategoryPillsProps {
  categories: StorefrontCategory[];
}

export function CategoryPills({ categories }: CategoryPillsProps) {
  const router = useRouter();

  if (categories.length === 0) return null;

  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      contentContainerStyle={styles.container}
    >
      {categories.map((cat) => (
        <Pressable
          key={cat.id}
          style={styles.pill}
          onPress={() => router.push(`/(tabs)/browse/category/${cat.slug}`)}
        >
          <Text style={styles.pillText}>{cat.name}</Text>
        </Pressable>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {
    paddingHorizontal: 16,
    paddingVertical: 12,
    gap: 8,
  },
  pill: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 20,
    backgroundColor: "#FFFFFF",
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
    marginRight: 8,
  },
  pillText: {
    fontSize: 13,
    fontWeight: "500",
    color: "#0E0E0C",
  },
});
