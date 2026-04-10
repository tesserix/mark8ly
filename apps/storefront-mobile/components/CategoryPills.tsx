import { useMemo } from "react";
import { ScrollView, Text, StyleSheet, Pressable } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useRouter } from "expo-router";
import type { StorefrontCategory } from "@repo/mobile-shared/api/storefront-types";

interface CategoryPillsProps {
  categories: StorefrontCategory[];
}

export function CategoryPills({ categories }: CategoryPillsProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
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

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    paddingHorizontal: 16,
    paddingVertical: 12,
    gap: 8,
  },
  pill: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 20,
    backgroundColor: theme.elevated,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: theme.border,
    marginRight: 8,
  },
  pillText: {
    fontSize: 13,
    fontWeight: "500",
    color: theme.text,
  },
});
}
