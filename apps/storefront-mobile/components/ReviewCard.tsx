import { View, Text, StyleSheet } from "react-native";
import { StarRating } from "./StarRating";

interface ReviewCardProps {
  rating: number;
  title: string;
  body: string;
  date: string;
}

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  } catch {
    return iso;
  }
}

export function ReviewCard({ rating, title, body, date }: ReviewCardProps) {
  return (
    <View style={styles.card}>
      <View style={styles.header}>
        <StarRating rating={rating} showCount={false} size={12} />
        <Text style={styles.date}>{formatDate(date)}</Text>
      </View>
      {title ? <Text style={styles.title}>{title}</Text> : null}
      {body ? (
        <Text style={styles.body} numberOfLines={4}>
          {body}
        </Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "#E5E4DF",
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 6,
  },
  date: {
    fontSize: 12,
    color: "#666666",
  },
  title: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
    marginBottom: 4,
  },
  body: {
    fontSize: 13,
    color: "#666666",
    lineHeight: 18,
  },
});
