import {
  View,
  Text,
  StyleSheet,
  FlatList,
  Pressable,
  ActivityIndicator,
} from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { MessageSquare } from "lucide-react-native";
import { useMyReviews } from "@/lib/hooks/use-reviews";
import { StarRating } from "@/components/StarRating";
import type { CustomerReview } from "@/lib/storefront-api/reviews";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export default function ReviewsScreen() {
  const router = useRouter();
  const { data: reviews, isLoading } = useMyReviews();

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!reviews || reviews.length === 0) {
    return (
      <View style={styles.centered}>
        <MessageSquare size={48} color="#CCCCCC" />
        <Text style={styles.emptyTitle}>No reviews yet</Text>
        <Text style={styles.emptySubtitle}>
          Share your thoughts on products you have purchased.
        </Text>
        <Pressable
          style={styles.browseButton}
          onPress={() => router.push("/(tabs)/browse")}
          accessibilityRole="button"
        >
          <Text style={styles.browseButtonText}>Browse products</Text>
        </Pressable>
      </View>
    );
  }

  const renderItem = ({ item }: { item: CustomerReview }) => (
    <Pressable
      style={styles.card}
      onPress={() => router.push(`/(tabs)/browse/product/${item.product_handle}`)}
      accessibilityRole="button"
      accessibilityLabel={`Review for ${item.product_title}`}
    >
      <View style={styles.cardRow}>
        <View style={styles.thumbnailContainer}>
          {item.product_thumbnail ? (
            <Image
              source={{ uri: item.product_thumbnail }}
              style={styles.thumbnail}
              contentFit="cover"
            />
          ) : (
            <View style={styles.thumbnailPlaceholder} />
          )}
        </View>
        <View style={styles.cardContent}>
          <Text style={styles.productTitle} numberOfLines={1}>
            {item.product_title}
          </Text>
          <StarRating rating={item.rating} size={14} />
          <Text style={styles.reviewDate}>{formatDate(item.created_at)}</Text>
        </View>
      </View>
      {item.title.length > 0 && (
        <Text style={styles.reviewTitle}>{item.title}</Text>
      )}
      <Text style={styles.reviewBody} numberOfLines={3}>
        {item.body}
      </Text>
    </Pressable>
  );

  return (
    <FlatList
      style={styles.container}
      contentContainerStyle={styles.listContent}
      data={reviews}
      keyExtractor={(item) => item.id}
      renderItem={renderItem}
    />
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  listContent: {
    padding: 16,
    gap: 12,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 10,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    marginTop: 12,
  },
  emptySubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
    lineHeight: 20,
  },
  browseButton: {
    backgroundColor: "#0E0E0C",
    height: 44,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    marginTop: 8,
  },
  browseButtonText: {
    color: "#FFFFFF",
    fontSize: 15,
    fontWeight: "600",
  },
  card: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 14,
    gap: 8,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  cardRow: {
    flexDirection: "row",
    gap: 12,
    alignItems: "center",
  },
  thumbnailContainer: {
    width: 48,
    height: 48,
    borderRadius: 6,
    overflow: "hidden",
    backgroundColor: "#E5E4DF",
  },
  thumbnail: {
    width: "100%",
    height: "100%",
  },
  thumbnailPlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: "#E5E4DF",
  },
  cardContent: {
    flex: 1,
    gap: 3,
  },
  productTitle: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  reviewDate: {
    fontSize: 12,
    color: "#999999",
  },
  reviewTitle: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  reviewBody: {
    fontSize: 13,
    color: "#666666",
    lineHeight: 19,
  },
});
