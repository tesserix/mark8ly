import { useMemo, useCallback } from "react";
import { View, Text, StyleSheet, FlatList, Pressable, ActivityIndicator } from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { MessageSquare } from "lucide-react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useMyReviews } from "@/lib/hooks/use-reviews";
import { StarRating } from "@/components/StarRating";
import type { CustomerReview } from "@/lib/storefront-api/reviews";

function formatDate(iso: string): string { return new Date(iso).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }); }

export default function ReviewsScreen() {
  const router = useRouter();
  const theme = useTheme();
  const { data: reviews, isLoading } = useMyReviews();

  const themed = useMemo(() => ({
    container: { backgroundColor: theme.background }, centered: { backgroundColor: theme.background },
    emptyTitle: { color: theme.text }, emptySubtitle: { color: theme.textSecondary },
    browseButton: { backgroundColor: theme.primary }, browseButtonText: { color: theme.elevated },
    card: { backgroundColor: theme.elevated, borderColor: theme.border },
    thumbnailContainer: { backgroundColor: theme.border }, thumbnailPlaceholder: { backgroundColor: theme.border },
    productTitle: { color: theme.text }, reviewDate: { color: theme.textSecondary },
    reviewTitle: { color: theme.text }, reviewBody: { color: theme.textSecondary },
  }), [theme]);

  const renderItem = useCallback(({ item }: { item: CustomerReview }) => (
    <Pressable style={[styles.card, themed.card]} onPress={() => router.push(`/(tabs)/browse/product/${item.product_handle}`)} accessibilityRole="button" accessibilityLabel={`Review for ${item.product_title}`}>
      <View style={styles.cardRow}>
        <View style={[styles.thumbnailContainer, themed.thumbnailContainer]}>
          {item.product_thumbnail ? (<Image source={{ uri: item.product_thumbnail }} style={styles.thumbnail} contentFit="cover" accessibilityLabel={item.product_title} />) : (<View style={[styles.thumbnailPlaceholder, themed.thumbnailPlaceholder]} />)}
        </View>
        <View style={styles.cardContent}>
          <Text style={[styles.productTitle, themed.productTitle]} numberOfLines={1}>{item.product_title}</Text>
          <StarRating rating={item.rating} size={14} />
          <Text style={[styles.reviewDate, themed.reviewDate]}>{formatDate(item.created_at)}</Text>
        </View>
      </View>
      {item.title.length > 0 && <Text style={[styles.reviewTitle, themed.reviewTitle]}>{item.title}</Text>}
      <Text style={[styles.reviewBody, themed.reviewBody]} numberOfLines={3}>{item.body}</Text>
    </Pressable>
  ), [themed, router]);

  if (isLoading) { return (<View style={[styles.centered, themed.centered]}><ActivityIndicator size="large" color={theme.primary} /></View>); }

  if (!reviews || reviews.length === 0) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <MessageSquare size={48} color="#CCCCCC" />
        <Text style={[styles.emptyTitle, themed.emptyTitle]}>No reviews yet</Text>
        <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>Share your thoughts on products you have purchased.</Text>
        <Pressable style={[styles.browseButton, themed.browseButton]} onPress={() => router.push("/(tabs)/browse")} accessibilityRole="button" accessibilityLabel="Browse products"><Text style={[styles.browseButtonText, themed.browseButtonText]}>Browse products</Text></Pressable>
      </View>
    );
  }

  return (<FlatList style={[styles.container, themed.container]} contentContainerStyle={styles.listContent} data={reviews} keyExtractor={(item) => item.id} renderItem={renderItem} />);
}

const styles = StyleSheet.create({
  container: { flex: 1 }, listContent: { padding: 16, gap: 12 },
  centered: { flex: 1, alignItems: "center", justifyContent: "center", paddingHorizontal: 32, gap: 10 },
  emptyTitle: { fontSize: 18, fontWeight: "700", marginTop: 12 }, emptySubtitle: { fontSize: 14, textAlign: "center", lineHeight: 20 },
  browseButton: { height: 44, borderRadius: 6, alignItems: "center", justifyContent: "center", paddingHorizontal: 32, marginTop: 8 }, browseButtonText: { fontSize: 15, fontWeight: "600" },
  card: { borderRadius: 6, padding: 14, gap: 8, borderWidth: StyleSheet.hairlineWidth },
  cardRow: { flexDirection: "row", gap: 12, alignItems: "center" },
  thumbnailContainer: { width: 48, height: 48, borderRadius: 6, overflow: "hidden" }, thumbnail: { width: "100%", height: "100%" }, thumbnailPlaceholder: { width: "100%", height: "100%" },
  cardContent: { flex: 1, gap: 3 }, productTitle: { fontSize: 14, fontWeight: "600" }, reviewDate: { fontSize: 12 },
  reviewTitle: { fontSize: 14, fontWeight: "600" }, reviewBody: { fontSize: 13, lineHeight: 19 },
});
