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
  /**
   * Opens the row's long-press menu. OPTIONAL rather than always-supplied
   * because absence is the guard: the list screen passes `undefined` while
   * this row's own request is in flight, which is what actually disarms the
   * gesture — a handler that returned early would still let the row engage
   * its long-press feedback. `SwipeRow.enabled` does NOT reach this, which is
   * why the two controls are gated separately.
   */
  onLongPress?: (review: Review) => void;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-AU", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function ReviewRow({ review, onPress, onLongPress }: ReviewRowProps) {
  const preview = review.title || review.content;
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(review)}
      onLongPress={onLongPress ? () => onLongPress(review) : undefined}
      style={styles.row}
      testID={`review-row-${review.id}`}
      accessibilityLabel={`Review by ${review.customer_name}, ${review.rating} stars, ${review.status}`}
      accessibilityHint={onLongPress ? "Long press for more actions" : undefined}
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
