import { View, StyleSheet } from "react-native";
import { Star } from "lucide-react-native";
import { theme } from "@/lib/theme";

interface ReviewStarsProps {
  /** 0-5. Values are rounded to the nearest whole star. */
  rating: number;
  size?: number;
}

/**
 * Filled stars are INK, not moss — a rating is editorial content shown on every
 * row, so it must not spend the one moss accent (reserved for primary actions).
 * Empty stars are a hairline outline.
 */
export function ReviewStars({ rating, size = 14 }: ReviewStarsProps) {
  const filled = Math.max(0, Math.min(5, Math.round(rating)));
  return (
    <View style={styles.row} accessibilityLabel={`${filled} out of 5 stars`}>
      {Array.from({ length: 5 }, (_, i) => {
        const on = i < filled;
        return (
          <Star
            key={i}
            size={size}
            strokeWidth={1.75}
            color={on ? theme.colors.text : theme.colors.border}
            fill={on ? theme.colors.text : "transparent"}
          />
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", gap: 2 },
});
