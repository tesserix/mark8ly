import { View, StyleSheet } from "react-native";
import { ChevronRight, Bookmark } from "lucide-react-native";
import { PressableRow, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { ReviewStars } from "./ReviewStars";
import { ReviewStatusBadge } from "./ReviewStatusBadge";
import type { Review } from "@repo/mobile-shared/api/types";

interface ReviewRowProps {
  review: Review;
  onPress: (review: Review) => void;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-AU", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function ReviewRow({ review, onPress }: ReviewRowProps) {
  const preview = review.title || review.content;
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(review)}
      style={styles.row}
      testID={`review-row-${review.id}`}
      accessibilityLabel={`Review by ${review.customer_name}, ${review.rating} stars, ${review.status}`}
    >
      <View style={styles.info}>
        <View style={styles.topRow}>
          <ReviewStars rating={review.rating} />
          <ReviewStatusBadge status={review.status} />
          {review.featured ? (
            <Bookmark size={13} color={theme.colors.textSecondary} strokeWidth={2} fill={theme.colors.textSecondary} />
          ) : null}
        </View>
        {preview ? (
          <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
            {preview}
          </Text>
        ) : null}
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {review.customer_name} · {formatDate(review.created_at)}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
});
