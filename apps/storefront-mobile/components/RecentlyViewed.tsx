import { useMemo } from "react";
import { View, Text, FlatList, StyleSheet, Pressable } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import {
  useRecentlyViewedStore,
  type RecentProduct,
} from "@/stores/recently-viewed-store";

export function RecentlyViewed() {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const products = useRecentlyViewedStore((s) => s.products);
  const router = useRouter();

  if (products.length === 0) return null;

  const renderItem = ({ item }: { item: RecentProduct }) => (
    <Pressable
      style={styles.card}
      onPress={() => router.push(`/(tabs)/browse/product/${item.handle}`)}
    >
      <View style={styles.imageContainer}>
        {item.imageUrl ? (
          <Image
            source={{ uri: item.imageUrl }}
            style={styles.image}
            contentFit="cover"
            transition={200}
          />
        ) : (
          <View style={styles.imagePlaceholder} />
        )}
      </View>
      <Text style={styles.title} numberOfLines={1}>
        {item.title}
      </Text>
      <Text style={styles.price}>
        {item.currencyCode} {item.priceAmount.toFixed(2)}
      </Text>
    </Pressable>
  );

  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>Recently viewed</Text>
      <FlatList
        data={products}
        keyExtractor={(item) => item.handle}
        renderItem={renderItem}
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={styles.list}
      />
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  section: {
    marginTop: 24,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "600",
    color: theme.text,
    paddingHorizontal: 16,
    marginBottom: 12,
    fontFamily: "SourceSerif4",
  },
  list: {
    paddingHorizontal: 16,
  },
  card: {
    width: 100,
    marginRight: 12,
  },
  imageContainer: {
    width: 100,
    height: 100,
    borderRadius: 6,
    overflow: "hidden",
    backgroundColor: theme.background,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: theme.border,
  },
  image: {
    width: "100%",
    height: "100%",
  },
  imagePlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: theme.border,
  },
  title: {
    fontSize: 11,
    fontWeight: "500",
    color: theme.text,
    marginTop: 6,
  },
  price: {
    fontSize: 11,
    fontWeight: "600",
    color: theme.text,
    marginTop: 2,
  },
});
}
