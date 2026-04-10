import { View, Text, StyleSheet } from "react-native";

interface StarRatingProps {
  rating: number;
  count?: number;
  size?: number;
  showCount?: boolean;
}

export function StarRating({
  rating,
  count,
  size = 14,
  showCount = true,
}: StarRatingProps) {
  const fullStars = Math.floor(rating);
  const hasHalf = rating - fullStars >= 0.5;
  const emptyStars = 5 - fullStars - (hasHalf ? 1 : 0);

  return (
    <View style={styles.container}>
      <Text style={[styles.stars, { fontSize: size }]}>
        {"★".repeat(fullStars)}
        {hasHalf ? "½" : ""}
        {"☆".repeat(emptyStars)}
      </Text>
      {showCount && count !== undefined && (
        <Text style={[styles.count, { fontSize: size - 2 }]}>({count})</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
  },
  stars: {
    color: "#0E0E0C",
    letterSpacing: 1,
  },
  count: {
    color: "#666666",
  },
});
